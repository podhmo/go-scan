package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestDocgen_WithFnPatterns(t *testing.T) {
	// This test reproduces the scenario from docs/trouble-docgen-minigo-import.md.
	// It verifies that docgen can load a minigo configuration script (`patterns.go`)
	// from a nested Go module, and that this script can successfully import
	// another package (`api`) from within that same nested module.
	// This relies on the `replace` directive in the nested module's go.mod
	// being correctly handled by the go-scan locator.

	files := map[string]string{
		// The parent module, representing the main go-scan project.
		// The path in the replace directive (`../../`) is relative to the nested module.
		"go.mod": "module github.com/podhmo/go-scan\n\ngo 1.24\n",
		// The `patterns` package that the minigo script will import.
		// This needs to exist within the test's file structure.
		"patterns/patterns.go": `
// Package patterns defines the extensible call patterns for the docgen tool.
package patterns

import (
	"context"
	"fmt"
	"strings"

	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
)

// Analyzer is a subset of the docgen.Analyzer interface needed by patterns.
// This avoids a circular dependency.
type Analyzer interface {
	OperationStack() []*openapi.Operation
	GetOpenAPI() *openapi.OpenAPI
}

// PatternType defines the type of analysis to perform for a custom pattern.
type PatternType string

const (
	RequestBody     PatternType = "requestBody"
	ResponseBody    PatternType = "responseBody"
	CustomResponse  PatternType = "customResponse"
	DefaultResponse PatternType = "defaultResponse"
	PathParameter   PatternType = "path"
	QueryParameter  PatternType = "query"
	HeaderParameter PatternType = "header"
)

// PatternConfig defines a user-configurable pattern for docgen analysis.
type PatternConfig struct {
	Key           string
	Type          PatternType
	ArgIndex      int
	StatusCode    string
	Description   string
	NameArgIndex  int
	// A dummy field to make this struct different from the one in the main package
	// This helps verify that the correct type is being resolved.
	IsTestPattern bool
}
`,

		// The nested module directory.
		"testdata/fn-patterns/go.mod": `
module github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns

go 1.24

replace github.com/podhmo/go-scan => ../../../..
replace github.com/podhmo/go-scan/examples/docgen => ../..
`,
		// The minigo script that imports a local package.
		"testdata/fn-patterns/patterns.go": `
package patterns

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
	"github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api"
)

var Patterns = []patterns.PatternConfig{
	{
		Key:      "github.com/podhmo/go-scan/examples/docgen/testdata/fn-patterns/api.GetFoo",
		Type:     patterns.RequestBody,
		ArgIndex: 1,
	},
}
`,
		// The local package to be imported.
		"testdata/fn-patterns/api/api.go": `
package api

import "net/http"

type Foo struct {
	Name string
}

func GetFoo(w http.ResponseWriter, r *http.Request, foo Foo) {}
`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// The module we want to operate within is the nested one.
	moduleDir := filepath.Join(dir, "testdata/fn-patterns")
	patternsFile := filepath.Join(moduleDir, "patterns.go")

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// The scanner `s` is pre-configured by scantest.Run to have its
		// WorkDir set to `moduleDir`, with a go.mod overlay that resolves
		// the relative `replace` path.
		logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

		// Call the docgen loader function, which will use the provided scanner.
		_, err := LoadPatternsFromConfig(patternsFile, logger, s)
		return err // If err is nil, the import was successful.
	}

	// Use scantest.Run to drive the test.
	// It's crucial to set the module root to `moduleDir` so the scanner
	// finds the correct `go.mod` and resolves the `replace` directive.
	if _, err := scantest.Run(t, moduleDir, nil, action, scantest.WithModuleRoot(moduleDir)); err != nil {
		t.Fatalf("scantest.Run() failed, indicating a failure in loading patterns: %+v", err)
	}
}
