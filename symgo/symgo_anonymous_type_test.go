package symgo_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestSymgo_AnonymousTypes(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	source := `
package main

type ObjectId interface {
	Hex() string
}

func AnonymousInterface(id interface {
	Hex() string
}) string {
	if id == nil {
		return ""
	}
	return id.Hex()
}

func AnonymousStruct(p struct {
	X, Y int
}) int {
	return p.X
}
`
	files := map[string]string{
		"go.mod":  "module mymodule",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	_, err := scantest.Run(t, ctx, dir, []string{"."}, func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		t.Run("anonymous interface", func(t *testing.T) {
			interpreter, err := symgo.NewInterpreter(s, symgo.WithLogger(logger))
			if err != nil {
				t.Fatalf("NewInterpreter failed: %+v", err)
			}

			var inspectedMethod *scanner.MethodInfo
			interpreter.RegisterDefaultIntrinsic(func(i *symgo.Interpreter, args []symgo.Object) symgo.Object {
				fn := args[0] // The function object itself
				if p, ok := fn.(*symgo.SymbolicPlaceholder); ok {
					if p.UnderlyingMethod != nil {
						inspectedMethod = p.UnderlyingMethod
					}
				}
				return &symgo.SymbolicPlaceholder{Reason: "default intrinsic result"}
			})

			mainFile := findFile(t, pkgs[0], "main.go")
			_, err = interpreter.Eval(ctx, mainFile, pkgs[0])
			if err != nil {
				t.Fatalf("Eval(file) failed: %+v", err)
			}

			fn, ok := interpreter.FindObject("AnonymousInterface")
			if !ok {
				t.Fatal("function AnonymousInterface not found")
			}

			_, err = interpreter.Apply(ctx, fn, []symgo.Object{&symgo.SymbolicPlaceholder{Reason: "test"}}, pkgs[0])
			if err != nil {
				t.Fatalf("Apply failed unexpectedly: %+v", err)
			}

			if inspectedMethod == nil {
				t.Fatal("did not capture an interface method call, UnderlyingMethod was nil")
			}
			if inspectedMethod.Name != "Hex" {
				t.Errorf("expected to capture method 'Hex', but got '%s'", inspectedMethod.Name)
			}
		})

		t.Run("anonymous struct", func(t *testing.T) {
			interpreter, err := symgo.NewInterpreter(s, symgo.WithLogger(logger))
			if err != nil {
				t.Fatalf("NewInterpreter failed: %+v", err)
			}

			mainFile := findFile(t, pkgs[0], "main.go")
			_, err = interpreter.Eval(ctx, mainFile, pkgs[0])
			if err != nil {
				t.Fatalf("Eval(file) failed: %+v", err)
			}

			fn, ok := interpreter.FindObject("AnonymousStruct")
			if !ok {
				t.Fatal("function AnonymousStruct not found")
			}

			result, err := interpreter.Apply(ctx, fn, []symgo.Object{&symgo.SymbolicPlaceholder{Reason: "test"}}, pkgs[0])
			if err != nil {
				t.Fatalf("Apply failed unexpectedly: %+v", err)
			}

			retVal, ok := result.(*object.ReturnValue)
			if !ok {
				t.Fatalf("expected ReturnValue, got %T", result)
			}

			placeholder, ok := retVal.Value.(*symgo.SymbolicPlaceholder)
			if !ok {
				t.Fatalf("expected symbolic placeholder return, got %T", retVal.Value)
			}

			if !strings.Contains(placeholder.Reason, "field access p.X") {
				t.Errorf("expected reason to contain 'field access p.X', but got %q", placeholder.Reason)
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}
