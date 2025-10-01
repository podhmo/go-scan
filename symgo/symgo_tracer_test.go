package symgo_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/symgotest"
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
	tracer := &recordingTracer{}
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main

func main() {
	x := 1 + 2
	return
}`,
		},
		EntryPoint: "example.com/me.main",
		Options: []symgotest.Option{
			symgotest.WithTracer(tracer),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
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

	symgotest.Run(t, tc, action)
}
