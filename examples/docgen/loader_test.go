package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scantest"
)

func TestPatternKeyFromFunc_Integration(t *testing.T) {
	// This test verifies that the key generation works for both regular functions
	// and method values when they are loaded from a minigo script.
	// It uses scantest to create an in-memory Go module for the test.

	moduleName := "key-gen-test"

	// Get the absolute path to the module root to create a robust replace directive.
	s, err := goscan.New()
	if err != nil {
		t.Fatalf("could not create scanner to find module root: %v", err)
	}
	rootDir := s.RootDir()

	files := map[string]string{
		"go.mod": fmt.Sprintf(`
module %s
go 1.21
replace github.com/podhmo/go-scan => %s
`, moduleName, rootDir),
		"main.go": `
package main

type MyStruct struct{}
func (s *MyStruct) MyMethod() {}
func MyFunc() {}
`,
		"patterns.go": `
//go:build minigo

package main

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

var Patterns = []patterns.PatternConfig{
	{Fn: MyFunc},
	{Fn: (*MyStruct).MyMethod},
}
`,
	}

	tmpdir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// 1. Evaluate the package containing the patterns.go script.
		// We pass "." to evaluate the package in the current module root.
		interp, err := minigo.NewInterpreter(s)
		if err != nil {
			return fmt.Errorf("failed to create interpreter: %w", err)
		}
		if err := interp.EvalPackage("."); err != nil {
			return fmt.Errorf("failed to eval package: %w", err)
		}

		// 2. Extract the evaluated PatternConfig objects.
		patternsObj, ok := interp.GlobalEnvForTest().Get("Patterns")
		if !ok {
			return fmt.Errorf("could not find 'Patterns' variable")
		}

		patternsArray, ok := patternsObj.(*object.Array)
		if !ok {
			return fmt.Errorf("expected 'Patterns' to be an array, got %T", patternsObj)
		}

		var configs []patterns.PatternConfig
		for _, item := range patternsArray.Elements {
			structInstance, ok := item.(*object.StructInstance)
			if !ok {
				continue
			}
			var config patterns.PatternConfig
			if fnObj, ok := structInstance.Fields["Fn"]; ok {
				config.Fn = fnObj
			}
			configs = append(configs, config)
		}
		if len(configs) != 2 {
			return fmt.Errorf("expected 2 pattern configs, got %d", len(configs))
		}

		// 3. Generate keys and verify them.
		wantKeys := []string{
			fmt.Sprintf("%s.MyFunc", moduleName),
			fmt.Sprintf("(*%s.MyStruct).MyMethod", moduleName),
		}

		for i, config := range configs {
			got, err := patternKeyFromFunc(config.Fn)
			if err != nil {
				return fmt.Errorf("patternKeyFromFunc() returned error for pattern %d: %+v", i, err)
			}
			if diff := cmp.Diff(wantKeys[i], got); diff != "" {
				return fmt.Errorf("key mismatch for pattern %d (-want +got):\n%s", i, diff)
			}
		}

		return nil
	}

	if _, err := scantest.Run(t, tmpdir, nil, action, scantest.WithModuleRoot(tmpdir)); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
