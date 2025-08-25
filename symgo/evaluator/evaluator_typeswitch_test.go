package evaluator

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestTypeSwitchStmt(t *testing.T) {
	source := `
package main

// inspect is a special function that will be implemented as an intrinsic
// to check the type of the variable passed to it.
func inspect(v any) {}

func main() {
	var x any = 123
	switch v := x.(type) {
	case int:
		inspect(v)
	case string:
		// This case exists to ensure we are correctly handling scopes,
		// but we only call main, so it won't be inspected.
		inspect(v)
	}
}
`
	files := map[string]string{
		"go.mod":  "module example.com/main",
		"main.go": source,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var inspectedTypes []string

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		// Register an intrinsic for the inspect function
		eval.RegisterIntrinsic("example.com/main.inspect", func(args ...object.Object) object.Object {
			if len(args) != 1 {
				return nil
			}
			arg := args[0]
			var typeName string
			if p, ok := arg.(*object.SymbolicPlaceholder); ok {
				if ft := p.FieldType(); ft != nil {
					typeName = ft.Name
				} else if ti := p.TypeInfo(); ti != nil {
					typeName = ti.Name
				}
			}

			if typeName == "" {
				if ft := arg.FieldType(); ft != nil {
					typeName = ft.Name
				} else if ti := arg.TypeInfo(); ti != nil {
					typeName = ti.Name
				}
			}

			if typeName == "" {
				// Fallback for basic types or literals that don't have full TypeInfo
				switch arg.(type) {
				case *object.Integer:
					typeName = "int"
				case *object.String:
					typeName = "string"
				case *object.Boolean:
					typeName = "bool"
				case *object.Nil:
					// This happens for the `var i interface{}` case before it's wrapped in a variable with type info.
					// A nil interface has no type. In Go, this is often represented as `<nil>`.
					// For the purpose of this test, let's call it "any" to match the interface{} type.
					typeName = "any"
				default:
					typeName = "unknown"
				}
			}
			inspectedTypes = append(inspectedTypes, typeName)
			return nil
		})

		env := object.NewEnvironment()
		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, env, mainPkg)
		}

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc := mainFuncObj.(*object.Function)

		result := eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)
		if err, ok := result.(*object.Error); ok && err != nil {
			return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
		}

		// Since the symbolic execution evaluates all branches, we expect inspect()
		// to be called for each case block.
		expectedTypes := []string{"int", "string"}
		if diff := cmp.Diff(expectedTypes, inspectedTypes); diff != "" {
			return fmt.Errorf("mismatch in inspected types (-want +got):\n%s", diff)
		}

		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action, scantest.WithModuleRoot(dir))
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
