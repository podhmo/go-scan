package symgo_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestMethodDispatch(t *testing.T) {
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

	overlay := scanner.Overlay{
		"main.go": []byte(source),
	}
	s, err := goscan.New(goscan.WithOverlay(overlay), goscan.WithIncludeTests(true))
	if err != nil {
		t.Fatalf("goscan.New failed: %v", err)
	}

	innerScanner, err := s.ScannerForSymgo()
	if err != nil {
		t.Fatalf("getting inner scanner failed: %v", err)
	}
	interp, err := symgo.NewInterpreter(innerScanner)
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}

	var calledMethods []string
	var mu sync.Mutex

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

	ctx := context.Background()
	pkgs, err := s.Scan("./...")
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("Scan failed: %v", err)
	}
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

	interp.Apply(ctx, mainFunc, nil, pkg)

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

	found = false
	for _, name := range calledMethods {
		if name == "NewGreeter" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected function 'NewGreeter' to be called, but it was not. Called functions: %v", calledMethods)
	}
}
