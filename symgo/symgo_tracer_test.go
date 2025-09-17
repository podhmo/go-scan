package symgo_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

// recordingTracer is a simple implementation of symgo.Tracer that records the
// types of the AST nodes it visits.
type recordingTracer struct {
	visitedNodeTypes []string
}

func (t *recordingTracer) Trace(event symgo.TraceEvent) {
	if event.Node == nil {
		return
	}
	// Get the type name of the node (e.g., "*ast.Ident", "*ast.SelectorExpr")
	typeName := fmt.Sprintf("%T", event.Node)
	t.visitedNodeTypes = append(t.visitedNodeTypes, typeName)
}

func TestInterpreter_WithTracer(t *testing.T) {
	source := `package main

func main() {
	x := 1 + 2
	return
}`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	tracer := &recordingTracer{}

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		mainFunc := pkg.Functions[0].AstDecl

		interpreter, err := symgo.NewInterpreter(s, symgo.WithTracer(tracer))
		if err != nil {
			return fmt.Errorf("NewInterpreter failed: %w", err)
		}

		_, err = interpreter.Eval(ctx, mainFunc.Body, pkg)
		if err != nil {
			return fmt.Errorf("Eval failed: %w", err)
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	// Check the sequence of visited nodes.
	expected := []string{
		"*ast.BlockStmt",
		"*ast.AssignStmt",
		"*ast.BinaryExpr",
		"*ast.BasicLit", // 1
		"*ast.BasicLit", // 2
		"*ast.ReturnStmt",
	}

	if !reflect.DeepEqual(tracer.visitedNodeTypes, expected) {
		t.Errorf("Tracer did not record the expected node types.\nGot:  %v\nWant: %v", tracer.visitedNodeTypes, expected)
	}
}
