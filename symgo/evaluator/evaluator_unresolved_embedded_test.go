package evaluator

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestUnresolvedEmbedded_SelfContained(t *testing.T) {
	sources := []scantest.SourceFile{
		{
			Name: "example.com/m/app/app.go",
			Code: `
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
}
`,
		},
		{
			Name: "example.com/m/cli/cli.go",
			Code: `
package cli

import "example.com/m/ext"

type Application struct {
	*ext.Application
}
`,
		},
		{
			Name: "example.com/m/ext/ext.go",
			Code: `
package ext

type Application struct {
	Name string
}

func (app *Application) Run() {}
`,
		},
	}

	t.Run("access method on embedded struct from out-of-policy package", func(t *testing.T) {
		s := scantest.NewScanner(t, sources, "")
		pkg := s.RequirePackage("example.com/m/app")
		fn := pkg.RequireFunction("NewAppMethod")

		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})

		// Setup for tracking function calls
		calledFunctions := make(map[string]bool)
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

		evaluator := New(s.Scanner, slog.New(h), nil, func(path string) bool {
			return path != "example.com/m/ext" // ext is out-of-policy
		})
		evaluator.RegisterDefaultIntrinsic(tracker)

		ctx := context.Background()
		appPkgObj, err := evaluator.getOrLoadPackage(ctx, pkg.ImportPath)
		if err != nil {
			t.Fatalf("could not load package: %v", err)
		}
		fnObj := evaluator.getOrResolveFunction(ctx, appPkgObj, fn)

		result := evaluator.Apply(ctx, fnObj, nil, pkg)
		if err, ok := result.(*object.Error); ok {
			t.Fatalf("got unexpected error, but want success: %+v", err.Error())
		}

		wantLog := `level=WARN msg="assuming method exists on unresolved embedded type" method_name=Run`
		if !strings.Contains(buf.String(), wantLog) {
			t.Errorf("did not find warning log entry\n  want: %q\n  logs:\n%s", wantLog, buf.String())
		}

		if !calledFunctions["example.com/m/app.markerFunc"] {
			t.Errorf("expected markerFunc to be called, but it was not")
		}
	})

	t.Run("access field on embedded struct from out-of-policy package", func(t *testing.T) {
		s := scantest.NewScanner(t, sources, "")
		pkg := s.RequirePackage("example.com/m/app")
		fn := pkg.RequireFunction("NewAppField")

		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})

		calledFunctions := make(map[string]bool)
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

		evaluator := New(s.Scanner, slog.New(h), nil, func(path string) bool {
			return path != "example.com/m/ext" // ext is out-of-policy
		})
		evaluator.RegisterDefaultIntrinsic(tracker)

		ctx := context.Background()
		appPkgObj, err := evaluator.getOrLoadPackage(ctx, pkg.ImportPath)
		if err != nil {
			t.Fatalf("could not load package: %v", err)
		}
		fnObj := evaluator.getOrResolveFunction(ctx, appPkgObj, fn)

		result := evaluator.Apply(ctx, fnObj, nil, pkg)
		if err, ok := result.(*object.Error); ok {
			t.Fatalf("got unexpected error, but want success: %+v", err.Error())
		}

		wantLog := `level=WARN msg="assuming field exists on unresolved embedded type" field_name=Name`
		if !strings.Contains(buf.String(), wantLog) {
			t.Errorf("did not find warning log entry\n  want: %q\n  logs:\n%s", wantLog, buf.String())
		}

		if !calledFunctions["example.com/m/app.markerFunc"] {
			t.Errorf("expected markerFunc to be called, but it was not")
		}
	})
}