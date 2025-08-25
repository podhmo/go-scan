package main

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

const testGoMod = `
module key-from-fn

go 1.21

replace github.com/podhmo/go-scan => ../../..
`

const testFooGo = `
package foo

// Foo is a sample struct.
type Foo struct{}

// Bar is a sample method with a pointer receiver.
func (f *Foo) Bar() {}

// Qux is a sample method with a value receiver.
func (f Foo) Qux() {}

// Baz is a standalone function.
func Baz() {}
`

const testPatternsGo = `
//go:build minigo
// +build minigo

package main

import (
	"key-from-fn/foo"
)

// PatternConfig is a local stub for the real patterns.PatternConfig.
type PatternConfig struct {
	Name     string
	Fn       any
	Type     string
	ArgIndex int
}

var (
	// variables for testing instance methods
	v = foo.Foo{}
	p = &foo.Foo{}
	n = new(foo.Foo)
)

var Patterns = []PatternConfig{
	// Method from typed nil
	{
		Name: "pattern-for-method-from-nil",
		Fn:   (*foo.Foo)(nil).Bar,
		Type: "responseBody",
	},
	// Standalone function
	{
		Name: "pattern-for-function",
		Fn:   foo.Baz,
		Type: "responseBody",
	},
	// Method from value instance
	{
		Name: "pattern-for-method-from-value",
		Fn:   v.Qux,
		Type: "responseBody",
	},
	// Method from pointer to struct literal instance
	{
		Name: "pattern-for-method-from-pointer-literal",
		Fn:   p.Bar,
		Type: "responseBody",
	},
	// Method from new() instance
	{
		Name: "pattern-for-method-from-new",
		Fn:   n.Bar,
		Type: "responseBody",
	},
}
`

func TestKeyFromFnWithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod":      testGoMod,
		"foo/foo.go":  testFooGo,
		"patterns.go": testPatternsGo,
	}

	// scantest.WriteFiles creates a temp directory with the specified file layout.
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// The action function where the main test logic resides.
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		logger := newTestLogger(io.Discard)

		patternsPath := filepath.Join(dir, "patterns.go")
		loadedPatterns, err := LoadPatternsFromConfig(patternsPath, logger, s)
		if err != nil {
			t.Fatalf("LoadPatternsFromConfig failed: %+v", err)
		}

		expectedKeys := map[string]bool{
			"key-from-fn/foo.(*Foo).Bar": true, // from nil, pointer literal, and new
			"key-from-fn/foo.Foo.Qux":    true, // from value
			"key-from-fn/foo.Baz":        true, // from standalone function
		}

		if len(loadedPatterns) != 5 {
			t.Fatalf("expected 5 patterns, got %d", len(loadedPatterns))
		}

		foundKeys := make(map[string]bool)
		for _, p := range loadedPatterns {
			foundKeys[p.Key] = true
		}

		if diff := cmp.Diff(expectedKeys, foundKeys); diff != "" {
			t.Errorf("key mismatch (-want +got):\n%s", diff)
			for k := range expectedKeys {
				if !foundKeys[k] {
					t.Errorf("expected key %q was not found", k)
				}
			}
			for k := range foundKeys {
				if !expectedKeys[k] {
					t.Errorf("found unexpected key %q", k)
				}
			}
		}
		return nil // Return nil on success
	}

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}
}
