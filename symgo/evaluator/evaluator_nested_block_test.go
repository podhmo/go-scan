package evaluator_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
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

	dir, cleanup := scantest.WriteFiles(t, source)
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

	var calledFunctions []string
	interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, v []object.Object) object.Object {
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

	// Evaluate the file to load symbols
	_, err = interp.Eval(context.Background(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Find and apply the main function
	mainFn, ok := interp.FindObject("run")
	if !ok {
		t.Fatal("could not find run function")
	}

	_, err = interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		t.Fatalf("Apply() failed: %+v", err)
	}

	// Verify that the call inside the nested block was tracked.
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

	dir, cleanup := scantest.WriteFiles(t, source)
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

	// Evaluate the file to load symbols
	_, err = interp.Eval(context.Background(), pkg.AstFiles[filepath.Join(dir, "main.go")], pkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	mainFn, ok := interp.FindObject("run")
	if !ok {
		t.Fatalf("run function not found")
	}

	ret, err := interp.Apply(context.Background(), mainFn, nil, pkg)
	if err != nil {
		t.Fatalf("Apply() failed: %v", err)
	}

	// The result from interp.Apply is the raw object from the evaluator, which is a ReturnValue.
	retVal, ok := ret.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected return value, got %T", ret)
	}
	multiRet, ok := retVal.Value.(*object.MultiReturn)
	if !ok {
		t.Fatalf("expected multi-return, got %T", retVal.Value)
	}

	if len(multiRet.Values) != 2 {
		t.Fatalf("expected 2 return values, got %d", len(multiRet.Values))
	}

	x, ok := multiRet.Values[0].(*object.Integer)
	if !ok {
		t.Fatalf("return 0 is not an integer, got %T", multiRet.Values[0])
	}
	y, ok := multiRet.Values[1].(*object.Integer)
	if !ok {
		t.Fatalf("return 1 is not an integer, got %T", multiRet.Values[1])
	}

	// x should be 1 (shadowed variable is popped)
	// y should be 2 (assigned in inner scope)
	want := [2]int64{1, 2}
	got := [2]int64{x.Value, y.Value}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}
