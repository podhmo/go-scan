package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/scantest"
)

func TestPatternKeyFromFunc_Integration(t *testing.T) {
	// This test verifies that the key generation works for both regular functions
	// and method values when they are loaded from a minigo script.
	// It uses scantest to create an in-memory Go module for the test.

	moduleName := "github.com/podhmo/go-scan/examples/docgen/testdata/key-gen-test"

	files := map[string]string{
		"go.mod": fmt.Sprintf(`
module %s
go 1.21
replace github.com/podhmo/go-scan => ../../../../
`, moduleName),
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

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

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
		var configs []patterns.PatternConfig
		result := minigo.Result{Value: patternsObj}
		// We must unmarshal manually because `Fn` is an `any` holding a minigo object.
		patternsArray, ok := patternsObj.(*minigo.Array)
		if !ok {
			return fmt.Errorf("expected 'Patterns' to be an array, got %T", patternsObj)
		}
		for _, item := range patternsArray.Elements {
			structInstance, ok := item.(*minigo.StructInstance)
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

	if _, err := scantest.Run(t, ".", files, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
