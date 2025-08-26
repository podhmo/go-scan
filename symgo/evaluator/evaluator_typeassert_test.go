package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_TypeAssertExpr(t *testing.T) {
	t.Run("single value", func(t *testing.T) {
		source := `
package main
type Stringer interface {
	String() string
}
type MyString string
func (s MyString) String() string { return string(s) }
func assertType(s MyString) {}

func main() {
	var i Stringer = MyString("hello")
	s := i.(MyString)
	assertType(s)
}
`
		files := map[string]string{
			"main.go": source,
			"go.mod":  "module main",
		}
		dir, cleanup := scantest.WriteFiles(t, files)
		defer cleanup()

		var assertedValue object.Object
		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			e := New(s, nil, nil, nil)

			e.RegisterIntrinsic("assertType", func(args ...object.Object) object.Object {
				if len(args) > 0 {
					assertedValue = args[0]
				}
				return nil
			})

			env := object.NewEnvironment()
			e.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

			mainFunc, ok := env.Get("main")
			if !ok {
				return fmt.Errorf("function 'main' not found")
			}
			e.Apply(ctx, mainFunc, []object.Object{}, pkg)

			if assertedValue == nil {
				t.Fatal("assertType intrinsic was not called")
			}

			typeInfo := assertedValue.TypeInfo()
			if typeInfo == nil {
				t.Fatal("value passed to assertType has no type info")
			}
			if want, got := "main.MyString", fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name); want != got {
				t.Errorf("want type %q, but got %q", want, got)
			}
			return nil
		}

		_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action)
		if err != nil {
			t.Fatalf("scantest.Run failed: %+v", err)
		}
	})

	t.Run("comma ok", func(t *testing.T) {
		source := `
package main
type Stringer interface {
	String() string
}
type MyString string
func (s MyString) String() string { return string(s) }
func assertType(s MyString, ok bool) {}

func main() {
	var i Stringer = MyString("hello")
	s, ok := i.(MyString)
	assertType(s, ok)
}
`
		files := map[string]string{
			"main.go": source,
			"go.mod":  "module main",
		}
		dir, cleanup := scantest.WriteFiles(t, files)
		defer cleanup()

		var assertedS, assertedOK object.Object
		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			e := New(s, nil, nil, nil)

			e.RegisterIntrinsic("assertType", func(args ...object.Object) object.Object {
				if len(args) > 1 {
					assertedS = args[0]
					assertedOK = args[1]
				}
				return nil
			})

			env := object.NewEnvironment()
			e.Eval(ctx, pkg.AstFiles[pkg.Files[0]], env, pkg)

			mainFunc, ok := env.Get("main")
			if !ok {
				return fmt.Errorf("function 'main' not found")
			}
			e.Apply(ctx, mainFunc, []object.Object{}, pkg)

			if assertedS == nil {
				t.Fatal("assertType intrinsic was not called with s")
			}
			if assertedOK == nil {
				t.Fatal("assertType intrinsic was not called with ok")
			}

			// check 's'
			typeInfo := assertedS.TypeInfo()
			if typeInfo == nil {
				t.Fatal("value of 's' has no type info")
			}
			if want, got := "main.MyString", fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name); want != got {
				t.Errorf("want type %q, but got %q", want, got)
			}

			// check 'ok'
			if _, ok := assertedOK.(*object.Boolean); !ok {
				if _, ok := assertedOK.(*object.SymbolicPlaceholder); !ok {
					t.Errorf("want 'ok' to be a Boolean or SymbolicPlaceholder, but got %T", assertedOK)
				}
			}
			return nil
		}

		_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action)
		if err != nil {
			t.Fatalf("scantest.Run failed: %+v", err)
		}
	})
}
