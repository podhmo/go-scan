package evaluator

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSliceLiterals(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"main.go": `
package main

type User struct {
	ID   int
	Name string
}

func main() {
	_ = []User{}
	_ = []*User{}
	_ = []string{}
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		// We just need to evaluate the file to trigger the composite literal evaluation.
		// The test is to ensure it doesn't panic and correctly creates slice objects.
		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		// A more robust test could inspect the environment or returned values,
		// but for now, we're just checking for crashes.
		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestSliceIndexExpr(t *testing.T) {
	source := `
package main

func main() {
	items := []string{"a", "b", "c"}
	_ = items[0]
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, 0)

		if isError(result) {
			return fmt.Errorf("evaluation failed: %s", result.Inspect())
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestSliceIndexExpr_Variable(t *testing.T) {
	source := `
package main

func main() {
	items := []string{"a", "b", "c"}
	i := 1
	_ = items[i]
}
`
	files := map[string]string{
		"go.mod":  "module example.com/me",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		pkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		for _, file := range pkg.AstFiles {
			eval.Eval(ctx, file, nil, pkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, pkg, 0)

		// The important part is that this doesn't crash. The result of an index
		// operation with a symbolic index is a symbolic value.
		if isError(result) {
			return fmt.Errorf("evaluation failed: %s", result.Inspect())
		}
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestSliceTypeFromExternalPackage(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me",
		"models/models.go": `
package models
type User struct { ID int }
`,
		"main.go": `
package main
import "example.com/me/models"
func main() {
	_ = []models.User{}
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := findPkg(pkgs, "main")
		if mainPkg == nil {
			return fmt.Errorf("main package not found")
		}
		eval := New(s, s.Logger, nil, nil)

		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/me")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/me'")
		}
		mainFunc, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found")
		}
		result := eval.applyFunction(ctx, mainFunc, []object.Object{}, mainPkg, 0)

		if isError(result) {
			return fmt.Errorf("evaluation failed: %s", result.Inspect())
		}

		return nil
	}
	if _, err := scantest.Run(t, t.Context(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
