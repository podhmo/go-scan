package evaluator_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestUnresolvedEmbedded(t *testing.T) {
	t.Run("access method on embedded struct from out-of-policy package", func(t *testing.T) {
		r := symgotest.NewRunner()
		pkg := r.Scanned.RequirePackage("example.com/m/app")
		fnInfo := pkg.RequireFunction("NewAppMethod")

		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		evaluator := evaluator.New(r.Scanner, slog.New(h), nil, func(path string) bool {
			return path != "example.com/m/ext" // ext is out-of-policy
		})

		ctx := context.Background()
		appPkgObj, err := evaluator.GetOrLoadPackageForTest(ctx, pkg.ImportPath)
		if err != nil {
			t.Fatalf("could not load package: %v", err)
		}
		fnObj := evaluator.GetOrResolveFunctionForTest(ctx, appPkgObj, fnInfo)

		result := evaluator.Apply(ctx, fnObj, nil, pkg)
		if err, ok := result.(*object.Error); ok {
			t.Fatalf("got unexpected error, but want success: %+v", err)
		}

		// check for warning log
		wantLog := `level=WARN msg="assuming method exists on unresolved embedded type" method_name=Run type_name=Application`
		if !strings.Contains(buf.String(), wantLog) {
			t.Errorf("did not find log entry\n  want: %q\n  logs:\n%s", wantLog, buf.String())
		}
	})

	t.Run("access field on embedded struct from out-of-policy package", func(t *testing.T) {
		r := symgotest.NewRunner()
		pkg := r.Scanned.RequirePackage("example.com/m/app")
		fnInfo := pkg.RequireFunction("NewAppField")

		var buf bytes.Buffer
		h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		evaluator := evaluator.New(r.Scanner, slog.New(h), nil, func(path string) bool {
			return path != "example.com/m/ext" // ext is out-of-policy
		})

		ctx := context.Background()
		appPkgObj, err := evaluator.GetOrLoadPackageForTest(ctx, pkg.ImportPath)
		if err != nil {
			t.Fatalf("could not load package: %v", err)
		}
		fnObj := evaluator.GetOrResolveFunctionForTest(ctx, appPkgObj, fnInfo)

		result := evaluator.Apply(ctx, fnObj, nil, pkg)
		if err, ok := result.(*object.Error); ok {
			t.Fatalf("got unexpected error, but want success: %+v", err)
		}

		// check for warning log
		wantLog := `level=WARN msg="assuming field exists on unresolved embedded type" field_name=Name type_name=Application`
		if !strings.Contains(buf.String(), wantLog) {
			t.Errorf("did not find log entry\n  want: %q\n  logs:\n%s", wantLog, buf.String())
		}
	})
}

func init() {
	symgotest.AddPkg(
		"example.com/m/app",
		`
package app

import "example.com/m/cli"

func NewAppMethod() {
	app := &cli.Application{}
	app.Run()
	return
}

func NewAppField() string {
	app := &cli.Application{}
	return app.Name
}
`,
	)
	symgotest.AddPkg(
		"example.com/m/cli",
		`
package cli

import "example.com/m/ext"

type Application struct {
	*ext.Application
}
`,
	)
	symgotest.AddPkg(
		"example.com/m/ext",
		`
package ext

type Application struct {
	Name string
}

func (app *Application) Run() {}
`,
	)
}