package astwalk

import (
	"go/ast"
	"go/parser"
	"go/token"

	// "slices" // For comparing slices if needed, though we primarily collect and compare.
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestToplevelStructs_Iterator(t *testing.T) {
	tests := []struct {
		name              string
		source            string
		expectedSpecNames []string // Expected names of TypeSpecs yielded
		stopAfterN        int      // Test early exit: stop after yielding N items (0 means iterate all)
		expectEarlyExit   bool     // Whether an early exit is expected
	}{
		{
			name:              "nil file",
			source:            "", // Will result in nil file for this specific test setup
			expectedSpecNames: []string{},
		},
		{
			name:              "empty source",
			source:            `package test`,
			expectedSpecNames: []string{},
		},
		{
			name: "only structs",
			source: `
package test
type Struct1 struct{}
type Struct2 struct { Field int }`,
			expectedSpecNames: []string{"Struct1", "Struct2"},
		},
		{
			name: "mixed types",
			source: `
package test
type Struct1 struct{}
type Alias1 int
func Func1() {}
type Interface1 interface{}
type Struct2 struct{}
type (
	Struct3 struct{}
	Alias2 string
)
`,
			expectedSpecNames: []string{"Struct1", "Struct2", "Struct3"},
		},
		{
			name:              "no structs",
			source:            `package test; type Alias1 int; func Func1() {}; type Interface1 interface{}`,
			expectedSpecNames: []string{},
		},
		{
			name: "early exit after 1 struct",
			source: `
package test
type StructA struct{}
type StructB struct{}
type StructC struct{}`,
			expectedSpecNames: []string{"StructA"},
			stopAfterN:        1,
			expectEarlyExit:   true,
		},
		{
			name: "early exit after 2 structs",
			source: `
package test
type S1 struct{}
type S2 struct{}
type S3 struct{}`,
			expectedSpecNames: []string{"S1", "S2"},
			stopAfterN:        2,
			expectEarlyExit:   true,
		},
		{
			name: "stopAfterN larger than actual structs (no early exit)",
			source: `
package test
type X struct{}
type Y struct{}`,
			expectedSpecNames: []string{"X", "Y"},
			stopAfterN:        5,
			expectEarlyExit:   false,
		},
		{
			name: "stopAfterN is 0 (iterate all)",
			source: `
package test
type Alpha struct{}
type Beta struct{}`,
			expectedSpecNames: []string{"Alpha", "Beta"},
			stopAfterN:        0,
			expectEarlyExit:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var file *ast.File
			var fset *token.FileSet
			collectedNames := []string{}
			iterations := 0

			if tt.name == "nil file" {
				// Test with nil file explicitly for the iterator
				// The iterator function itself handles nil file and should not panic.
				// The loop `for range ToplevelStructs(nil, nil)` should simply not execute.
				for range ToplevelStructs(nil, nil) {
					t.Error("Iterator for nil file yielded an item, expected none")
				}
				// Check if expectedSpecNames is also empty for nil file case.
				if len(tt.expectedSpecNames) != 0 {
					t.Errorf("Expected spec names for nil file case should be empty, got %v", tt.expectedSpecNames)
				}
				return // End this test case
			}

			fset = token.NewFileSet()
			var err error
			file, err = parser.ParseFile(fset, "", tt.source, parser.ParseComments)
			if err != nil {
				if tt.source == "package test" && len(tt.expectedSpecNames) == 0 {
					// Valid empty file
				} else {
					t.Fatalf("Failed to parse source: %v", err)
				}
			}

			// Using the range-over-function syntax
			for typeSpec := range ToplevelStructs(fset, file) {
				iterations++
				collectedNames = append(collectedNames, typeSpec.Name.Name)
				if tt.stopAfterN > 0 && iterations == tt.stopAfterN {
					// To test early exit, the `yield` function in the iterator needs to return `false`.
					// The current ToplevelStructs signature is `func(yield func(*ast.TypeSpec) bool)`
					// The range-over-function syntax implies `yield` returns `true` to continue.
					// To stop it, the `yield` itself would have to be designed to return `false`.
					// The test here simulates the *consumer* stopping.
					// The iterator itself stops if `yield` returns false.
					// Let's refine the iterator to accept a yield that can signal stop.
					// No, the current test structure is testing the *consumer* breaking the loop.
					// The `ToplevelStructs` is `func(yield func(V) bool)`.
					// The `for item := range iterFunc` will break if `yield` returns `false`.
					// So, the `stopAfterN` logic should be inside the yield passed to the iterator if we were to test that.
					// However, the current structure is: the test *breaks* the loop.
					// This doesn't directly test if the *iterator* correctly stops if `yield` returns false.
					// Let's adjust the test for `ToplevelStructs` to test its reaction to `yield` returning `false`.

					// For now, this test assumes the consumer breaks.
					// To test the iterator's early exit capability, we'd call it directly:
					// ToplevelStructs(fset, file)(func(ts *ast.TypeSpec) bool { ... return false; })
					break
				}
			}

			if diff := cmp.Diff(tt.expectedSpecNames, collectedNames); diff != "" {
				t.Errorf("ToplevelStructs() yielded unexpected struct names (-want +got):\n%s", diff)
			}

			// Test the iterator's own early exit mechanism
			if tt.expectEarlyExit {
				yieldCount := 0
				ToplevelStructs(fset, file)(func(ts *ast.TypeSpec) bool {
					yieldCount++
					if yieldCount == tt.stopAfterN {
						return false // Signal iterator to stop
					}
					return true
				})
				if yieldCount != tt.stopAfterN {
					t.Errorf("Iterator did not stop early as expected. Expected %d yields, got %d", tt.stopAfterN, yieldCount)
				}
			} else { // Should iterate all if not expecting early exit by iterator's control
				controlYieldCount := 0
				expectedTotalYields := len(tt.expectedSpecNames)
				// If stopAfterN was set but larger than total, it means the consumer would not break early.
				if tt.stopAfterN > 0 && tt.stopAfterN < expectedTotalYields {
					// This case is covered by the main loop test.
				} else {
					ToplevelStructs(fset, file)(func(ts *ast.TypeSpec) bool {
						controlYieldCount++
						return true // Always continue
					})
					if controlYieldCount != len(tt.expectedSpecNames) {
						t.Errorf("Iterator did not yield all items when not signalled to stop early. Expected %d, got %d", len(tt.expectedSpecNames), controlYieldCount)
					}
				}
			}
		})
	}
}

// Helper to collect all items from an iterator for simpler comparison if needed.
func collect[T any](iter func(yield func(T) bool)) []T {
	var items []T
	iter(func(item T) bool {
		items = append(items, item)
		return true
	})
	return items
}

// TestToplevelStructs_Collect demonstrates using a helper to collect results.
func TestToplevelStructs_Collect(t *testing.T) {
	fset := token.NewFileSet()
	source := `package demo; type S1 struct{}; type S2 struct{}; type A int`
	file, err := parser.ParseFile(fset, "", source, parser.ParseComments)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	specs := collect(ToplevelStructs(fset, file))
	if len(specs) != 2 {
		t.Errorf("Expected 2 specs, got %d", len(specs))
	}
	expectedNames := []string{"S1", "S2"}
	gotNames := make([]string, len(specs))
	for i, s := range specs {
		gotNames[i] = s.Name.Name
	}
	if diff := cmp.Diff(expectedNames, gotNames); diff != "" {
		t.Errorf("Collected specs mismatch (-want +got):\n%s", diff)
	}
}

// Ensure slices import is available if used directly, e.g. for slices.Equal
// var _ = slices.Contains
