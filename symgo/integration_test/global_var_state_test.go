package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestStateTracking_GlobalVarWithMethodCall(t *testing.T) {
	source := `
package main

type MyType struct {}

func NewMyType() *MyType {
	return &MyType{}
}

func (t *MyType) DoSomething() {}

var instance = NewMyType()

func init() {
	instance.DoSomething()
}

func main() {
	// main is empty, the key action is in init
}
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/hello\ngo 1.21\n",
		"main.go": source,
	})
	defer cleanup()

	var calledMethods []string
	var mu sync.Mutex

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		// Register an intrinsic to record all function/method calls
		interp.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			fn, ok := args[0].(*object.Function)
			if !ok {
				return nil
			}

			mu.Lock()
			defer mu.Unlock()
			if fn.Receiver != nil {
				recvType := fn.Receiver.TypeInfo()
				if recvType != nil {
					calledMethods = append(calledMethods, recvType.Name+"."+fn.Name.Name)
				}
			} else if fn.Name != nil {
				calledMethods = append(calledMethods, fn.Name.Name)
			}
			return nil
		})

		pkg := pkgs[0]

		// First, eval the whole package. This should process the global var
		// and also define the init and main functions.
		if _, err := interp.Eval(ctx, pkg.AstFiles[pkg.Files[0]], pkg); err != nil {
			t.Fatalf("Eval package failed: %v", err)
		}

		// Now, find and run the init function.
		initFuncObj, ok := interp.FindObject("init")
		if !ok {
			t.Fatal("could not find init function object")
		}
		initFunc, ok := initFuncObj.(*object.Function)
		if !ok {
			t.Fatal("init is not a function")
		}

		// Run the symbolic execution starting from init()
		if _, err := interp.Apply(ctx, initFunc, nil, pkg); err != nil {
			return err
		}

		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	// Verify that the method called in init() was correctly traced.
	foundDoSomething := false
	for _, name := range calledMethods {
		if strings.HasSuffix(name, "MyType.DoSomething") {
			foundDoSomething = true
			break
		}
	}

	if !foundDoSomething {
		t.Errorf("Expected method 'DoSomething' to be called, but it was not. Called functions: %v", calledMethods)
	}
}
