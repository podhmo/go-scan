package evaluator

import (
	"context"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestPolicy_FunctionCallOnOutOfPolicyPackage(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main
import "example.com/me/foreign"

func main() {
	foreign.DoSomething()
}
`,
		"foreign/foreign.go": `
package foreign

func DoSomething() string {
	return "done"
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// Policy: do not scan the 'foreign' package.
		policy := func(path string) bool {
			return !strings.Contains(path, "foreign")
		}

		evaluator := New(s, nil, nil, policy)

		var mainPkg *goscan.Package
		for _, pkg := range pkgs {
			if pkg.ImportPath == "example.com/me" {
				mainPkg = pkg
				break
			}
		}
		if mainPkg == nil {
			t.Fatal("main package not found")
		}

		pkgEnv := object.NewEnclosedEnvironment(evaluator.UniverseEnv)
		for _, file := range mainPkg.AstFiles {
			evaluator.Eval(ctx, file, pkgEnv, mainPkg)
		}

		mainFunc, _ := pkgEnv.Get("main")

		var capturedFunc object.Object
		evaluator.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) > 0 {
				capturedFunc = args[0]
			}
			return nil
		})

		result := evaluator.Apply(ctx, mainFunc, nil, mainPkg, pkgEnv)
		if err, ok := result.(*object.Error); ok {
			t.Fatalf("Apply() failed: %s", err.Inspect())
		}

		if capturedFunc == nil {
			t.Fatal("function call on foreign package was not captured")
		}

		unresolved, ok := capturedFunc.(*object.UnresolvedType)
		if !ok {
			t.Fatalf("expected captured function to be UnresolvedType, got %T", capturedFunc)
		}

		if unresolved.PkgPath != "example.com/me/foreign" {
			t.Errorf("expected unresolved type to have PkgPath 'example.com/me/foreign', got %q", unresolved.PkgPath)
		}
		if unresolved.TypeName != "DoSomething" {
			t.Errorf("expected unresolved type to have TypeName 'DoSomething', got %q", unresolved.TypeName)
		}

		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
