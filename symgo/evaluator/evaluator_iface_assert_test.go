package evaluator_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestTypeAssertion_PointerToFuncType(t *testing.T) {
	source := `
package main

type MyFunc func()

func (f *MyFunc) WithReceiver(r interface{}) *MyFunc {
	return f
}

func Get() interface{} {
	var f MyFunc = func() {}
	return &f
}

func main() {
	f, ok := Get().(*MyFunc)
	if !ok {
		return
	}
	f.WithReceiver(nil)
	inspect(f)
}
`
	var got object.Object
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module main",
			"main.go": source,
		},
		EntryPoint: "main.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("main.inspect", func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
				got = args[0]
				return nil
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run() failed unexpectedly: %+v", r.Error)
		}

		if got == nil {
			t.Fatal("inspect was not called")
		}

		ptr, ok := got.(*object.Pointer)
		if !ok {
			t.Fatalf("expected object to be a Pointer, but got %T", got)
		}

		fn, ok := ptr.Value.(*object.Function)
		if !ok {
			t.Fatalf("expected pointer value to be a Function, but got %T", fn)
		}

		if fn.TypeInfo() == nil || fn.TypeInfo().Name != "MyFunc" {
			t.Errorf("expected function to have TypeInfo for 'MyFunc', but got %v", fn.TypeInfo())
		}
	}

	symgotest.Run(t, tc, action)
}