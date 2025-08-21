package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/resolver"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestTypeInfoPropagation(t *testing.T) {
	source := `
package main

type User struct {
	ID   int
	Name string
}

func inspect_type(u *User) {
}

func main() {
	var user User
	inspect_type(&user)
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		pkg := pkgs[0]

		r := resolver.New(s)
		eval := New(r, s, s.Logger)

		var inspectedType object.Object
		env := object.NewEnvironment()
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		intrinsic := &object.Intrinsic{
			Fn: func(args ...object.Object) object.Object {
				if len(args) > 0 {
					inspectedType = args[0]
				}
				return nil
			},
		}
		env.Set("inspect_type", intrinsic)

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not an object.Function, got %T", mainFuncObj)
		}

		// We use applyFunction directly to simulate a call to main()
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		if inspectedType == nil {
			t.Fatal("intrinsic was not called")
		}
		typeInfo := inspectedType.TypeInfo()
		if typeInfo == nil {
			return fmt.Errorf("TypeInfo() on the received object is nil")
		}
		if typeInfo.Name != "User" {
			return fmt.Errorf("expected type name to be 'User', but got %q", typeInfo.Name)
		}
		if typeInfo.Struct == nil {
			return fmt.Errorf("expected type to have struct info, but it was nil")
		}
		if len(typeInfo.Struct.Fields) != 2 {
			return fmt.Errorf("expected struct to have 2 fields, but got %d", len(typeInfo.Struct.Fields))
		}
		return nil
	}

	_, err := scantest.Run(t, dir, []string{"."}, action)
	if err != nil {
		t.Fatal(err)
	}
}
