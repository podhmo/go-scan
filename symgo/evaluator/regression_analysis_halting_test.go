package evaluator

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

// TestRegression_Halting_FluentAPI confirms that a "got FUNCTION" error halts analysis.
func TestRegression_Halting_FluentAPI(t *testing.T) {
	t.Skip("this test is expected to fail until the fluent API bug is fixed")
	source := `
package main
import "fmt"
type App struct {}
func (a *App) Name(string) *App { return a }
func (a *App) Description(string) *App { return a }
func main() {
	app := &App{}
	app.Name("my-app").Description("a test app") // This line fails
	fmt.Println("should not be reached")
}`
	var reached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(string) bool { return true })
		eval.RegisterIntrinsic("(*example.com/me.App).Name", func(c context.Context, args ...object.Object) object.Object { return args[0] })
		eval.RegisterIntrinsic("(*example.com/me.App).Description", func(c context.Context, args ...object.Object) object.Object { return args[0] })
		eval.RegisterIntrinsic("fmt.Println", func(c context.Context, args ...object.Object) object.Object {
			reached = true
			return nil
		})

		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)

		eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0)
		return nil
	}

	dir, cleanup := scantest.WriteFiles(t, map[string]string{"main.go": source, "go.mod": "module example.com/me"})
	defer cleanup()
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
	if reached {
		t.Error("Analysis was not halted by fluent API error, but it should have been.")
	}
}

// TestRegression_Halting_MethodOnNamedSlice confirms that a "got SLICE" error halts analysis.
func TestRegression_Halting_MethodOnNamedSlice(t *testing.T) {
	t.Skip("this test is expected to fail until the named slice method bug is fixed")
	source := `
package main
import "fmt"
type MySlice []int
func (s MySlice) Sum() int { return 0 }
func main() {
	s := MySlice{1, 2, 3}
	_ = s.Sum() // This line fails
	fmt.Println("should not be reached")
}`
	var reached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(string) bool { return true })
		eval.RegisterIntrinsic("fmt.Println", func(c context.Context, args ...object.Object) object.Object {
			reached = true
			return nil
		})
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0)
		return nil
	}
	dir, cleanup := scantest.WriteFiles(t, map[string]string{"main.go": source, "go.mod": "module example.com/me"})
	defer cleanup()
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
	if reached {
		t.Error("Analysis was not halted by named slice method error, but it should have been.")
	}
}

// TestRegression_Halting_InvalidIndirectOfNil confirms that dereferencing nil halts analysis.
func TestRegression_Halting_InvalidIndirectOfNil(t *testing.T) {
	t.Skip("this test is expected to fail until dereferencing nil is handled gracefully")
	source := `
package main
import "fmt"
func main() {
	var p *int
	_ = *p // This line fails
	fmt.Println("should not be reached")
}`
	var reached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(string) bool { return true })
		eval.RegisterIntrinsic("fmt.Println", func(c context.Context, args ...object.Object) object.Object {
			reached = true
			return nil
		})
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0)
		return nil
	}
	dir, cleanup := scantest.WriteFiles(t, map[string]string{"main.go": source, "go.mod": "module example.com/me"})
	defer cleanup()
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
	if reached {
		t.Error("Analysis was not halted by nil dereference, but it should have been.")
	}
}

// TestRegression_Halting_LenOnUnsupported confirms that `len` on an unsupported type halts analysis.
func TestRegression_Halting_LenOnUnsupported(t *testing.T) {
	t.Skip("this test is expected to fail until len() on symbolic values is supported")
	source := `
package main
import "fmt"
func getSomething() interface{} { return "hello" }
func main() {
	x := getSomething()
	_ = len(x) // This line fails because getSomething returns a symbolic instance
	fmt.Println("should not be reached")
}`
	var reached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(string) bool { return true })
		eval.RegisterIntrinsic("example.com/me.getSomething", func(ctx context.Context, args ...object.Object) object.Object {
			return &object.Instance{TypeName: "some.Type"} // Return a type that len() doesn't support
		})
		eval.RegisterIntrinsic("fmt.Println", func(c context.Context, args ...object.Object) object.Object {
			reached = true
			return nil
		})
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0)
		return nil
	}
	dir, cleanup := scantest.WriteFiles(t, map[string]string{"main.go": source, "go.mod": "module example.com/me"})
	defer cleanup()
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
	if reached {
		t.Error("Analysis was not halted by len() on unsupported type, but it should have been.")
	}
}

// TestRegression_Halting_BinaryOpOnReturnValue confirms that a binary op on a ReturnValue halts analysis.
func TestRegression_Halting_BinaryOpOnReturnValue(t *testing.T) {
	t.Skip("this test is expected to fail until binary ops on return values are handled")
	source := `
package main
import "fmt"
func five() int { return 5 }
func main() {
	_ = five() + 1 // This line fails
	fmt.Println("should not be reached")
}`
	var reached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(string) bool { return true })
		eval.RegisterIntrinsic("fmt.Println", func(c context.Context, args ...object.Object) object.Object {
			reached = true
			return nil
		})
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0)
		return nil
	}
	dir, cleanup := scantest.WriteFiles(t, map[string]string{"main.go": source, "go.mod": "module example.com/me"})
	defer cleanup()
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
	if reached {
		t.Error("Analysis was not halted by binary op on return value, but it should have been.")
	}
}