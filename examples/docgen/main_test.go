package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/types"
	"testing"

	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/symgo"
	goscan "github.com/podhmo/go-scan"
)

// Analyzer analyzes Go code and generates an OpenAPI specification.
type Analyzer struct {
	Scanner   *goscan.Scanner
	Evaluator *symgo.Evaluator
	OpenAPI   *openapi.OpenAPI
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(s *goscan.Scanner) *Analyzer {
	evaluator := symgo.New(s)
	return &Analyzer{
		Scanner:   s,
		Evaluator: evaluator,
		OpenAPI: &openapi.OpenAPI{
			OpenAPI: "3.1.0",
			Info: openapi.Info{
				Title:   "Sample API",
				Version: "0.0.1",
			},
			Paths: make(map[string]*openapi.PathItem),
		},
	}
}

// Analyze analyzes the package at the given import path.
func (a *Analyzer) Analyze(ctx context.Context, importPath string) error {
	pkg, err := a.Scanner.ScanPackageByImport(ctx, importPath)
	if err != nil {
		return fmt.Errorf("failed to load sample API package: %w", err)
	}

	registerHandlers, ok := pkg.Scope.Lookup("RegisterHandlers").(*types.Func)
	if !ok {
		return fmt.Errorf("RegisterHandlers function not found in package %s", importPath)
	}

	httpPkg, err := a.Scanner.ScanPackageByImport(ctx, "net/http")
	if err != nil {
		return fmt.Errorf("could not load net/http package: %w", err)
	}
	handleFuncObj, ok := httpPkg.Scope.Lookup("HandleFunc").(*types.Func)
	if !ok {
		return fmt.Errorf("http.HandleFunc not found")
	}

	a.Evaluator.Intrinsics.Register(handleFuncObj, func(evaluator *symgo.Evaluator, call *ast.CallExpr, scope *symgo.Scope) symgo.Object {
		if len(call.Args) != 2 {
			return symgo.NewError(fmt.Errorf("expected 2 arguments to http.HandleFunc, got %d", len(call.Args)))
		}

		pathLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return symgo.NewError(fmt.Errorf("expected string literal for path, got %T", call.Args[0]))
		}
		path := symgo.Unquote(pathLit.Value)

		handlerIdent, ok := call.Args[1].(*ast.Ident)
		if !ok {
			return symgo.NewError(fmt.Errorf("expected identifier for handler, got %T", call.Args[1]))
		}

		handlerSym := scope.Lookup(handlerIdent.Name)
		if handlerSym == nil {
			return symgo.NewError(fmt.Errorf("could not resolve handler function %q", handlerIdent.Name))
		}

		a.OpenAPI.Paths[path] = &openapi.PathItem{} // placeholder

		return symgo.Void
	})

	_, err = a.Evaluator.Eval(ctx, registerHandlers)
	if err != nil {
		return fmt.Errorf("failed to evaluate RegisterHandlers: %w", err)
	}

	return nil
}


func TestIt(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	s, err := goscan.New()
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	analyzer := NewAnalyzer(s)

	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}

	if _, ok := analyzer.OpenAPI.Paths["/users"]; !ok {
		t.Errorf("expected path /users to be registered, but it was not")
	}
}
