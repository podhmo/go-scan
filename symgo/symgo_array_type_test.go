package symgo_test

import (
	"context"
	"go/ast"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
)

func TestArrayTypeExpression(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/me/myapp",
		"main.go": `
package main
func main() {
	_ = []byte("hello")
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			t.Fatalf("NewInterpreter() failed: %v", err)
		}

		// Evaluate the file to populate the environment
		var file *ast.File
		for _, f := range mainPkg.AstFiles {
			file = f
			break
		}
		if file == nil {
			t.Fatalf("could not find ast file in package")
		}
		if _, err := interp.Eval(context.Background(), file, mainPkg); err != nil {
			t.Fatalf("Eval(file) failed: %+v", err)
		}

		// Find and apply the main function
		mainObj, ok := interp.FindObject("main")
		if !ok {
			t.Fatalf("could not find main function in interpreter")
		}
		mainFn, ok := mainObj.(*symgo.Function)
		if !ok {
			t.Fatalf("main is not a function, but %T", mainObj)
		}

		_, err = interp.Apply(context.Background(), mainFn, []symgo.Object{}, mainPkg)
		if err != nil {
			t.Errorf("Apply(main) should not have failed, but got: %+v", err)
		}
		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
