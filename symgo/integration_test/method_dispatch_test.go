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

func TestMethodDispatchWithScantest(t *testing.T) {
	source := `
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
		var mainFunc *object.Function
		for _, fnInfo := range pkg.Functions {
			if fnInfo.Name == "main" {
				fileAst, ok := pkg.AstFiles[fnInfo.FilePath]
				if !ok {
					t.Fatalf("could not find AST file %s for main function", fnInfo.FilePath)
				}
				_, err := interp.Eval(ctx, fileAst, pkg)
				if err != nil {
					t.Fatalf("Eval package failed: %v", err)
				}
				mainFuncObj, ok := interp.FindObject("main")
				if !ok {
					t.Fatal("could not find main function object")
				}
				mainFunc, ok = mainFuncObj.(*object.Function)
				if !ok {
					t.Fatal("main is not a function")
				}
				break
			}
		}

		if mainFunc == nil {
			t.Fatal("main function not found")
		}

		_, err = interp.Apply(ctx, mainFunc, nil, pkg)
		return err
	}

	_, err := scantest.Run(t, nil, dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}

	// Verify that the method was called
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
