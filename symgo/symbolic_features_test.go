package symgo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymbolic_IfElse(t *testing.T) {
	var ifCalled, elseCalled bool

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main

func IfBlock() {}
func ElseBlock() {}

func main() {
	x := 1 // In symbolic execution, this value doesn't matter.
	if x > 0 {
		IfBlock()
	} else {
		ElseBlock()
	}
}`,
		},
		EntryPoint: "example.com/me.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("example.com/me.IfBlock", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				ifCalled = true
				return nil
			}),
			symgotest.WithIntrinsic("example.com/me.ElseBlock", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				elseCalled = true
				return nil
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		if !ifCalled {
			t.Error("if block was not called")
		}
		if !elseCalled {
			t.Error("else block was not called")
		}
	}

	symgotest.Run(t, tc, action)
}

func TestSymbolic_For(t *testing.T) {
	var callCount int

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main

func ForBody() {}

func main() {
	// Symbolic execution should unroll this once.
	for i := 0; i < 10; i++ {
		ForBody()
	}
}`,
		},
		EntryPoint: "example.com/me.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("example.com/me.ForBody", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				callCount++
				return nil
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		if callCount != 1 {
			t.Errorf("for loop body should be called once in symbolic execution, but was called %d times", callCount)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestSymbolic_Switch(t *testing.T) {
	var aCalled, bCalled, defaultCalled bool

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main

func CaseA() {}
func CaseB() {}
func DefaultCase() {}

func main() {
	v := "a" // This value doesn't matter for symbolic execution.
	switch v {
	case "a":
		CaseA()
	case "b":
		CaseB()
	default:
		DefaultCase()
	}
}`,
		},
		EntryPoint: "example.com/me.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("example.com/me.CaseA", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				aCalled = true
				return nil
			}),
			symgotest.WithIntrinsic("example.com/me.CaseB", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				bCalled = true
				return nil
			}),
			symgotest.WithIntrinsic("example.com/me.DefaultCase", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				defaultCalled = true
				return nil
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		if !aCalled {
			t.Error("case 'a' was not called")
		}
		if !bCalled {
			t.Error("case 'b' was not called")
		}
		if !defaultCalled {
			t.Error("default case was not called")
		}
	}

	symgotest.Run(t, tc, action)
}
