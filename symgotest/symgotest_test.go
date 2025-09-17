package symgotest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestRun_Simple(t *testing.T) {
	tc := TestCase{
		Source: map[string]string{
			"go.mod": "module example.com",
			"me/me.go": `
package me
type User struct { Name string }
func NewUser(name string) *User {
	return &User{Name: name}
}
`,
		},
		EntryPoint: "example.com/me.NewUser",
		Args:       []object.Object{object.NewString("Alice")},
	}

	action := func(t *testing.T, r *Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}

		ptr := AssertAs[*object.Pointer](r, t, 0)
		instance := AssertAs[*object.Instance](&Result{ReturnValue: ptr.Value}, t, 0)

		expectedTypeName := "example.com/me.User"
		if diff := cmp.Diff(expectedTypeName, instance.TypeName); diff != "" {
			t.Errorf("instance type name mismatch (-want +got):\n%s", diff)
		}
	}

	Run(t, tc, action)
}

func TestRun_MaxStepsExceeded(t *testing.T) {
	var body strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&body, "func f%d() { f%d() }\n", i, i+1)
	}
	fmt.Fprintln(&body, "func f21() {}")

	source := `package me
` + body.String()

	tc := TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/me",
			"main.go": source,
		},
		EntryPoint: "example.com/me.f1",
		Options: []Option{
			WithMaxSteps(10),
		},
	}

	res := runLogic(t, tc)
	if res.Error == nil {
		t.Fatalf("expected runLogic to fail, but it succeeded")
	}

	if !strings.Contains(res.Error.Message, "max execution steps (10) exceeded") {
		t.Errorf("expected error to contain 'max execution steps (10) exceeded', but got: %v", res.Error)
	}

	if res == nil {
		t.Fatalf("expected a non-nil result with trace info, but got nil")
	}
	if res.Trace == nil {
		t.Fatalf("expected result to have a trace, but it was nil")
	}
	if len(res.Trace.Events) < 1 {
		t.Errorf("expected trace to have captured events, but it was empty")
	}
}

func TestRunExpression(t *testing.T) {
	action := func(t *testing.T, r *Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		integerObj := AssertAs[*object.Integer](r, t, 0)
		if integerObj.Value != 3 {
			t.Errorf("expected 3, got %d", integerObj.Value)
		}
	}
	RunExpression(t, "1 + 2", action)
}

func TestRunStatements(t *testing.T) {
	action := func(t *testing.T, r *Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		val, ok := r.FinalEnv.Get("x")
		if !ok {
			t.Fatalf("variable 'x' not found in final environment")
		}

		variable := AssertAs[*object.Variable](&Result{ReturnValue: val}, t, 0)
		integerObj := AssertAs[*object.Integer](&Result{ReturnValue: variable.Value}, t, 0)
		if integerObj.Value != 10 {
			t.Errorf("expected 10, got %d", integerObj.Value)
		}
	}
	RunStatements(t, "x := 10", action)
}

func TestRun_ExpectError_FromIntrinsic(t *testing.T) {
	tc := TestCase{
		Source: map[string]string{
			"go.mod": "module example.com",
			"main.go": `
package main

// This function will be replaced by an intrinsic that returns an error.
func customErrorFunc() {}

func main() {
	customErrorFunc()
}
`,
		},
		EntryPoint:  "example.com.main",
		ExpectError: true,
		Options: []Option{
			WithIntrinsic("example.com.customErrorFunc", func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
				return &object.Error{Message: "this is a forced error"}
			}),
		},
	}

	action := func(t *testing.T, r *Result) {
		if r.Error == nil {
			t.Fatalf("expected an error, but got nil")
		}
		if !strings.Contains(r.Error.Message, "this is a forced error") {
			t.Errorf("expected error message to contain 'this is a forced error', but got %q", r.Error.Message)
		}
	}

	Run(t, tc, action)
}
