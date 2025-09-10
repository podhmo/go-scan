package symgo_test

import (
	"context"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
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
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(context.Background(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	_, err = interp.Eval(context.Background(), pkg.AstFiles[pkg.Files[0]], pkg)
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	mainFn, ok := interp.FindObject("main")
	if !ok {
		t.Fatalf("main function not found")
	}

	_, err = interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		t.Fatalf("Apply() failed: %+v", err)
	}

	// Now, check the value of the global variable V
	vObj, ok := interp.FindObject("V")
	if !ok {
		t.Fatalf("global variable V not found")
	}
	vVar, ok := vObj.(*object.Variable)
	if !ok {
		t.Fatalf("V is not a variable, got %T", vObj)
	}

	// Assert the value
	intVal, ok := vVar.Value.(*object.Integer)
	if !ok {
		t.Fatalf("V is not an integer, got %T: %s", vVar.Value, vVar.Value.Inspect())
	}
	if intVal.Value != 42 {
		t.Errorf("expected V to be 42, got %d", intVal.Value)
	}
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
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(context.Background(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	_, err = interp.Eval(context.Background(), pkg.AstFiles[pkg.Files[0]], pkg)
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	mainFn, ok := interp.FindObject("main")
	if !ok {
		t.Fatalf("main function not found")
	}

	// This apply call should not crash.
	_, err = interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		t.Fatalf("Apply() failed unexpectedly: %+v", err)
	}
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
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(context.Background(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	_, err = interp.Eval(context.Background(), pkg.AstFiles[pkg.Files[0]], pkg)
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	mainFn, ok := interp.FindObject("main")
	if !ok {
		t.Fatalf("main function not found")
	}

	_, err = interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		t.Fatalf("Apply() failed: %+v", err)
	}

	// Now, check the value of the global variable V
	vObj, ok := interp.FindObject("V")
	if !ok {
		t.Fatalf("global variable V not found")
	}
	vVar, ok := vObj.(*object.Variable)
	if !ok {
		t.Fatalf("V is not a variable, got %T", vObj)
	}

	// Assert the value
	instance, ok := vVar.Value.(*object.Instance)
	if !ok {
		t.Fatalf("V is not an instance, got %T: %s", vVar.Value, vVar.Value.Inspect())
	}

	// The type name check is tricky. For now, let's just check that it's an instance.
	// A more robust check would be needed once the implementation is in place.
	expectedTypeNamePrefix := "mymodule.MySlice"
	if !strings.HasPrefix(instance.TypeName, expectedTypeNamePrefix) {
		t.Errorf("expected V to be of type %q, but got %q", expectedTypeNamePrefix, instance.TypeName)
	}
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
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	pkgs, err := s.Scan(context.Background(), ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %+v", err)
	}
	pkg := pkgs[0]

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	_, err = interp.Eval(context.Background(), pkg.AstFiles[pkg.Files[0]], pkg)
	if err != nil {
		t.Fatalf("Eval() failed: %+v", err)
	}

	mainFn, ok := interp.FindObject("main")
	if !ok {
		t.Fatalf("main function not found")
	}

	_, err = interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		t.Fatalf("Apply() failed: %+v", err)
	}

	// The evaluator is smart enough to handle this simple case without full
	// type inference. It passes the argument through. Let's assert this correct behavior.
	vObj, ok := interp.FindObject("V")
	if !ok {
		t.Fatalf("global variable V not found")
	}
	vVar, ok := vObj.(*object.Variable)
	if !ok {
		t.Fatalf("V is not a variable, got %T", vObj)
	}

	// Assert the value
	intVal, ok := vVar.Value.(*object.Integer)
	if !ok {
		t.Fatalf("V is not an integer, got %T: %s", vVar.Value, vVar.Value.Inspect())
	}
	if intVal.Value != 42 {
		t.Errorf("expected V to be 42, got %d", intVal.Value)
	}
}
