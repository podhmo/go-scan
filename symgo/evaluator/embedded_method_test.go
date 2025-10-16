package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestEval_EmbeddedMethodCall(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m",
		"base/base.go": `
package base

type BaseController struct{}

func (c *BaseController) Validate(ob any) error {
	return nil
}
`,
		"main.go": `
package main

import "example.com/m/base"

type Controller struct {
	base.BaseController
}

func main() {
	ctrl := &Controller{}
	ctrl.Validate(nil)
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var calledFunctions []string
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		if mainPkg.Name != "main" {
			return fmt.Errorf("expected main package, but got %s", mainPkg.Name)
		}

		eval := New(s, s.Logger, nil, nil)

		// This default intrinsic will be called for every function call.
		// We use it to record what was called.
		eval.RegisterDefaultIntrinsic(func(ctx context.Context, args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			fnObj := args[0]
			switch fn := fnObj.(type) {
			case *object.Function:
				var signature string
				if fn.Def != nil && fn.Def.Receiver != nil {
					// This is a method. Construct the signature from its definition.
					recvType := fn.Def.Receiver.Type

					var builder strings.Builder
					builder.WriteString("(")
					if recvType.IsPointer {
						builder.WriteString("*")
					}
					builder.WriteString(recvType.FullImportPath)
					builder.WriteString(".")
					builder.WriteString(recvType.TypeName)
					builder.WriteString(")")
					builder.WriteString(".")
					builder.WriteString(fn.Name.Name)
					signature = builder.String()
				} else if fn.Package != nil {
					// This is a regular function.
					signature = fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Name.Name)
				} else {
					signature = fn.Name.Name
				}
				calledFunctions = append(calledFunctions, signature)
			case *object.SymbolicPlaceholder:
				// This case can be useful for debugging external calls
			}
			return nil
		})

		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		pkgEnv, ok := eval.PackageEnvForTest("example.com/m")
		if !ok {
			return fmt.Errorf("could not get package env for 'example.com/m'")
		}
		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc, ok := mainFuncObj.(*object.Function)
		if !ok {
			return fmt.Errorf("main is not an object.Function, got %T", mainFuncObj)
		}

		eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	found := false
	expected := "(*example.com/m/base.BaseController).Validate"
	for _, called := range calledFunctions {
		if strings.Contains(called, "Validate") {
			t.Logf("found call: %s", called)
		}
		if called == expected {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("expected to find call to %q, but it was not called.\nCalled functions:\n%s", expected, strings.Join(calledFunctions, "\n"))
	}
}
