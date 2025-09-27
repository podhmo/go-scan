package integration_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestMethodDispatchWithScantest(t *testing.T) {
	var calledMethods []string
	var mu sync.Mutex

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/hello\ngo 1.21\n",
			"main.go": `
package main

type Greeter struct {
	prefix string
}

func NewGreeter(prefix string) *Greeter {
	return &Greeter{prefix: prefix}
}

func (g *Greeter) SayHello(name string) string {
	return g.prefix + name
}

func main() {
	g := NewGreeter("hello, ")
	g.SayHello("world")
}
`,
		},
		EntryPoint: "example.com/hello.main",
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

		found := false
		for _, name := range calledMethods {
			if strings.HasSuffix(name, "Greeter.SayHello") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected method 'SayHello' to be called, but it was not. Called functions: %v", calledMethods)
		}
	}

	symgotest.Run(t, tc, action)
}
