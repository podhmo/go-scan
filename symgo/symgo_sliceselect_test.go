package symgo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSliceExpr(t *testing.T) {
	source := `
package main
var s []int
func getLow() int { return 0 }
func getHigh() int { return 10 }
func main() {
	_ = s[getLow():getHigh()]
}
`
	var calledFunctions []string
	intrinsic := symgotest.WithDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) > 0 {
			if fn, ok := args[0].(*object.Function); ok {
				if fn.Def != nil {
					calledFunctions = append(calledFunctions, fn.Def.Name)
				}
			}
		}
		return nil
	})

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/me",
			"main.go": source,
		},
		EntryPoint: "example.com/me.main",
		Options:    []symgotest.Option{intrinsic},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run() failed: %+v", r.Error)
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
}

func TestSelectStmt(t *testing.T) {
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
	var calledFunctions []string
	intrinsic := symgotest.WithDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) > 0 {
			if fn, ok := args[0].(*object.Function); ok {
				if fn.Def != nil {
					calledFunctions = append(calledFunctions, fn.Def.Name)
				}
			}
		}
		return nil
	})

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/me",
			"main.go": source,
		},
		EntryPoint: "example.com/me.main",
		Options:    []symgotest.Option{intrinsic},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run() failed: %+v", r.Error)
		}

		calledMap := make(map[string]bool)
		for _, name := range calledFunctions {
			calledMap[name] = true
		}

		if !calledMap["getChan"] {
			t.Errorf("want 'getChan' to be called, but it wasn't. Called: %v", calledFunctions)
		}
		if !calledMap["handle"] {
			t.Errorf("want 'handle' to be called, but it wasn't. Called: %v", calledFunctions)
		}
	})
}
