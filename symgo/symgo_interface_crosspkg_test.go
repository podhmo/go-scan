package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

// TestMultiImplementationResolution verifies that a call on an interface
// is conservatively propagated to all known concrete implementations.
func TestMultiImplementationResolution(t *testing.T) {
	files := map[string]string{
		"go.mod": "module app\n\ngo 1.22",
		"iface/iface.go": `
package iface
type MultiGreeter interface {
	Greet()
}`,
		"impl/impl.go": `
package impl
type One struct{}
func (o One) Greet() {}
type Two struct{}
func (o Two) Greet() {}
`,
		"main/main.go": `
package main
import (
	"app/iface"
	"app/impl"
)
var G iface.MultiGreeter
func run() {
	// Assign one type, then another.
	G = impl.One{}
	G = impl.Two{}
	// Call the method.
	G.Greet()
}
`,
	}

	var oneGreetCalled, twoGreetCalled bool

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	_, err = scantest.Run(t, context.Background(), dir, []string{"./..."}, func(ctx context.Context, scanner *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}

		interp.RegisterIntrinsic("(app/impl.One).Greet", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			oneGreetCalled = true
			return nil
		})
		interp.RegisterIntrinsic("(app/impl.Two).Greet", func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			twoGreetCalled = true
			return nil
		})

		// Evaluate all packages. Order doesn't matter for this test's goal.
		for _, pkg := range pkgs {
			for _, astFile := range pkg.AstFiles {
				if _, err := interp.Eval(ctx, astFile, pkg); err != nil {
					return fmt.Errorf("symgo eval of %s failed: %+v", pkg.Name, err)
				}
			}
		}

		runFn, ok := interp.GlobalEnvForTest().Get("run")
		if !ok {
			return fmt.Errorf("function 'run' not found")
		}

		mainPkg := pkgs[0] // just need a valid package context
		for _, p := range pkgs {
			if p.Name == "main" {
				mainPkg = p
				break
			}
		}

		if _, err := interp.Apply(ctx, runFn, nil, mainPkg); err != nil {
			return fmt.Errorf("symgo apply of 'run' failed: %+v", err)
		}

		if !oneGreetCalled {
			return fmt.Errorf("expected (impl.One).Greet to be called, but it was not")
		}
		if !twoGreetCalled {
			return fmt.Errorf("expected (impl.Two).Greet to be called, but it was not")
		}
		return nil
	}, scantest.WithScanner(s))

	if err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}
}
