package symgo_test

import (
	"context"
	"go/parser"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestNewInterpreter(t *testing.T) {
	t.Run("nil scanner", func(t *testing.T) {
		_, err := symgo.NewInterpreter(nil, nil)
		if err == nil {
			t.Error("expected an error for nil scanner, but got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		dir, cleanup := scantest.WriteFiles(t, map[string]string{
			"go.mod": "module mymodule",
		})
		defer cleanup()

		s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
		if err != nil {
			t.Fatalf("goscan.New() failed: %+v", err)
		}

		interp, err := symgo.NewInterpreter(s, nil)
		if err != nil {
			t.Errorf("NewInterpreter() failed: %+v", err)
		}
		if interp == nil {
			t.Error("expected interpreter to be non-nil")
		}
	})
}

func TestInterpreter_Eval_Simple(t *testing.T) {
	source := `
package main
import "fmt"
func main() {
	fmt.Println("hello")
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	interp, err := symgo.NewInterpreter(s, nil)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// Evaluate an expression that uses an imported package.
	// The interpreter should lazy-load the package and return a symbolic placeholder.
	node, err := parser.ParseExpr(`fmt.Println`)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %+v", err)
	}

	// We need to evaluate the file first to process imports.
	mainGoPath := filepath.Join(dir, "main.go")
	fileAst, err := parser.ParseFile(s.Fset(), mainGoPath, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parser.ParseFile() failed: %+v", err)
	}
	_, err = interp.Eval(context.Background(), fileAst)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Now evaluate the expression
	result, err := interp.Eval(context.Background(), node)
	if err != nil {
		t.Fatalf("interp.Eval(expr) failed: %+v", err)
	}

	_, ok := result.(*object.SymbolicPlaceholder)
	if !ok {
		t.Errorf("Expected a SymbolicPlaceholder for an external function, but got %T", result)
	}
}

func TestInterpreter_RegisterIntrinsic(t *testing.T) {
	// Test that a registered intrinsic function is correctly called during evaluation.
	source := `
package main
import "fmt"
func main() {
	fmt.Println("hello")
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	interp, err := symgo.NewInterpreter(s, nil)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// Simplified intrinsic handler
	expectedResult := &object.String{Value: "Intrinsic was called!"}
	handler := func(eval *symgo.Interpreter, args []symgo.Object) symgo.Object {
		return expectedResult
	}
	interp.RegisterIntrinsic("fmt.Println", handler)

	// Evaluate an expression that calls the intrinsic
	node, err := parser.ParseExpr(`fmt.Println("hello")`)
	if err != nil {
		t.Fatalf("parser.ParseExpr() failed: %+v", err)
	}

	// We need to evaluate the file first to process imports.
	mainGoPath := filepath.Join(dir, "main.go")
	fileAst, err := parser.ParseFile(s.Fset(), mainGoPath, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parser.ParseFile() failed: %+v", err)
	}
	_, err = interp.Eval(context.Background(), fileAst)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Now evaluate the call expression
	result, err := interp.Eval(context.Background(), node)
	if err != nil {
		t.Fatalf("interp.Eval(expr) failed: %+v", err)
	}

	if diff := cmp.Diff(expectedResult, result); diff != "" {
		t.Errorf("result mismatch (-want +got):\n%s", diff)
	}
}
