package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestUnresolvedEmbedded_SelfContained(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m",
		"app/app.go": `
package app
import "example.com/m/cli"
func markerFunc() {}
func NewAppMethod() {
	app := &cli.Application{}
	app.Run() // access embedded method from out-of-policy package
	markerFunc() // this should still be called
	return
}
func NewAppField() string {
	app := &cli.Application{}
	name := app.Name // access embedded field from out-of-policy package
	markerFunc() // this should still be called
	return name
}`,
		"cli/cli.go": `
package cli
import "example.com/m/ext"
type Application struct {
	*ext.Application
}`,
		"ext/ext.go": `
package ext
type Application struct {
	Name string
}
func (app *Application) Run() {}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	t.Run("access method on embedded struct from out-of-policy package", func(t *testing.T) {
		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		calledFunctions := make(map[string]bool)

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			mainPkg := pkgs[0]
			var fnDef *goscan.FunctionInfo
			for _, fn := range mainPkg.Functions {
				if fn.Name == "NewAppMethod" {
					fnDef = fn
					break
				}
			}
			if fnDef == nil {
				return fmt.Errorf("function NewAppMethod not found")
			}

			tracker := func(ctx context.Context, args ...object.Object) object.Object {
				if len(args) > 0 {
					if fn, ok := args[0].(*object.Function); ok {
						if fn.Def != nil {
							key := fn.Def.PkgPath + "." + fn.Def.Name
							calledFunctions[key] = true
						}
					}
				}
				return nil
			}

			evaluator := New(s, slog.New(h), nil, func(path string) bool {
				return path != "example.com/m/ext" // ext is out-of-policy
			})
			evaluator.RegisterDefaultIntrinsic(tracker)

			appPkgObj, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
			if err != nil {
				return fmt.Errorf("could not load package: %w", err)
			}
			fnObj := evaluator.GetOrResolveFunctionForTest(ctx, appPkgObj, fnDef)

			result := evaluator.Apply(ctx, fnObj, nil, mainPkg)
			if err, ok := result.(*object.Error); ok {
				return fmt.Errorf("got unexpected error, but want success: %+v", err.Error())
			}

			var logEntry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				return fmt.Errorf("failed to unmarshal log output: %v\noutput: %s", err, buf.String())
			}

			expectedMsg := "assuming method exists on unresolved embedded type"
			if msg, _ := logEntry["msg"].(string); msg != expectedMsg {
				return fmt.Errorf("unexpected log message: got %q, want %q", msg, expectedMsg)
			}

			expectedMethodName := "Run"
			if name, _ := logEntry["method_name"].(string); name != expectedMethodName {
				return fmt.Errorf("unexpected method_name: got %q, want %q", name, expectedMethodName)
			}

			if !calledFunctions["example.com/m/app.markerFunc"] {
				return fmt.Errorf("expected markerFunc to be called, but it was not")
			}
			return nil
		}

		_, err := scantest.Run(t, t.Context(), dir, []string{"example.com/m/app"}, action, scantest.WithModuleRoot(dir))
		if err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}
	})

	t.Run("access field on embedded struct from out-of-policy package", func(t *testing.T) {
		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		calledFunctions := make(map[string]bool)

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			mainPkg := pkgs[0]
			var fnDef *goscan.FunctionInfo
			for _, fn := range mainPkg.Functions {
				if fn.Name == "NewAppField" {
					fnDef = fn
					break
				}
			}
			if fnDef == nil {
				return fmt.Errorf("function NewAppField not found")
			}

			tracker := func(ctx context.Context, args ...object.Object) object.Object {
				if len(args) > 0 {
					if fn, ok := args[0].(*object.Function); ok {
						if fn.Def != nil {
							key := fn.Def.PkgPath + "." + fn.Def.Name
							calledFunctions[key] = true
						}
					}
				}
				return nil
			}

			evaluator := New(s, slog.New(h), nil, func(path string) bool {
				return path != "example.com/m/ext" // ext is out-of-policy
			})
			evaluator.RegisterDefaultIntrinsic(tracker)

			appPkgObj, err := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
			if err != nil {
				return fmt.Errorf("could not load package: %w", err)
			}
			fnObj := evaluator.GetOrResolveFunctionForTest(ctx, appPkgObj, fnDef)

			result := evaluator.Apply(ctx, fnObj, nil, mainPkg)
			if err, ok := result.(*object.Error); ok {
				return fmt.Errorf("got unexpected error, but want success: %+v", err.Error())
			}

			var logEntry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				return fmt.Errorf("failed to unmarshal log output: %v\noutput: %s", err, buf.String())
			}

			expectedMsg := "assuming field exists on unresolved embedded type"
			if msg, _ := logEntry["msg"].(string); msg != expectedMsg {
				return fmt.Errorf("unexpected log message: got %q, want %q", msg, expectedMsg)
			}

			expectedFieldName := "Name"
			if name, _ := logEntry["field_name"].(string); name != expectedFieldName {
				return fmt.Errorf("unexpected field_name: got %q, want %q", name, expectedFieldName)
			}

			if !calledFunctions["example.com/m/app.markerFunc"] {
				return fmt.Errorf("expected markerFunc to be called, but it was not")
			}
			return nil
		}
		_, err := scantest.Run(t, t.Context(), dir, []string{"example.com/m/app"}, action, scantest.WithModuleRoot(dir))
		if err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}
	})
}