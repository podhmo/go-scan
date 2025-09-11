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

func TestEval_FunctionInCompositeLiteral(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m",
		"main.go": `
package main

type Y struct {
	Handler func()
}

type X struct {
	Ys []Y
}

func f() {}

func g(fn func()) func() {
	return fn
}

func main() {
	_ = &X{
		Ys: []Y{
			{
				Handler: g(f),
			},
		},
	}
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	usedFunctions := make(map[string]bool)

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		// This default intrinsic will be called for every function call.
		// We use it to record what was called, and what was passed as arguments.
		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}

			// Mark the function being called as used
			if fn, ok := args[0].(*object.Function); ok {
				sig := getFunctionSignature(fn)
				usedFunctions[sig] = true
			}

			// Mark any functions passed as arguments as used
			for _, arg := range args[1:] {
				if fn, ok := arg.(*object.Function); ok {
					sig := getFunctionSignature(fn)
					usedFunctions[sig] = true
				}
			}

			// Return a placeholder for the call's result
			return &object.SymbolicPlaceholder{Reason: "intrinsic call"}
		})

		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		pkgEnv := eval.PackageEnvForTest("example.com/m")
		if pkgEnv == nil {
			return fmt.Errorf("could not get package env for 'example.com/m'")
		}
		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc := mainFuncObj.(*object.Function)

		eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	t.Logf("Used functions: %v", usedFunctions)

	f_signature := "example.com/m.f"
	if _, ok := usedFunctions[f_signature]; !ok {
		t.Errorf("expected function 'f' to be marked as used, but it was not")
	}

	g_signature := "example.com/m.g"
	if _, ok := usedFunctions[g_signature]; !ok {
		t.Errorf("expected function 'g' to be marked as used, but it was not")
	}
}

// Helper to create a consistent function signature for tracking.
func getFunctionSignature(fn *object.Function) string {
	if fn.Def != nil && fn.Def.Receiver != nil {
		// This is a method.
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
		return builder.String()
	} else if fn.Package != nil {
		// This is a regular function.
		return fmt.Sprintf("%s.%s", fn.Package.ImportPath, fn.Name.Name)
	}
	return fn.Name.Name
}

func TestEval_FunctionInMapLiteral(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m",
		"main.go": `
package main

func f() {}

func g(fn func()) func() {
	return fn
}

func h() string {
	return "key2"
}

func main() {
	_ = map[string]func(){
		"key1":  f,
		h(): g(f),
	}
}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	usedFunctions := make(map[string]bool)

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		mainPkg := pkgs[0]
		eval := New(s, s.Logger, nil, nil)

		eval.RegisterDefaultIntrinsic(func(args ...object.Object) object.Object {
			if len(args) == 0 {
				return nil
			}
			if fn, ok := args[0].(*object.Function); ok {
				sig := getFunctionSignature(fn)
				usedFunctions[sig] = true
			}
			for _, arg := range args[1:] {
				if fn, ok := arg.(*object.Function); ok {
					sig := getFunctionSignature(fn)
					usedFunctions[sig] = true
				}
			}
			return &object.SymbolicPlaceholder{Reason: "intrinsic call"}
		})

		for _, file := range mainPkg.AstFiles {
			eval.Eval(ctx, file, nil, mainPkg)
		}

		pkgEnv := eval.PackageEnvForTest("example.com/m")
		if pkgEnv == nil {
			return fmt.Errorf("could not get package env for 'example.com/m'")
		}
		mainFuncObj, ok := pkgEnv.Get("main")
		if !ok {
			return fmt.Errorf("main function not found in environment")
		}
		mainFunc := mainFuncObj.(*object.Function)

		eval.Apply(ctx, mainFunc, []object.Object{}, mainPkg)
		return nil
	}

	if _, err := scantest.Run(t, t.Context(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}

	t.Logf("Used functions in map literal test: %v", usedFunctions)

	expected := []string{
		"example.com/m.f",
		"example.com/m.g",
		"example.com/m.h",
	}

	for _, sig := range expected {
		if _, ok := usedFunctions[sig]; !ok {
			t.Errorf("expected function '%s' to be marked as used, but it was not", sig)
		}
	}
}
