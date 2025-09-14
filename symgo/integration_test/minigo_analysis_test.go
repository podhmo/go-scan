package integration_test

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	scan "github.com/podhmo/go-scan/scanner"
)

func TestAnalyzeMinigoPackage(t *testing.T) {
	// t.Skip("skipping test that reproduces an infinite recursion bug, as per user instruction")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("could not get absolute path for project root: %v", err)
	}

	scanner, err := goscan.New(goscan.WithWorkDir(root), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	minigoPackagePrefix := "github.com/podhmo/go-scan/minigo"
	pkgs, err := scanner.Scan(ctx, minigoPackagePrefix+"/...")
	if err != nil {
		t.Fatalf("failed to scan packages: %v", err)
	}

	logLevel := new(slog.LevelVar)
	// logLevel.Set(slog.LevelDebug) // Uncomment for verbose logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     logLevel,
	}))

	interp, err := symgo.NewInterpreter(scanner,
		symgo.WithLogger(logger),
		symgo.WithPrimaryAnalysisScope(
			minigoPackagePrefix,
			minigoPackagePrefix+"/evaluator",
			minigoPackagePrefix+"/object",
			minigoPackagePrefix+"/ffibridge",
		),
	)
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}
	t.Logf("interpreter created successfully with primary analysis scope: %s/...", minigoPackagePrefix)

	// 1. Load all packages into the interpreter.
	for _, pkg := range pkgs {
		for _, fileAst := range pkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, pkg); err != nil {
				t.Logf("initial load warning for file %s: %v", scanner.Fset().File(fileAst.Pos()).Name(), err)
			}
		}
	}
	t.Logf("Finished loading all %d packages.", len(pkgs))

	// 2. Iterate through the loaded files and directly analyze function bodies.
	var functionsAnalyzed int
	for _, fileScope := range interp.Files() {
		if fileScope.AST == nil {
			continue
		}

		filePath := scanner.Fset().File(fileScope.AST.Pos()).Name()
		isMinigoFile := false
		var currentPkg *goscan.Package
		for _, pkg := range pkgs {
			if strings.HasPrefix(pkg.ImportPath, minigoPackagePrefix) {
				for _, goFile := range pkg.Files {
					if goFile == filePath {
						isMinigoFile = true
						currentPkg = pkg
						break
					}
				}
			}
			if isMinigoFile {
				break
			}
		}

		if !isMinigoFile {
			continue
		}

		t.Logf("Scanning file for functions: %s", filePath)

		pkgEnv, ok := interp.PackageEnvForTest(currentPkg.ImportPath)
		if !ok {
			t.Fatalf("could not get package environment for %s", currentPkg.ImportPath)
		}

		for _, decl := range fileScope.AST.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}

			var funcInfo *scan.FunctionInfo
			for _, f := range currentPkg.Functions {
				if f.AstDecl == funcDecl {
					funcInfo = f
					break
				}
			}

			fn := &object.Function{
				Name:       funcDecl.Name,
				Parameters: funcDecl.Type.Params,
				Body:       funcDecl.Body,
				Decl:       funcDecl,
				Package:    currentPkg,
				Env:        pkgEnv, // Use the correct package-level environment
				Def:        funcInfo,
			}

			fnName := fmt.Sprintf("%s.%s", currentPkg.ImportPath, funcDecl.Name.Name)
			t.Logf("Analyzing function: %s", fnName)
			functionsAnalyzed++

			// Create a dummy call expression to satisfy the ApplyFunction signature.
			dummyCall := &ast.CallExpr{
				Fun:  funcDecl.Name,
				Args: []ast.Expr{},
			}
			result := interp.ApplyFunction(ctx, dummyCall, fn, nil, fileScope)

			// The analysis should now succeed for all functions.
			if err, isErr := result.(*object.Error); isErr {
				t.Fatalf("Analysis of function %s failed unexpectedly: %s", fnName, err.Inspect())
			}
		}
	}

	if functionsAnalyzed == 0 {
		t.Fatal("Test setup failed: no function declarations were found to analyze in any of the loaded minigo files.")
	}
}
