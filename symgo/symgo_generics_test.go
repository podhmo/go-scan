package symgo_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestGenericFunctionCall(t *testing.T) {
	source := `
package main

var V int

func identity[T any](v T) T {
	return v
}

func main() {
	V = identity[int](42)
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
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		vObj, ok := r.Interpreter.FindObjectInPackage(t.Context(), "mymodule", "V")
		if !ok {
			t.Fatalf("global variable V not found")
		}
		vVar, ok := vObj.(*object.Variable)
		if !ok {
			t.Fatalf("V is not a variable, got %T", vObj)
		}

		want := &object.Integer{Value: 42}
		if diff := cmp.Diff(want, vVar.Value, cmp.AllowUnexported(object.Integer{})); diff != "" {
			t.Errorf("V mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericCallWithInterfaceUnionConstraint(t *testing.T) {
	source := `
package main

// Target: symgo should be able to understand that T can be *Foo or *Bar.

type Foo struct {
	Name string
}
type Bar struct {
	ID int
}

type Loginable interface {
	*Foo | *Bar
}

func main() {
	// We don't need to call anything, just parse the types.
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
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		pkgObj, ok := r.Interpreter.FindPackage(t.Context(), "mymodule")
		if !ok {
			t.Fatal("could not find package mymodule")
		}
		pkgInfo := pkgObj.Info()
		if pkgInfo == nil {
			t.Fatal("PackageInfo is nil")
		}

		loginableType := pkgInfo.Lookup("Loginable")
		if loginableType == nil {
			t.Fatal("could not find type Loginable")
		}

		if loginableType.Interface == nil {
			t.Fatal("Loginable is not an interface")
		}

		// This is the core of the test.
		// Before the fix, the parser sees `*Foo | *Bar` as a single invalid expression.
		// So, `Embedded` will have one element.
		// After the fix, it should correctly parse two distinct types.
		if len(loginableType.Interface.Embedded) != 2 {
			t.Errorf("expected interface to have 2 embedded types, but got %d", len(loginableType.Interface.Embedded))
			for i, emb := range loginableType.Interface.Embedded {
				t.Logf("  [%d]: %s", i, emb.String())
			}
		}

		// For now, let's also check the failing case to be sure.
		if len(loginableType.Interface.Embedded) == 1 {
			t.Log("Test correctly detected 1 embedded type (the expected pre-fix state)")
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericCallWithInterfaceConstraint(t *testing.T) {
	source := `
package main

import "fmt"

func PrintStringer[T fmt.Stringer](v T) {
	// We don't need to evaluate the body for this test,
	// just ensure the call doesn't crash.
}

type MyString string

func (s MyString) String() string {
	return string(s)
}

func main() {
	var s MyString = "hello"
	PrintStringer[MyString](s)
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
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericTypeDefinition(t *testing.T) {
	source := `
package main

type MySlice[T any] struct {
	Data []T
}

var V MySlice[int]

func main() {
	V = MySlice[int]{Data: []int{10, 20}}
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
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		vObj, ok := r.Interpreter.FindObjectInPackage(t.Context(), "mymodule", "V")
		if !ok {
			t.Fatalf("global variable V not found")
		}
		vVar, ok := vObj.(*object.Variable)
		if !ok {
			t.Fatalf("V is not a variable, got %T", vObj)
		}

		instance, ok := vVar.Value.(*object.Instance)
		if !ok {
			t.Fatalf("V is not an instance, got %T: %s", vVar.Value, vVar.Value.Inspect())
		}

		expectedTypeNamePrefix := "mymodule.MySlice"
		if !strings.HasPrefix(instance.TypeName, expectedTypeNamePrefix) {
			t.Errorf("expected V to be of type %q, but got %q", expectedTypeNamePrefix, instance.TypeName)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestGenericCallWithOmittedArgs(t *testing.T) {
	source := `
package main

var V int

func identity[T any](v T) T {
	return v
}

func main() {
	// This call omits the type argument, relying on type inference.
	// The evaluator should not crash, but can return a symbolic value.
	V = identity(42)
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
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}

		vObj, ok := r.Interpreter.FindObjectInPackage(t.Context(), "mymodule", "V")
		if !ok {
			t.Fatalf("global variable V not found")
		}
		vVar, ok := vObj.(*object.Variable)
		if !ok {
			t.Fatalf("V is not a variable, got %T", vObj)
		}

		want := &object.Integer{Value: 42}
		if diff := cmp.Diff(want, vVar.Value, cmp.AllowUnexported(object.Integer{})); diff != "" {
			t.Errorf("V mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}
