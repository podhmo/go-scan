package minigo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scantest"
)

func TestMinigo_Scantest_BasicImport(t *testing.T) {
	files := map[string]string{
		"go.mod": "module mytest\n\ngo 1.24\n",
		"helper/helper.go": `package helper

func Greet() string {
	return "hello from helper"
}`,
		"main.mgo": `package main

import "mytest/helper"

var result = helper.Greet()
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		interp, err := NewInterpreter(s)
		if err != nil {
			return err
		}

		mainMgoPath := filepath.Join(dir, "main.mgo")
		source, err := os.ReadFile(mainMgoPath)
		if err != nil {
			return err
		}

		if err := interp.LoadFile("main.mgo", source); err != nil {
			return err
		}

		if _, err := interp.Eval(ctx); err != nil {
			return err
		}

		val, ok := interp.globalEnv.Get("result")
		if !ok {
			return fmt.Errorf("variable 'result' not found")
		}

		got, ok := val.(*object.String)
		if !ok {
			return fmt.Errorf("result is not a string, got=%s", val.Type())
		}

		want := "hello from helper"
		if diff := cmp.Diff(want, got.Value); diff != "" {
			return fmt.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		return nil
	}

	// We use scantest.Run to get the properly configured scanner.
	// We don't need to specify patterns for scantest itself, as minigo will trigger the scanning.
	// The module root will be correctly identified as `dir`.
	if _, err := scantest.Run(t, nil, dir, nil, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}

func TestMinigo_Scantest_NestedModuleImportWithReplace(t *testing.T) {
	files := map[string]string{
		"go.mod": "module my-root\n\ngo 1.24\n",
		"rootpkg/root.go": `package rootpkg
func GetVersion() string {
	return "v1.0.0"
}`,
		"nested/go.mod": "module my-nested-module\n\ngo 1.24\n\nreplace my-root => ../\n",
		"nested/main.mgo": `package main
import "my-root/rootpkg"
var result = rootpkg.GetVersion()
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	nestedDir := filepath.Join(dir, "nested")

	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		interp, err := NewInterpreter(s)
		if err != nil {
			return err
		}

		mainMgoPath := filepath.Join(nestedDir, "main.mgo")
		source, err := os.ReadFile(mainMgoPath)
		if err != nil {
			return err
		}

		if err := interp.LoadFile("main.mgo", source); err != nil {
			return err
		}

		if _, err := interp.Eval(ctx); err != nil {
			return err
		}

		val, ok := interp.globalEnv.Get("result")
		if !ok {
			return fmt.Errorf("variable 'result' not found")
		}

		got, ok := val.(*object.String)
		if !ok {
			return fmt.Errorf("result is not a string, got=%s", val.Type())
		}

		want := "v1.0.0"
		if diff := cmp.Diff(want, got.Value); diff != "" {
			return fmt.Errorf("result mismatch (-want +got):\n%s", diff)
		}
		return nil
	}

	// Here, we explicitly tell scantest.Run to use the `nestedDir` as the module root
	// for the scanner it creates. This ensures the scanner finds `nested/go.mod`
	// and correctly interprets the relative `replace` directive.
	if _, err := scantest.Run(t, nil, nestedDir, nil, action, scantest.WithModuleRoot(nestedDir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
