package symgo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestMapIndexAssignment(t *testing.T) {
	var getValueCalled bool
	source := `
package main

func getValue() string {
	return "world"
}

func main() {
	m := make(map[string]string)
	m["hello"] = getValue()
}`

	intrinsic := func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		getValueCalled = true
		return &object.String{Value: "world"}
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module myapp",
			"main.go": source,
		},
		EntryPoint: "myapp.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("myapp.getValue", intrinsic),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		if !getValueCalled {
			t.Errorf("intrinsic for getValue was not called")
		}
	}

	symgotest.Run(t, tc, action)
}

func TestSliceIndexAssignment(t *testing.T) {
	var getValueCalled bool
	source := `
package main

func getValue() string {
	return "world"
}

func main() {
	s := make([]string, 1)
	s[0] = getValue()
}`

	intrinsic := func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
		getValueCalled = true
		return &object.String{Value: "world"}
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module myapp",
			"main.go": source,
		},
		EntryPoint: "myapp.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("myapp.getValue", intrinsic),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		if !getValueCalled {
			t.Errorf("intrinsic for getValue was not called")
		}
	}

	symgotest.Run(t, tc, action)
}
