package evaluator

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

// TestRegression_FluentAPI reproduces a bug where method chaining fails.
// The evaluator should not fail with "... got FUNCTION".
func TestRegression_FluentAPI(t *testing.T) {
	t.Skip("this test is expected to fail until the fluent API bug is fixed")

	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
type App struct {}
func (a *App) Name(name string) *App { return a }
func (a *App) Description(desc string) *App { return a }
func main() {
	app := &App{}
	app.Name("my-app").Description("a test app")
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })

		// We just need to run it and ensure it doesn't crash.
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)

		if res := eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0); res != nil && isError(res) {
			return res.(*object.Error)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

// TestRegression_MethodOnNamedSlice reproduces a bug where methods on named slices are not found.
// The evaluator should not fail with "... got SLICE".
func TestRegression_MethodOnNamedSlice(t *testing.T) {
	t.Skip("this test is expected to fail until the named slice method bug is fixed")
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
type MySlice []int
func (s MySlice) Sum() int { return 0 }
func main() {
	s := MySlice{1, 2, 3}
	_ = s.Sum()
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		if res := eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0); res != nil && isError(res) {
			return res.(*object.Error)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

// TestRegression_InvalidIndirectOfNil reproduces a bug where dereferencing nil panics.
// The evaluator should return a symbolic placeholder, not crash.
func TestRegression_InvalidIndirectOfNil(t *testing.T) {
	t.Skip("this test is expected to fail until dereferencing nil is handled gracefully")
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
func main() {
	var p *int
	_ = *p
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		if res := eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0); res != nil && isError(res) {
			return res.(*object.Error)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

// TestRegression_UnaryOnUnresolved reproduces a bug where a unary operator on an unresolved function fails.
// The evaluator should return a symbolic placeholder.
func TestRegression_UnaryOnUnresolved(t *testing.T) {
	t.Skip("this test is expected to fail until unary ops on unresolved functions are handled")
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
import "os"
func main() {
	_ = -os.Getpid() // os.Getpid is an unresolved function in this context
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		// Policy: don't scan os package
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return pkgPath != "os" })
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		if res := eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0); res != nil && isError(res) {
			return res.(*object.Error)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

// TestRegression_LenOnUnsupported reproduces a bug where `len` is called on an unsupported type.
// The evaluator should return a symbolic placeholder.
func TestRegression_LenOnUnsupported(t *testing.T) {
	t.Skip("this test is expected to fail until len() on symbolic values is supported")
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
func getSomething() interface{} { return "hello" }
func main() {
	x := getSomething()
	_ = len(x)
}`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })
		eval.RegisterIntrinsic("example.com/me.getSomething", func(ctx context.Context, args ...object.Object) object.Object {
			// Return a symbolic instance, which len() doesn't support
			return &object.Instance{TypeName: "some.Type"}
		})
		pkgEnv, _ := eval.PackageEnvForTest(mainPkg.ImportPath)
		mainFuncObj, _ := pkgEnv.Get("main")
		mainFunc, _ := mainFuncObj.(*object.Function)
		if res := eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0); res != nil && isError(res) {
			return res.(*object.Error)
		}
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}