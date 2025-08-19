package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func mustParse(t *testing.T, source string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", source, 0)
	if err != nil {
		t.Fatalf("mustParse: %v", err)
	}
	return f
}

func findFunc(t *testing.T, pkg *goscan.Package, name string) *ast.FuncDecl {
	t.Helper()
	for _, f := range pkg.Functions {
		if f.Name == name {
			return f.AstDecl
		}
	}
	return nil
}

func TestEvalCallExprOnFunction(t *testing.T) {
	source := `
package main
func add(a, b int) int { return a + b }
func main() { add(1, 2) }
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir))
	if err != nil {
		t.Fatalf("NewScanner() failed: %v", err)
	}
	pkg, err := s.ScanPackage(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanPackage() failed: %v", err)
	}

	internalScanner, err := s.ScannerForSymgo()
	if err != nil {
		t.Fatalf("ScannerForSymgo() failed: %v", err)
	}
	eval := New(internalScanner, s.Logger)
	env := object.NewEnvironment()
	for _, file := range pkg.AstFiles {
		eval.Eval(file, env, pkg)
	}

	mainFuncObj, ok := env.Get("main")
	if !ok {
		t.Fatal("main function not found in environment")
	}
	mainFunc, ok := mainFuncObj.(*object.Function)
	if !ok {
		t.Fatalf("main is not an object.Function, got %T", mainFuncObj)
	}

	eval.applyFunction(mainFunc, []object.Object{}, pkg)
}

func TestEvalCallExprOnIntrinsic(t *testing.T) {
	source := `
package main
import "fmt"
func main() { fmt.Println("hello") }
`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir))
	if err != nil {
		t.Fatalf("NewScanner() failed: %v", err)
	}
	pkg, err := s.ScanPackage(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanPackage() failed: %v", err)
	}

	internalScanner, err := s.ScannerForSymgo()
	if err != nil {
		t.Fatalf("ScannerForSymgo() failed: %v", err)
	}
	eval := New(internalScanner, s.Logger)
	env := object.NewEnvironment()

	var got string
	eval.RegisterIntrinsic("fmt.Println", func(args ...object.Object) object.Object {
		if len(args) > 0 {
			if s, ok := args[0].(*object.String); ok {
				got = s.Value
			}
		}
		return &object.SymbolicPlaceholder{}
	})

	for _, file := range pkg.AstFiles {
		eval.Eval(file, env, pkg)
	}

	mainFuncObj, ok := env.Get("main")
	if !ok {
		t.Fatal("main function not found in environment")
	}
	mainFunc, ok := mainFuncObj.(*object.Function)
	if !ok {
		t.Fatalf("main is not an object.Function, got %T", mainFuncObj)
	}
	eval.applyFunction(mainFunc, []object.Object{}, pkg)

	if want := "hello"; got != want {
		t.Errorf("intrinsic not called correctly, want %q, got %q", want, got)
	}
}

func TestEvalCallExprOnInstanceMethod(t *testing.T) {
	source := `
package main
import "net/http"
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", nil)
}`
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	})
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir))
	if err != nil {
		t.Fatalf("NewScanner() failed: %v", err)
	}
	pkg, err := s.ScanPackage(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScanPackage() failed: %v", err)
	}

	internalScanner, err := s.ScannerForSymgo()
	if err != nil {
		t.Fatalf("ScannerForSymgo() failed: %v", err)
	}
	eval := New(internalScanner, s.Logger)
	env := object.NewEnvironment()

	const serveMuxTypeName = "net/http.ServeMux"
	eval.RegisterIntrinsic("net/http.NewServeMux", func(args ...object.Object) object.Object {
		return &object.Instance{TypeName: serveMuxTypeName}
	})

	var gotPattern string
	eval.RegisterIntrinsic(fmt.Sprintf("(*%s).HandleFunc", serveMuxTypeName), func(args ...object.Object) object.Object {
		if len(args) > 0 {
			if s, ok := args[0].(*object.String); ok {
				gotPattern = s.Value
			}
		}
		return nil
	})

	for _, file := range pkg.AstFiles {
		eval.Eval(file, env, pkg)
	}

	mainFuncObj, ok := env.Get("main")
	if !ok {
		t.Fatal("main function not found in environment")
	}
	mainFunc, ok := mainFuncObj.(*object.Function)
	if !ok {
		t.Fatalf("main is not an object.Function, got %T", mainFuncObj)
	}

	eval.applyFunction(mainFunc, []object.Object{}, pkg)

	if want := "/"; gotPattern != want {
		t.Errorf("HandleFunc pattern wrong, want %q, got %q", want, gotPattern)
	}
}
