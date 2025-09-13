package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"log/slog"
	"os"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
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

		eval := New(s, s.Logger, nil, nil)
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, ok := pkgEnv.Get("main")
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

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalCallExpr_SymbolicInterfaceMethod_MultiReturn(t *testing.T) {
	// This test specifically targets the bug where a call to a symbolic interface
	// method that returns multiple values was returning a single placeholder instead
	// of a MultiReturn object.
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

type PairGetter interface {
	GetPair() (string, error)
}

func useGetter(g PairGetter) {
	_, _ = g.GetPair()
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true }) // Scan all

		// 1. Find the interface and method info
		var pairGetter *scanner.TypeInfo
		var getPairMethod *scanner.MethodInfo
		for _, p := range pkgs {
			for _, ti := range p.Types {
				if ti.Name == "PairGetter" {
					pairGetter = ti
					break
				}
			}
		}
		if pairGetter == nil {
			return fmt.Errorf("could not find PairGetter interface")
		}
		for _, m := range pairGetter.Interface.Methods {
			if m.Name == "GetPair" {
				getPairMethod = m
				break
			}
		}
		if getPairMethod == nil {
			return fmt.Errorf("could not find GetPair method")
		}

		// 2. Create a symbolic placeholder for the function call itself.
		// This simulates what `evalSelectorExpr` would create for `g.GetPair()`.
		fnPlaceholder := &object.SymbolicPlaceholder{
			Reason: "interface method call GetPair",
			UnderlyingFunc: &scanner.FunctionInfo{
				Name:       getPairMethod.Name,
				Parameters: getPairMethod.Parameters,
				Results:    getPairMethod.Results,
			},
			// The receiver would be another placeholder for `g`.
			Receiver: &object.SymbolicPlaceholder{Reason: "variable g"},
		}

		// 3. Apply the function placeholder. This is where the bug occurs.
		result := eval.applyFunction(ctx, fnPlaceholder, []object.Object{}, mainPkg, token.NoPos)

		// 4. Check the result
		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T (%s)", result, result.Inspect())
		}
		multiRet, ok := retVal.Value.(*object.MultiReturn)
		if !ok {
			// This is the core of the bug. The result should be MultiReturn,
			// but it was likely a single SymbolicPlaceholder.
			return fmt.Errorf("expected MultiReturn, got %T (%s)", result, result.Inspect())
		}
		if len(multiRet.Values) != 2 {
			return fmt.Errorf("expected 2 return values, got %d", len(multiRet.Values))
		}

		// Check the type of the first return value (string)
		val1 := multiRet.Values[0]
		if val1.FieldType().Name != "string" {
			return fmt.Errorf("expected first return value to be string, got %s", val1.FieldType().Name)
		}

		// Check the type of the second return value (error)
		val2 := multiRet.Values[1]
		if val2.FieldType().Name != "error" {
			return fmt.Errorf("expected second return value to be error, got %s", val2.FieldType().Name)
		}
		if val2.TypeInfo() == nil || val2.TypeInfo().Interface == nil {
			return fmt.Errorf("expected second return value to have resolved TypeInfo for error interface")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestEvalCallExpr_UnresolvedFunction_MultiReturn(t *testing.T) {
	// This test simulates the case where a package initially fails to scan,
	// leading to an UnresolvedFunction object. We then check if applyFunction
	// can correctly re-resolve this function and produce a multi-return symbolic value.
	files := map[string]string{
		"go.mod": "module example.com/me",
		"helper/helper.go": `
package helper
func GetPair() (string, error) { return "ok", nil }
`,
		"main.go": `
package main
import "example.com/me/helper"
func main() {
	_, _ = helper.GetPair()
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		if mainPkg.Name != "main" {
			for _, p := range pkgs {
				if p.Name == "main" {
					mainPkg = p
					break
				}
			}
		}

		// The evaluator is created without a scan policy, so it should be able to find the package.
		eval := New(s, s.Logger, nil, nil)

		// Simulate the creation of an UnresolvedFunction, which is what the
		// evaluator would do if `resolvePackageWithoutPolicyCheck` failed inside `evalSelectorExpr`.
		unresolvedFn := &object.UnresolvedFunction{
			PkgPath:  "example.com/me/helper",
			FuncName: "GetPair",
		}

		// Apply the unresolved function. This should trigger the new logic in `applyFunction`.
		result := eval.applyFunction(ctx, unresolvedFn, []object.Object{}, mainPkg, token.NoPos)

		// Check the result
		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T (%v)", result, result)
		}
		multiRet, ok := retVal.Value.(*object.MultiReturn)
		if !ok {
			return fmt.Errorf("expected MultiReturn, got %T", retVal.Value)
		}
		if len(multiRet.Values) != 2 {
			return fmt.Errorf("expected 2 return values, got %d", len(multiRet.Values))
		}

		// Check the type of the first return value (string)
		val1 := multiRet.Values[0]
		if val1.FieldType().Name != "string" {
			return fmt.Errorf("expected first return value to be string, got %s", val1.FieldType().Name)
		}

		// Check the type of the second return value (error)
		val2 := multiRet.Values[1]
		if val2.FieldType().Name != "error" {
			return fmt.Errorf("expected second return value to be error, got %s", val2.FieldType().Name)
		}
		if val2.TypeInfo() == nil || val2.TypeInfo().Interface == nil {
			return fmt.Errorf("expected second return value to have resolved TypeInfo for error interface")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
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
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterIntrinsic("fmt.Println", func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) > 0 {
				if s, ok := args[0].(*object.String); ok {
					got = s.Value
				}
			}
			return &object.SymbolicPlaceholder{}
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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
		eval := New(s, s.Logger, nil, nil)

		const serveMuxTypeName = "net/http.ServeMux"
		eval.RegisterIntrinsic("net/http.NewServeMux", func(ctx context.Context, args ...object.Object) object.Object {
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

		eval.RegisterIntrinsic(fmt.Sprintf("(*%s).HandleFunc", serveMuxTypeName), func(ctx context.Context, args ...object.Object) object.Object {
			handleFuncCalled = true
			if len(args) > 1 { // self, pattern, handler
				if s, ok := args[1].(*object.String); ok {
					gotPattern = s.Value
				}
			}
			return nil
		})

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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
			handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
			logger := slog.New(handler)
			eval := New(s, logger, nil, nil)

			key := fmt.Sprintf("(%s.S).Do", pkg.ImportPath)
			eval.RegisterIntrinsic(key, func(ctx context.Context, args ...object.Object) object.Object {
				doCalled = true
				return nil
			})

			for _, file := range pkg.AstFiles {
				eval.Eval(ctx, file, nil, pkg)
			}

			pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
			if !ok {
				return fmt.Errorf("could not get package env for 'example.com/me'")
			}
			mainFuncObj, _ := pkgEnv.Get("main")
			mainFunc := mainFuncObj.(*object.Function)
			eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
			return nil
		}

		if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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
			handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
			logger := slog.New(handler)
			eval := New(s, logger, nil, nil)

			greeterTypeName := fmt.Sprintf("%s.Greeter", pkg.ImportPath)
			eval.RegisterIntrinsic(fmt.Sprintf("%s.NewGreeter", pkg.ImportPath), func(ctx context.Context, args ...object.Object) object.Object {
				name := args[0].(*object.String).Value
				return &object.Instance{TypeName: fmt.Sprintf("*%s", greeterTypeName), State: map[string]object.Object{"name": &object.String{Value: name}}}
			})
			eval.RegisterIntrinsic(fmt.Sprintf("(*%s).WithName", greeterTypeName), func(ctx context.Context, args ...object.Object) object.Object {
				g := args[0].(*object.Instance)
				name := args[1].(*object.String).Value
				if g.State == nil {
					g.State = make(map[string]object.Object)
				}
				g.State["name"] = &object.String{Value: name}
				return g
			})
			eval.RegisterIntrinsic(fmt.Sprintf("(*%s).Greet", greeterTypeName), func(ctx context.Context, args ...object.Object) object.Object {
				greetCalled = true
				g := args[0].(*object.Instance)
				name, _ := g.State["name"].(*object.String)
				finalName = name.Value
				return &object.String{Value: "Hello, " + finalName}
			})
			eval.RegisterIntrinsic("fmt.Println", func(ctx context.Context, args ...object.Object) object.Object {
				return nil
			})

			for _, file := range pkg.AstFiles {
				eval.Eval(ctx, file, nil, pkg)
			}

			pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
			if !ok {
				return fmt.Errorf("could not get package env for 'example.com/me'")
			}
			mainFuncObj, _ := pkgEnv.Get("main")
			mainFunc := mainFuncObj.(*object.Function)
			eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
			return nil
		}

		if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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
			eval := New(s, s.Logger, nil, nil)

			eval.RegisterIntrinsic(fmt.Sprintf("%s.add", pkg.ImportPath), func(ctx context.Context, args ...object.Object) object.Object {
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
				eval.Eval(ctx, file, nil, pkg)
			}

			pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
			if !ok {
				return fmt.Errorf("could not get package env for 'example.com/me'")
			}
			mainFuncObj, _ := pkgEnv.Get("main")
			mainFunc := mainFuncObj.(*object.Function)
			eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, token.NoPos)
			return nil
		}

		if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
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

func TestEvalCallExpr_IntraPackageCall(t *testing.T) {
	// This test reproduces the scenario where a function (main) calls another
	// function (bootstrap) in the same package. The bootstrap function has more
	// complex statements like defer and multi-value assignment to better
	// reproduce the user's issue.
	files := map[string]string{
		"go.mod": "module example.com/me",
		"lib/lib.go": `
package lib

func DoSomething() {}
func Cleanup() {}
func GetPair() (int, error) { return 1, nil }
`,
		"main.go": `
package main
import "example.com/me/lib"

func bootstrap() {
	defer lib.Cleanup()
	_, err := lib.GetPair()
	if err != nil {
		return
	}
	lib.DoSomething()
}

func main() {
	bootstrap()
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var doSomethingCalled bool
	var cleanupCalled bool

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		if mainPkg.Name != "main" {
			for _, p := range pkgs {
				if p.Name == "main" {
					mainPkg = p
					break
				}
			}
		}

		eval := New(s, s.Logger, nil, nil)

		// Register intrinsics to track calls
		eval.RegisterIntrinsic("example.com/me/lib.DoSomething", func(ctx context.Context, args ...object.Object) object.Object {
			doSomethingCalled = true
			return nil
		})
		eval.RegisterIntrinsic("example.com/me/lib.Cleanup", func(ctx context.Context, args ...object.Object) object.Object {
			cleanupCalled = true
			return nil
		})
		eval.RegisterIntrinsic("example.com/me/lib.GetPair", func(ctx context.Context, args ...object.Object) object.Object {
			// Simulate multi-value return
			return &object.MultiReturn{Values: []object.Object{
				&object.Integer{Value: 1},
				object.NIL,
			}}
		})

		// Evaluate all files in the main package to populate the environment.
		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		// Find the main function to start execution.
		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not a function, but %T", mainFuncObj)
		}

		// Apply the main function, which should trigger the chain of calls.
		// We are not checking the error here because this test is DESIGNED to fail
		// and we want to see the error log from find-orphans itself.
		eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, token.NoPos)
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	if !cleanupCalled {
		t.Error("expected lib.Cleanup to be called, but it was not")
	}
	if !doSomethingCalled {
		t.Error("expected lib.DoSomething to be called, but it was not")
	}
}

func TestEvalCallExpr_OutOfPolicy_MultiReturn(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"helper/helper.go": `
package helper
func GetPair() (string, error) { return "ok", nil }
`,
		"main.go": `
package main
import "example.com/me/helper"
func main() {
	_, _ = helper.GetPair()
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		if mainPkg.Name != "main" {
			for _, p := range pkgs {
				if p.Name == "main" {
					mainPkg = p
					break
				}
			}
		}

		// Policy: scan "main" package, but not "helper" package
		scanPolicy := func(pkgPath string) bool {
			return pkgPath == "example.com/me"
		}

		eval := New(s, s.Logger, nil, scanPolicy)

		// Evaluate main package to populate env
		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		// Manually create the placeholder for the external function call
		// This simulates what would happen inside the evaluator when it encounters helper.GetPair
		helperPkg, err := s.ScanPackageByImport(ctx, "example.com/me/helper")
		if err != nil {
			return fmt.Errorf("failed to scan helper package: %w", err)
		}
		var getPairFunc *goscan.FunctionInfo
		for _, f := range helperPkg.Functions {
			if f.Name == "GetPair" {
				getPairFunc = f
				break
			}
		}
		if getPairFunc == nil {
			return fmt.Errorf("could not find GetPair function in helper package")
		}

		// The resolver creates a placeholder because the helper package is out of policy.
		fnPlaceholder := eval.resolver.ResolveFunction(ctx, &object.Package{
			Path:        helperPkg.ImportPath,
			ScannedInfo: helperPkg,
		}, getPairFunc)

		// Apply the function placeholder
		result := eval.applyFunction(ctx, fnPlaceholder, []object.Object{}, mainPkg, token.NoPos)

		// Check the result
		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			return fmt.Errorf("expected ReturnValue, got %T", result)
		}
		multiRet, ok := retVal.Value.(*object.MultiReturn)
		if !ok {
			return fmt.Errorf("expected MultiReturn, got %T", retVal.Value)
		}
		if len(multiRet.Values) != 2 {
			return fmt.Errorf("expected 2 return values, got %d", len(multiRet.Values))
		}

		// Check the type of the first return value (string)
		val1 := multiRet.Values[0]
		if val1.FieldType().Name != "string" {
			return fmt.Errorf("expected first return value to be string, got %s", val1.FieldType().Name)
		}

		// Check the type of the second return value (error)
		val2 := multiRet.Values[1]
		if val2.FieldType().Name != "error" {
			return fmt.Errorf("expected second return value to be error, got %s", val2.FieldType().Name)
		}
		if val2.TypeInfo() == nil || val2.TypeInfo().Interface == nil {
			return fmt.Errorf("expected second return value to have resolved TypeInfo for error interface")
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
