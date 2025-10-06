package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestRegression_FluentAPI(t *testing.T) {
	t.Skip("this test is expected to fail until the fluent API bug is fixed")

	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

type App struct {
	name string
}
func (a *App) Name(name string) *App {
	a.name = name
	return a
}
func (a *App) Description(desc string) *App {
	return a
}

func main() {
	app := &App{}
	app.Name("my-app").Description("a test app")
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var calls []string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })

		eval.RegisterIntrinsic("(*example.com/me.App).Name", func(ctx context.Context, args ...object.Object) object.Object {
			calls = append(calls, "Name")
			return args[0]
		})
		eval.RegisterIntrinsic("(*example.com/me.App).Description", func(ctx context.Context, args ...object.Object) object.Object {
			calls = append(calls, "Description")
			return args[0]
		})

		for _, file := range mainPkg.AstFiles {
			if res := eval.Eval(ctx, file, nil, mainPkg); res != nil && isError(res) {
				return res.(*object.Error)
			}
		}

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

	want := []string{"Name", "Description"}
	if fmt.Sprintf("%v", calls) != fmt.Sprintf("%v", want) {
		t.Errorf("fluent api calls mismatch, want=%v, got=%v", want, calls)
	}
}

func TestRegression_MethodOnNamedSlice(t *testing.T) {
	t.Skip("this test is expected to fail until the named slice method bug is fixed")

	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

type MySlice []int
func (s MySlice) Sum() int {
	sum := 0
	for _, v := range s {
		sum += v
	}
	return sum
}

func main() {
	s := MySlice{1, 2, 3}
	_ = s.Sum()
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sumCalled bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		eval := New(s, s.Logger, nil, func(pkgPath string) bool { return true })

		eval.RegisterIntrinsic("(example.com/me.MySlice).Sum", func(ctx context.Context, args ...object.Object) object.Object {
			sumCalled = true
			return &object.Integer{Value: 6}
		})

		for _, file := range mainPkg.AstFiles {
			if res := eval.Eval(ctx, file, nil, mainPkg); res != nil && isError(res) {
				return res.(*object.Error)
			}
		}

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

	if !sumCalled {
		t.Error("Sum method on named slice was not called")
	}
}