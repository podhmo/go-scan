package symgo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSliceExprAndSelectStmt(t *testing.T) {
	t.Run("SliceExpr", func(t *testing.T) {
		source := `
package main
var s []int
func getLow() int { return 0 }
func getHigh() int { return 10 }
func main() {
	_ = s[getLow():getHigh()]
}
`
		dir, cleanup := scantest.WriteFiles(t, map[string]string{
			"go.mod":  "module example.com/me",
			"main.go": source,
		})
		defer cleanup()

		var calledFunctions []string

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			eval := evaluator.New(s, s.Logger, nil, nil)

			eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
				if len(args) > 0 {
					if fn, ok := args[0].(*object.Function); ok {
						if fn.Def != nil {
							calledFunctions = append(calledFunctions, fn.Def.Name)
						}
					}
				}
				return nil
			})

			if res := eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], nil, pkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial eval failed: %s", res.Inspect())
			}

			pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
			if !ok {
				return fmt.Errorf("could not get package environment for 'example.com/me'")
			}

			mainFunc, ok := pkgEnv.Get("main")
			if !ok {
				return fmt.Errorf("function 'main' not found")
			}

			if res := eval.Apply(ctx, mainFunc, []object.Object{}, pkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("evaluating main failed: %s", res.Inspect())
			}

			return nil
		}

		if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}

		calledMap := make(map[string]bool)
		for _, name := range calledFunctions {
			calledMap[name] = true
		}

		if !calledMap["getLow"] {
			t.Errorf("want 'getLow' to be called, but it wasn't. Called: %v", calledFunctions)
		}
		if !calledMap["getHigh"] {
			t.Errorf("want 'getHigh' to be called, but it wasn't. Called: %v", calledFunctions)
		}
	})

	t.Run("SelectStmt", func(t *testing.T) {
		source := `
package main
func getChan() chan int { return make(chan int) }
func handle() {}
func main() {
	select {
	case <-getChan():
		handle()
	}
}
`
		dir, cleanup := scantest.WriteFiles(t, map[string]string{
			"go.mod":  "module example.com/me",
			"main.go": source,
		})
		defer cleanup()

		var calledFunctions []string

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			eval := evaluator.New(s, s.Logger, nil, nil)

			eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
				if len(args) > 0 {
					if fn, ok := args[0].(*object.Function); ok {
						if fn.Def != nil {
							calledFunctions = append(calledFunctions, fn.Def.Name)
						}
					}
				}
				return nil
			})

			if res := eval.Eval(ctx, pkg.AstFiles[pkg.Files[0]], nil, pkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial eval failed: %s", res.Inspect())
			}

			pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
			if !ok {
				return fmt.Errorf("could not get package environment for 'example.com/me'")
			}

			mainFunc, ok := pkgEnv.Get("main")
			if !ok {
				return fmt.Errorf("function 'main' not found")
			}

			if res := eval.Apply(ctx, mainFunc, []object.Object{}, pkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("evaluating main failed: %s", res.Inspect())
			}
			return nil
		}

		if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}

		calledMap := make(map[string]bool)
		for _, name := range calledFunctions {
			calledMap[name] = true
		}

		if !calledMap["getChan"] {
			t.Errorf("want 'getChan' to be called, but it wasn't. Called: %s", strings.Join(calledFunctions, ", "))
		}
		if !calledMap["handle"] {
			t.Errorf("want 'handle' to be called, but it wasn't. Called: %s", strings.Join(calledFunctions, ", "))
		}
	})
}
