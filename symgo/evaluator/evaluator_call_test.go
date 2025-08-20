package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"log/slog"
	"os"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalCallExprOnFunction_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
func add(a, b int) int { return a + b }
func main() { add(1, 2) }
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, but got %d", len(pkgs))
		}
		pkg := pkgs[0]

		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return fmt.Errorf("ScannerForSymgo() failed: %w", err)
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, ok := env.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not an object.Function, got %T", mainFuncObj)
		}

		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalCallExprOnIntrinsic_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
import "fmt"
func main() { fmt.Println("hello") }
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var got string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		eval.RegisterIntrinsic("fmt.Println", func(args ...object.Object) object.Object {
			if len(args) > 0 {
				if s, ok := args[0].(*object.String); ok {
					got = s.Value
				}
			}
			return &object.SymbolicPlaceholder{}
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if want := "hello"; got != want {
		t.Errorf("intrinsic not called correctly, want %q, got %q", want, got)
	}
}

func TestEvalCallExprOnInstanceMethod_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
import "net/http"
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", nil)
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var gotPattern string
	handleFuncCalled := false

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		internalScanner, err := s.ScannerForSymgo()
		if err != nil {
			return err
		}
		eval := New(internalScanner, s.Logger)
		env := object.NewEnvironment()

		const serveMuxTypeName = "net/http.ServeMux"
		eval.RegisterIntrinsic("net/http.NewServeMux", func(args ...object.Object) object.Object {
			// Create a fake TypeInfo for ServeMux. In unit tests, we can't easily
			// scan external packages. This provides the minimum information needed
			// by the evaluator to resolve method calls on the variable.
			fakeServeMuxTypeInfo := &goscan.TypeInfo{
				Name:    "ServeMux",
				PkgPath: "net/http",
				Kind:    goscan.StructKind,
			}
			return &object.Instance{
				TypeName:   serveMuxTypeName,
				BaseObject: object.BaseObject{ResolvedTypeInfo: fakeServeMuxTypeInfo},
			}
		})

		eval.RegisterIntrinsic(fmt.Sprintf("(*%s).HandleFunc", serveMuxTypeName), func(args ...object.Object) object.Object {
			handleFuncCalled = true
			if len(args) > 1 { // self, pattern, handler
				if s, ok := args[1].(*object.String); ok {
					gotPattern = s.Value
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, env, pkg)
		}

		mainFuncObj, _ := env.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if !handleFuncCalled {
		t.Fatal("HandleFunc intrinsic was not called")
	}
	if want := "/"; gotPattern != want {
		t.Errorf("HandleFunc pattern wrong, want %q, got %q", want, gotPattern)
	}
}

func TestEvalCallExpr_VariousPatterns(t *testing.T) {
	t.Run("method call on a struct literal", func(t *testing.T) {
		files := map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `
package main
type S struct{}
func (s S) Do() {}
func main() {
	S{}.Do()
}`,
		}
		dir, cleanup := scantest.WriteFiles(t, files)
		defer cleanup()

		var doCalled bool
		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			internalScanner, err := s.ScannerForSymgo()
			if err != nil {
				return err
			}
			handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
			logger := slog.New(handler)
			eval := New(internalScanner, logger)
			env := object.NewEnvironment()

			key := fmt.Sprintf("(%s.S).Do", pkg.ImportPath)
			eval.RegisterIntrinsic(key, func(args ...object.Object) object.Object {
				doCalled = true
				return nil
			})

			for _, file := range pkg.AstFiles {
				eval.Eval(ctx, file, env, pkg)
			}

			mainFuncObj, _ := env.Get("main")
			mainFunc := mainFuncObj.(*object.Function)
			eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
			return nil
		}

		if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}
		if !doCalled {
			t.Error("Do method was not called")
		}
	})

	t.Run("method chaining", func(t *testing.T) {
		files := map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `
package main
import "fmt"
type Greeter struct { name string }
func NewGreeter(name string) *Greeter { return &Greeter{name: name} }
func (g *Greeter) Greet() string { return "Hello, " + g.name }
func (g *Greeter) WithName(name string) *Greeter { g.name = name; return g }
func main() {
	v := NewGreeter("world").WithName("gopher").Greet()
	fmt.Println(v)
}`,
		}
		dir, cleanup := scantest.WriteFiles(t, files)
		defer cleanup()

		var greetCalled bool
		var finalName string
		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			internalScanner, err := s.ScannerForSymgo()
			if err != nil {
				return err
			}
			handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
			logger := slog.New(handler)
			eval := New(internalScanner, logger)
			env := object.NewEnvironment()

			greeterTypeName := fmt.Sprintf("%s.Greeter", pkg.ImportPath)
			eval.RegisterIntrinsic(fmt.Sprintf("%s.NewGreeter", pkg.ImportPath), func(args ...object.Object) object.Object {
				name := args[0].(*object.String).Value
				return &object.Instance{TypeName: fmt.Sprintf("*%s", greeterTypeName), State: map[string]object.Object{"name": &object.String{Value: name}}}
			})
			eval.RegisterIntrinsic(fmt.Sprintf("(*%s).WithName", greeterTypeName), func(args ...object.Object) object.Object {
				g := args[0].(*object.Instance)
				name := args[1].(*object.String).Value
				if g.State == nil {
					g.State = make(map[string]object.Object)
				}
				g.State["name"] = &object.String{Value: name}
				return g
			})
			eval.RegisterIntrinsic(fmt.Sprintf("(*%s).Greet", greeterTypeName), func(args ...object.Object) object.Object {
				greetCalled = true
				g := args[0].(*object.Instance)
				name, _ := g.State["name"].(*object.String)
				finalName = name.Value
				return &object.String{Value: "Hello, " + finalName}
			})
			eval.RegisterIntrinsic("fmt.Println", func(args ...object.Object) object.Object {
				return nil
			})

			for _, file := range pkg.AstFiles {
				eval.Eval(ctx, file, env, pkg)
			}

			mainFuncObj, _ := env.Get("main")
			mainFunc := mainFuncObj.(*object.Function)
			eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
			return nil
		}

		if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}
		if !greetCalled {
			t.Error("Greet method was not called")
		}
		if want := "gopher"; finalName != want {
			t.Errorf("final name is wrong, want %q, got %q", want, finalName)
		}
	})

	t.Run("nested function calls", func(t *testing.T) {
		files := map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `
package main
func add(a, b int) int { return a + b }
func main() {
	add(add(1, 2), 3)
}`,
		}
		dir, cleanup := scantest.WriteFiles(t, files)
		defer cleanup()

		var callCount int
		var lastResult int
		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			pkg := pkgs[0]
			internalScanner, err := s.ScannerForSymgo()
			if err != nil {
				return err
			}
			eval := New(internalScanner, s.Logger)
			env := object.NewEnvironment()

			eval.RegisterIntrinsic(fmt.Sprintf("%s.add", pkg.ImportPath), func(args ...object.Object) object.Object {
				callCount++
				a := args[0].(*object.Integer).Value
				b := args[1].(*object.Integer).Value
				result := int(a + b)
				if callCount == 2 {
					lastResult = result
				}
				return &object.Integer{Value: int64(result)}
			})

			for _, file := range pkg.AstFiles {
				eval.Eval(ctx, file, env, pkg)
			}

			mainFuncObj, _ := env.Get("main")
			mainFunc := mainFuncObj.(*object.Function)
			eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
			return nil
		}

		if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}
		if callCount != 2 {
			t.Errorf("expected add to be called 2 times, but got %d", callCount)
		}
		if want := 6; lastResult != want {
			t.Errorf("final result is wrong, want %d, got %d", want, lastResult)
		}
	})
}
