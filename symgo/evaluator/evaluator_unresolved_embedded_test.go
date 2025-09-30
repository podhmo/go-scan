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

func TestUnresolvedEmbedded_IncompletePath(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m",
		"app/app.go": `
package app
import "example.com/m/cli"
func markerFunc() {}
func RunApp() {
	app := &cli.Application{}
	app.DoSomething() // This should trigger the warning due to incomplete path
	markerFunc()
}`,
		"cli/cli.go": `
package cli
import "example.com/m/ext"
type Application struct {
	*ext.Thing
}`,
		"ext/ext.go": `
package ext
type Thing struct{}
func (t *Thing) DoSomething() {}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	t.Run("should warn when embedded field has incomplete type info", func(t *testing.T) {
		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		calledFunctions := make(map[string]bool)

		action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
			// Find the cli.Application type and tamper with its embedded field's type info
			// to simulate a scanner failure where the import path is missing.
			var cliPkg *goscan.Package
			for _, p := range pkgs {
				if p.ImportPath == "example.com/m/cli" {
					cliPkg = p
					break
				}
			}
			if cliPkg == nil {
				return fmt.Errorf("package 'cli' not found in scan results")
			}
			var cliAppType *goscan.TypeInfo
			for _, typ := range cliPkg.Types {
				if typ.Name == "Application" {
					cliAppType = typ
					break
				}
			}
			if cliAppType == nil {
				return fmt.Errorf("type 'Application' not found in package 'cli'")
			}
			embeddedField := cliAppType.Struct.Fields[0]

			// *** The core of the test: simulate incomplete type info ***
			embeddedField.Type.FullImportPath = ""

			// Now run the analysis with the tampered data.
			var mainPkg *goscan.Package
			for _, p := range pkgs {
				if p.ImportPath == "example.com/m/app" {
					mainPkg = p
					break
				}
			}
			if mainPkg == nil {
				return fmt.Errorf("package 'app' not found")
			}
			var fnDef *goscan.FunctionInfo
			for _, fn := range mainPkg.Functions {
				if fn.Name == "RunApp" {
					fnDef = fn
					break
				}
			}
			if fnDef == nil {
				return fmt.Errorf("function 'RunApp' not found")
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

			evaluator := New(s, slog.New(h), nil, func(path string) bool { return true })
			evaluator.RegisterDefaultIntrinsic(tracker)

			appPkgObj, _ := evaluator.GetOrLoadPackageForTest(ctx, mainPkg.ImportPath)
			fnObj := evaluator.GetOrResolveFunctionForTest(ctx, appPkgObj, fnDef)

			if result := evaluator.Apply(ctx, fnObj, nil, mainPkg); isError(result) {
				return fmt.Errorf("got unexpected error: %+v", result.(*object.Error).Error())
			}

			var logEntry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				return fmt.Errorf("failed to unmarshal log output: %v\noutput: %s", err, buf.String())
			}

			expectedMsg := "assuming method exists on unresolved embedded type"
			if msg, _ := logEntry["msg"].(string); msg != expectedMsg {
				return fmt.Errorf("unexpected log message: got %q, want %q", msg, expectedMsg)
			}
			if !calledFunctions["example.com/m/app.markerFunc"] {
				return fmt.Errorf("expected markerFunc to be called")
			}
			return nil
		}
		_, err := scantest.Run(t, t.Context(), dir, []string{"example.com/m/app", "example.com/m/cli"}, action, scantest.WithModuleRoot(dir))
		if err != nil {
			t.Fatalf("scantest.Run() failed: %v", err)
		}
	})
}