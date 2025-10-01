package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestStateTracking_GlobalVarWithMethodCall(t *testing.T) {
	var calledMethods []string
	var mu sync.Mutex

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/hello\ngo 1.21\n",
			"main.go": `
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
`,
		},
		EntryPoint: "example.com/hello.init",
		Options: []symgotest.Option{
			symgotest.WithDefaultIntrinsic(func(ctx context.Context, i *symgo.Interpreter, args []object.Object) object.Object {
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
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %v", r.Error)
		}

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

	symgotest.Run(t, tc, action)
}
