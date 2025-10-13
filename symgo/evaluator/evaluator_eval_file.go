package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"os"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalFile(ctx context.Context, file *ast.File, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Get the canonical package object for this file. This is the source of truth
	// for the package's isolated environment.
	pkgObj, err := e.getOrLoadPackage(ctx, pkg.ImportPath)
	if err != nil {
		e.logc(ctx, slog.LevelWarn, "could not load package for file evaluation", "package", pkg.ImportPath, "error", err)
		// We cannot proceed with evaluation if the package context cannot be loaded.
		return nil
	}
	targetEnv := pkgObj.Env // Always use the package's own, isolated environment.

	// Populate package-level constants and functions once per package.
	// This will correctly populate targetEnv.
	e.ensurePackageEnvPopulated(ctx, pkgObj)

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			switch d.Tok {
			case token.VAR:
				result := e.evalGenDecl(ctx, d, targetEnv, pkg)
				if isError(result) {
					fmt.Fprintf(os.Stderr, "!!!!!!!@@error: %s\n", result.Inspect())
				}
			case token.TYPE:
				e.evalTypeDecl(ctx, d, targetEnv, pkg)
			}
		case *ast.FuncDecl:
			var funcInfo *scan.FunctionInfo
			for _, f := range pkg.Functions {
				if f.AstDecl == d {
					funcInfo = f
					break
				}
			}
			// When creating the function object, it's critical that its definition
			// environment (`Env`) is the isolated environment of its package.
			fn := &object.Function{
				Name:       d.Name,
				Parameters: d.Type.Params,
				Body:       d.Body,
				Env:        targetEnv, // Use the package's own environment
				Decl:       d,
				Package:    pkg,
				Def:        funcInfo,
			}
			targetEnv.Set(d.Name.Name, fn) // Set in the package's own environment
		}
	}
	return nil
}
