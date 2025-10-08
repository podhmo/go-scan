package symgo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestNewInterpreter(t *testing.T) {
	t.Run("nil scanner", func(t *testing.T) {
		_, err := symgo.NewInterpreter(nil)
		if err == nil {
			t.Error("expected an error for nil scanner, but got nil")
		}
	})

	t.Run("success", func(t *testing.T) {
		s, err := goscan.New() // a scanner with no options is valid
		if err != nil {
			t.Fatalf("goscan.New() failed: %+v", err)
		}

		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			t.Errorf("NewInterpreter() failed: %+v", err)
		}
		if interp == nil {
			t.Error("expected interpreter to be non-nil")
		}
	})
}

func TestRecursion_PackageLoading(t *testing.T) {
	// This test sets up a project with a circular dependency between two packages,
	// `a` and `b`. Previously, this would cause the symgo engine to hang due to
	// infinite recursion during package loading. This test ensures that the
	// recursion guard is working correctly, allowing the analysis to complete
	// without error.
	source := map[string]string{
		"go.mod": `
module example.com/m

require (
	example.com/a v0.0.0
	example.com/b v0.0.0
)

replace (
	example.com/a => ./a
	example.com/b => ./b
)
`,
		"a/a.go": `
package a
import "example.com/b"
func A() {
	b.B()
}`,
		"b/b.go": `
package b
import "example.com/a"
func B() {
	a.A()
}`,
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/m/a.A",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest: expected no error, but got: %v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestBuiltinLen_OnUnresolvedFunction(t *testing.T) {
	source := `
package main
import "os"

func main() {
	// This tests a specific scenario where a package-level variable (os.Args)
	// from an unscanned package might be incorrectly resolved as an
	// UnresolvedFunction. The len() intrinsic should be robust enough to
	// handle this without crashing.
	_ = len(os.Args)
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		// The main check is that it doesn't panic. The result of len() on
		// such an object is a symbolic placeholder, which is fine.
	}

	symgotest.Run(t, tc, action)
}

func TestBuiltinNew_OnUnresolvedFunction(t *testing.T) {
	source := `
package main
import "net/http"

func main() {
	_ = new(http.HandlerFunc)
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
		Options:    []symgotest.Option{
			// We explicitly do NOT include net/http in the scan policy
			// to ensure http.HandlerFunc is an unresolved type.
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		// The main check is that execution doesn't fail with an
		// "invalid argument for new" error.
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestBuiltinLen_OnFunctionResult(t *testing.T) {
	source := `
package main

func getSlice() []string {
	return []string{"a", "b", "c"}
}

func main() int {
	return len(getSlice())
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		got, ok := r.ReturnValue.(*object.Integer)
		if !ok {
			t.Fatalf("Expected return value to be an *object.Integer, but got %T", r.ReturnValue)
		}
		if got.Value != 3 {
			t.Errorf("Expected len to be 3, but got %d", got.Value)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestInterpreter_Eval_Simple(t *testing.T) {
	source := `
package main
import "fmt"
func GetExpr() any {
	return fmt.Println
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.GetExpr",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		_, ok := r.ReturnValue.(*object.UnresolvedFunction)
		if !ok {
			t.Errorf("Expected an UnresolvedFunction for an external function, but got %T", r.ReturnValue)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestInterpreter_RegisterIntrinsic(t *testing.T) {
	source := `
package main
import "fmt"
func CallIntrinsic() any {
	return fmt.Println("hello")
}
`
	expectedResult := &object.String{Value: "Intrinsic was called!"}
	handler := func(ctx context.Context, interp *symgo.Interpreter, args []object.Object) object.Object {
		return expectedResult
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.CallIntrinsic",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("fmt.Println", handler),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		if diff := cmp.Diff(expectedResult, r.ReturnValue); diff != "" {
			t.Errorf("result mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestBlockStatement_executesAllStatements(t *testing.T) {
	source := `
package main

func log(msg string) string {
	return msg
}

func main() {
	log("call 1")
	log("call 2")
	log("call 3")
}
`
	var callLog []string
	logIntrinsic := func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
		if len(args) > 0 {
			if str, ok := args[0].(*object.String); ok {
				callLog = append(callLog, str.Value)
			}
		}
		return &object.String{Value: "dummy return"}
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("mymodule.log", logIntrinsic),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		expected := []string{"call 1", "call 2", "call 3"}
		if diff := cmp.Diff(expected, callLog); diff != "" {
			t.Errorf("call log mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestNakedReturn_AssignedToVar(t *testing.T) {
	source := `
package main

func myFunc() *int {
	return
}

func main() {
	x := myFunc()
	_ = x
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}
		// The main check is that it doesn't panic.
	}

	symgotest.Run(t, tc, action)
}

func TestEntryPoint_WithMissingArguments(t *testing.T) {
	source := `
package main

type MyInterface interface {
	DoSomething() string
}

func MyFunction(p MyInterface) {
	if p == nil {
		return
	}
	_ = p.DoSomething()
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.MyFunction",
		Args:       []symgo.Object{}, // Explicitly pass no arguments
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Apply() failed with unexpected error: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestEntryPoint_VariadicInterface(t *testing.T) {
	source := `
package main

import "log"

func MyLogf(f string, args ...interface{}) {
	if f == "" {
		log.Println("f is empty")
	}
	_ = args
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.MyLogf",
		Args: []symgo.Object{
			&object.String{Value: "hello"},
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Apply() failed with unexpected error: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestEntryPoint_Variadic_WithMissingArguments(t *testing.T) {
	source := `
package main

func MyVariadicFunc(a, b int, c ...string) {
	_ = a
	_ = b
	_ = c
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module mymodule",
			"main.go": source,
		},
		EntryPoint: "mymodule.MyVariadicFunc",
		Args: []symgo.Object{
			&object.Integer{Value: 1},
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		// This should not fail. The interpreter should create placeholders for missing args.
		if r.Error != nil {
			t.Fatalf("Apply() failed with unexpected error: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}
