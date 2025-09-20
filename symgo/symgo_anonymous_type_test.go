package symgo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgotest"
)

const anonymousTypesSource = `
package main

type ObjectId interface {
	Hex() string
}

func AnonymousInterface(id interface {
	Hex() string
}) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

func AnonymousStruct(p struct {
	X, Y int
}) int {
	return p.X
}
`

func TestAnonymousTypes_Interface(t *testing.T) {
	var inspectedMethod *scanner.FunctionInfo

	defaultIntrinsic := func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		fn := args[0] // The function object itself
		var foundFunc *scanner.FunctionInfo
		switch f := fn.(type) {
		case *symgo.Function:
			foundFunc = f.Def
		case *symgo.SymbolicPlaceholder:
			if f.UnderlyingFunc != nil {
				foundFunc = f.UnderlyingFunc
			}
		}

		if foundFunc != nil {
			inspectedMethod = foundFunc
		}
		return &symgo.SymbolicPlaceholder{Reason: "default intrinsic result"}
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": anonymousTypesSource,
		},
		EntryPoint: "mymodule.AnonymousInterface",
		Args:       []symgo.Object{&symgo.SymbolicPlaceholder{Reason: "test"}},
		Options: []symgotest.Option{
			symgotest.WithDefaultIntrinsic(defaultIntrinsic),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		if inspectedMethod == nil {
			t.Fatal("did not capture an interface method call, UnderlyingFunc was nil")
		}
		if inspectedMethod.Name != "Hex" {
			t.Errorf("expected to capture method 'Hex', but got '%s'", inspectedMethod.Name)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestAnonymousTypes_Struct(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": anonymousTypesSource,
		},
		EntryPoint: "mymodule.AnonymousStruct",
		Args:       []symgo.Object{&symgo.SymbolicPlaceholder{Reason: "test"}},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		placeholder, ok := r.ReturnValue.(*symgo.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("expected symbolic placeholder return, got %T", r.ReturnValue)
		}

		if !strings.Contains(placeholder.Reason, "field access") {
			t.Errorf("expected reason to contain 'field access', but got %q", placeholder.Reason)
		}
	}

	symgotest.Run(t, tc, action)
}
