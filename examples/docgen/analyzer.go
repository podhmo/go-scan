package main

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/podhmo/go-scan/examples/docgen/openapi"
	goscan "github.com/podhmo/go-scan"
)

// Analyzer analyzes Go code and generates an OpenAPI specification.
type Analyzer struct {
	Scanner *goscan.Scanner
	OpenAPI *openapi.OpenAPI
}

// NewAnalyzer creates a new Analyzer.
func NewAnalyzer(s *goscan.Scanner) (*Analyzer, error) {
	return &Analyzer{
		Scanner: s,
		OpenAPI: &openapi.OpenAPI{
			OpenAPI: "3.1.0",
			Info: openapi.Info{
				Title:   "Sample API",
				Version: "0.0.1",
			},
			Paths: make(map[string]*openapi.PathItem),
		},
	}, nil
}

// Analyze analyzes the package at the given import path.
func (a *Analyzer) Analyze(ctx context.Context, importPath string) error {
	pkg, err := a.Scanner.ScanPackageByImport(ctx, importPath)
	if err != nil {
		return fmt.Errorf("failed to load sample API package: %w", err)
	}

	// Create a map of function names to their declarations for easy lookup.
	funcDecls := make(map[string]*ast.FuncDecl)
	for _, f := range pkg.Functions {
		funcDecls[f.Name] = f.AstDecl
	}

	// Find the RegisterHandlers function.
	registerHandlersDecl, ok := funcDecls["RegisterHandlers"]
	if !ok {
		return fmt.Errorf("RegisterHandlers function not found in package %s", importPath)
	}

	// Walk the AST of RegisterHandlers to find calls to http.HandleFunc.
	ast.Inspect(registerHandlersDecl, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true // Continue traversal
		}

		// Check if this is a call to http.HandleFunc
		selector, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		xIdent, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if xIdent.Name == "http" && selector.Sel.Name == "HandleFunc" {
			a.analyzeHandleFuncCall(callExpr, funcDecls)
		}

		return true
	})

	return nil
}

func (a *Analyzer) analyzeHandleFuncCall(callExpr *ast.CallExpr, funcDecls map[string]*ast.FuncDecl) {
	if len(callExpr.Args) != 2 {
		return // Not a valid HandleFunc call
	}

	// Extract path from the first argument.
	pathLit, ok := callExpr.Args[0].(*ast.BasicLit)
	if !ok || pathLit.Kind != token.STRING {
		return
	}
	path, err := strconv.Unquote(pathLit.Value)
	if err != nil {
		return
	}

	// Extract handler name from the second argument.
	handlerIdent, ok := callExpr.Args[1].(*ast.Ident)
	if !ok {
		return
	}
	handlerName := handlerIdent.Name

	// Look up the handler's function declaration.
	handlerDecl, ok := funcDecls[handlerName]
	if !ok {
		return
	}

	// Create the OpenAPI operation.
	op := &openapi.Operation{
		OperationID: handlerName,
	}
	if handlerDecl.Doc != nil {
		op.Description = handlerDecl.Doc.Text()
	}

	// For now, assume GET.
	a.OpenAPI.Paths[path] = &openapi.PathItem{
		Get: op,
	}
}
