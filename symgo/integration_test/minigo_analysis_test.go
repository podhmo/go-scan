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
)

func TestAnalyzeMinigoPackage(t *testing.T) {
	// t.Skip("skipping test that reproduces an infinite recursion bug, as per user instruction")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("could not get absolute path for project root: %v", err)
	}

	s, err := goscan.New(goscan.WithWorkDir(root))
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	minigoPackagePrefix := "github.com/podhmo/go-scan/minigo"
	pkgs, err := s.Scan(ctx, minigoPackagePrefix+"/...")
	if err != nil {
		t.Fatalf("failed to scan packages: %v", err)
	}

	logLevel := new(slog.LevelVar)
	// logLevel.Set(slog.LevelDebug) // Uncomment for verbose logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     logLevel,
	}))

	interp, err := symgo.NewInterpreter(s,
		symgo.WithLogger(logger),
		symgo.WithPrimaryAnalysisScope(minigoPackagePrefix+"/..."),
	)
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}
	t.Logf("interpreter created successfully with primary analysis scope: %s/...", minigoPackagePrefix)

	// 1. Load all packages into the interpreter.
	for _, pkg := range pkgs {
		for _, fileAst := range pkg.AstFiles {
			if _, err := interp.Eval(ctx, fileAst, pkg); err != nil {
				t.Logf("initial load warning for file %s: %v", s.Fset().File(fileAst.Pos()).Name(), err)
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

		filePath := s.Fset().File(fileScope.AST.Pos()).Name()
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

		for _, decl := range fileScope.AST.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}

			fn := &object.Function{
				Name:       funcDecl.Name,
				Parameters: funcDecl.Type.Params,
				Body:       funcDecl.Body,
				Decl:       funcDecl,
				Package:    currentPkg,
				Env:        interp.GlobalEnvForTest(),
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

			if err, isErr := result.(*object.Error); isErr {
				t.Logf("Successfully reproduced analysis failure for function %s: %s", fnName, err.Inspect())
				return
			}
		}
	}

	if functionsAnalyzed == 0 {
		t.Fatal("Test setup failed: no function declarations were found to analyze in any of the loaded minigo files.")
	}

	t.Errorf("Test completed without triggering the expected analysis failure")
}
