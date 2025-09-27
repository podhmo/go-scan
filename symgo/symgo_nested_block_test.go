package symgo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestNestedBlockCallIsTracked(t *testing.T) {
	source := map[string]string{
		"go.mod": "module t",
		"main.go": `
package main

import "t/helpers"

func run() {
	{
		helpers.DoSomething()
	}
}
`,
		"helpers/helpers.go": `
package helpers

func DoSomething() {}
`,
	}

	var calledFunctions []string
	intrinsic := symgotest.WithDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, v []object.Object) object.Object {
		if len(v) == 0 {
			return nil
		}
		var fullName string
		if fn, ok := v[0].(*object.Function); ok && fn.Def != nil && fn.Package != nil {
			fullName = fn.Package.ImportPath + "." + fn.Def.Name
		} else if sp, ok := v[0].(*object.SymbolicPlaceholder); ok && sp.UnderlyingFunc != nil && sp.Package != nil {
			fullName = sp.Package.ImportPath + "." + sp.UnderlyingFunc.Name
		}
		if fullName != "" {
			calledFunctions = append(calledFunctions, fullName)
		}
		return nil
	})

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "t.run",
		Options:    []symgotest.Option{intrinsic},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}

		var found bool
		for _, name := range calledFunctions {
			if strings.HasSuffix(name, "t/helpers.DoSomething") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected call to helpers.DoSomething to be tracked, but it wasn't. tracked calls: %v", calledFunctions)
		}
	})
}

func TestNestedBlockVariableScoping(t *testing.T) {
	source := map[string]string{
		"go.mod": "module t",
		"main.go": `
package main

func run() (int, int) {
	x := 1
	y := 1
	{
		x := 2 // shadow
		y = 2  // assign
	}
	return x, y
}
`,
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "t.run",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}

		x := symgotest.AssertAs[*object.Integer](r, t, 0)
		y := symgotest.AssertAs[*object.Integer](r, t, 1)

		// x should be 1 (shadowed variable is popped)
		// y should be 2 (assigned in inner scope)
		want := [2]int64{1, 2}
		got := [2]int64{x.Value, y.Value}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}
