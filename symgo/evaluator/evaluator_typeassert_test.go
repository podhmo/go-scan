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

func TestTypeAssertExpr(t *testing.T) {
	cases := []struct {
		name          string
		source        string
		inspectedVars map[string]string // var name -> expected type name
	}{
		{
			name: "single value success",
			source: `
package main
func main() {
	var i any = "hello"
	s := i.(string)
	_ = s
}
`,
			inspectedVars: map[string]string{
				"s": "string",
			},
		},
		{
			name: "two values success",
			source: `
package main
func main() {
	var i any = "hello"
	s, ok := i.(string)
	_, _ = s, ok
}
`,
			inspectedVars: map[string]string{
				"s":  "string",
				"ok": "bool",
			},
		},
		{
			name: "two values failure",
			source: `
package main
func main() {
	var i any = 123 // Different type
	s, ok := i.(string)
	_, _ = s, ok
}
`,
			inspectedVars: map[string]string{
				"s":  "string",
				"ok": "bool",
			},
		},
		{
			name: "assertion on struct",
			source: `
package main
type MyStruct struct { Name string }
func main() {
	var i any = MyStruct{Name: "test"}
	s, ok := i.(MyStruct)
	_, _ = s, ok
}
`,
			inspectedVars: map[string]string{
				"s":  "MyStruct",
				"ok": "bool",
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"go.mod":  "module example.com/main",
				"main.go": tt.source,
			}

			dir, cleanup := scantest.WriteFiles(t, files)
			defer cleanup()

			action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
				mainPkg := pkgs[0]
				eval := New(s, s.Logger, nil, nil)

				// 1. Evaluate the file to populate the top-level declarations (like `main` func).
				env := object.NewEnvironment()
				for _, file := range mainPkg.AstFiles {
					eval.Eval(ctx, file, env, mainPkg)
				}

				// 2. Find the main function.
				mainFuncObj, ok := env.Get("main")
				if !ok {
					return fmt.Errorf("main function not found")
				}
				mainFunc, ok := mainFuncObj.(*object.Function)
				if !ok {
					return fmt.Errorf("main is not a function, but %T", mainFuncObj)
				}

				// 3. Evaluate the body of the main function in a new scope.
				mainEnv := object.NewEnclosedEnvironment(env)
				result := eval.Eval(ctx, mainFunc.Body, mainEnv, mainPkg)
				if err, ok := result.(*object.Error); ok && err != nil {
					return fmt.Errorf("evaluation failed unexpectedly: %s", err.Message)
				}

				// 4. Inspect the resulting environment.
				actualInspectedVars := make(map[string]string)
				for varName := range tt.inspectedVars {
					obj, found := mainEnv.Get(varName)
					if !found {
						return fmt.Errorf("variable %q not found in environment", varName)
					}
					v, ok := obj.(*object.Variable)
					if !ok {
						return fmt.Errorf("object %q is not a variable, but %T", varName, obj)
					}

					var actualTypeName string
					if v.Value.FieldType() != nil {
						actualTypeName = v.Value.FieldType().Name
					} else if v.Value.TypeInfo() != nil {
						actualTypeName = v.Value.TypeInfo().Name
					} else {
						// Fallback for builtins that might not have full type info attached
						// in all placeholder scenarios.
						switch v.Value.(type) {
						case *object.Boolean:
							actualTypeName = "bool"
						default:
							// Check the variable's static type as a last resort.
							if v.FieldType() != nil {
								actualTypeName = v.FieldType().Name
							} else if v.TypeInfo() != nil {
								actualTypeName = v.TypeInfo().Name
							} else {
								return fmt.Errorf("variable %q has no type information", varName)
							}
						}
					}
					actualInspectedVars[varName] = actualTypeName
				}

				if diff := cmp.Diff(tt.inspectedVars, actualInspectedVars); diff != "" {
					return fmt.Errorf("mismatch in inspected variable types (-want +got):\n%s", diff)
				}

				return nil
			}

			_, err := scantest.Run(t, t.Context(), dir, []string{"."}, action, scantest.WithModuleRoot(dir))
			if err != nil {
				t.Fatalf("scantest.Run() failed: %v", err)
			}
		})
	}
}
