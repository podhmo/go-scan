package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"go/ast"
	"go/token"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/examples/docgen/openapi"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"gopkg.in/yaml.v3"
)

var (
	update = flag.Bool("update", false, "update golden files")
	debug  = flag.Bool("debug", false, "enable debug logging")
)

func newTestLogger(w io.Writer) *slog.Logger {
	level := slog.LevelWarn
	if *debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func TestDocgen(t *testing.T) {
	const sampleAPIPath = "github.com/podhmo/go-scan/examples/docgen/sampleapi"

	// Setup: Run the analysis once and reuse the result for all sub-tests.
	logger := newTestLogger(os.Stderr)
	s, err := goscan.New(
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}
	analyzer, err := NewAnalyzer(s, logger) // no options needed for the default test
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, sampleAPIPath, "NewServeMux"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Define test cases for each format
	testCases := []struct {
		format      string
		goldenFile  string
		marshalFunc func(io.Writer, *openapi.OpenAPI) error
	}{
		{
			format:     "yaml",
			goldenFile: "golden.yaml",
			marshalFunc: func(w io.Writer, spec *openapi.OpenAPI) error {
				enc := yaml.NewEncoder(w)
				return enc.Encode(spec)
			},
		},
		{
			format:     "json",
			goldenFile: "golden.json",
			marshalFunc: func(w io.Writer, spec *openapi.OpenAPI) error {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(spec)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			var got bytes.Buffer
			if err := tc.marshalFunc(&got, apiSpec); err != nil {
				t.Fatalf("failed to marshal OpenAPI spec to %s: %v", tc.format, err)
			}

			goldenPath := filepath.Join("testdata", tc.goldenFile)
			if *update {
				if err := os.WriteFile(goldenPath, got.Bytes(), 0644); err != nil {
					t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
				}
				t.Logf("golden file updated: %s", goldenPath)
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				// If the file doesn't exist and we're not in update mode,
				// fail with a helpful message.
				if os.IsNotExist(err) && !*update {
					t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenPath)
				}
				t.Fatalf("failed to read golden file %s: %v", goldenPath, err)
			}

			if diff := cmp.Diff(string(want), got.String()); diff != "" {
				t.Errorf("OpenAPI spec mismatch for %s (-want +got):\n%s", tc.format, diff)
			}
		})
	}
}

func TestDocgen_withCustomPatterns(t *testing.T) {
	const (
		gomod = `
module custom-patterns

go 1.21

replace github.com/podhmo/go-scan => ../../../../
`
		apiGo = `
package main

import (
	"encoding/json"
	"net/http"
)

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// SendJSON is a custom helper to send a JSON response.
func SendJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// GetUser handles retrieving a user.
func GetUser(w http.ResponseWriter, r *http.Request) {
	user := User{ID: 1, Name: "John Doe"}
	SendJSON(w, http.StatusOK, user)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", GetUser)
	mux.HandleFunc("GET /pets/{petID}", GetPet)
	http.ListenAndServe(":8080", mux)
}

// GetPet is a handler that uses a custom helper to get a path parameter.
func GetPet(w http.ResponseWriter, r *http.Request) {
	_ = GetPetID(r) // The important part for the analyzer
	w.Write([]byte("ok"))
}

// GetPetID is a custom helper function to extract a path parameter.
func GetPetID(r *http.Request) string {
	// In a real app, this would parse the URL.
	return "pet-id"
}
`
		patternsGo = `
//go:build minigo

package main

import "github.com/podhmo/go-scan/examples/docgen/patterns"

// Patterns defines the custom patterns for this test case.
// It now uses the 'Fn' field for type-safe references.
var Patterns = []patterns.PatternConfig{
	{
		Fn:       SendJSON,
		Type:     patterns.ResponseBody,
		ArgIndex: 2, // The 3rd argument `data any` is what we want to analyze.
	},
	{
		// Note: The 'Name' field is no longer part of the key, but a separate field
		// used by the HandleCustomParameter logic.
		Fn:          GetPetID,
		Type:        patterns.PathParameter,
		Name:        "petID",
		Description: "ID of the pet to fetch",
		ArgIndex:    0, // The *http.Request argument
	},
}
`
	)

	scantest.Run(t, scantest.TestCase{
		Gomod: gomod,
		Files: map[string]string{
			"api.go":      apiGo,
			"patterns.go": patternsGo,
		},
		Action: func(t *testing.T, s *scantest.Scanner) {
			logger := newTestLogger(io.Discard)
			goldenFile := filepath.Join("testdata", "custom-patterns.golden.json")
			apiPath := "custom-patterns"      // module name
			patternsPath := "patterns.go" // path relative to temp module root

			customPatterns, err := LoadPatternsFromConfig(patternsPath, logger, s)
			if err != nil {
				t.Fatalf("failed to load custom patterns: %v", err)
			}

			var opts []any
			for _, p := range customPatterns {
				opts = append(opts, p)
			}
			analyzer, err := NewAnalyzer(s, logger, opts...)
			if err != nil {
				t.Fatalf("failed to create analyzer: %v", err)
			}

			if err := analyzer.Analyze(context.Background(), apiPath, "main"); err != nil {
				t.Fatalf("failed to analyze package: %+v", err)
			}
			apiSpec := analyzer.OpenAPI

			var got bytes.Buffer
			enc := json.NewEncoder(&got)
			enc.SetIndent("", "  ")
			if err := enc.Encode(apiSpec); err != nil {
				t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
			}

			if *update {
				if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
					t.Fatalf("failed to write golden file %s: %v", goldenFile, err)
				}
				t.Logf("golden file updated: %s", goldenFile)
				return
			}
			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
			}
			if diff := cmp.Diff(string(want), got.String()); diff != "" {
				t.Errorf("OpenAPI spec mismatch for custom patterns (-want +got):\n%s", diff)
			}
		},
	})
}

func TestDocgen_fullParameters(t *testing.T) {
	const (
		gomod = `
module full-parameters

go 1.21

replace github.com/podhmo/go-scan => ../../../../
`
		apiGo = `
package main

import (
	"net/http"
)

// Helper functions to be recognized by custom patterns.
func GetQueryParam(r *http.Request, key string) string {
	return r.URL.Query().Get(key)
}

func GetHeader(r *http.Request, key string) string {
	return r.Header.Get(key)
}

func GetPathValue(r *http.Request, key string) string {
	// In a real app, this would parse the URL path.
	// For analysis, the implementation doesn't matter.
	return "some-value"
}

// Handler that uses all parameter types.
func GetResource(w http.ResponseWriter, r *http.Request) {
	// These calls will be detected by the custom patterns.
	_ = GetPathValue(r, "resourceId")
	_ = GetQueryParam(r, "filter")
	_ = GetHeader(r, "X-Request-ID")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	// Note: The path parameter name in the route must match the one in the pattern.
	mux.HandleFunc("GET /resources/{resourceId}", GetResource)
	http.ListenAndServe(":8080", mux)
}
`
		patternsGo = `
//go:build minigo

package main

import (
	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

// Patterns defines the custom patterns for this test case.
// This version uses dynamic name inference.
var Patterns = []patterns.PatternConfig{
	{
		Fn:           GetQueryParam,
		Type:         patterns.QueryParameter,
		NameArgIndex: 1, // The 'key' argument
		ArgIndex:     0, // Dummy value, schema will default to string
		Description:  "A filter for the resource list.",
	},
	{
		Fn:           GetHeader,
		Type:         patterns.HeaderParameter,
		NameArgIndex: 1,
		ArgIndex:     0,
		Description:  "A unique ID for the request.",
	},
	{
		Fn:           GetPathValue,
		Type:         patterns.PathParameter,
		NameArgIndex: 1,
		ArgIndex:     0,
		Description:  "The ID of the resource.",
	},
}
`
	)
	scantest.Run(t, scantest.TestCase{
		Gomod: gomod,
		Files: map[string]string{
			"api.go":      apiGo,
			"patterns.go": patternsGo,
		},
		Action: func(t *testing.T, s *scantest.Scanner) {
			logger := newTestLogger(io.Discard)
			goldenFile := filepath.Join("testdata", "full-parameters.golden.json")
			apiPath := "full-parameters"      // module name
			patternsPath := "patterns.go" // path relative to temp module root

			type recordingTracer struct {
				visitedNodePositions map[token.Pos]bool
			}
			tracer := &recordingTracer{visitedNodePositions: make(map[token.Pos]bool)}
			visit := func(node ast.Node) {
				if node != nil {
					tracer.visitedNodePositions[node.Pos()] = true
				}
			}

			customPatterns, err := LoadPatternsFromConfig(patternsPath, logger, s)
			if err != nil {
				t.Fatalf("failed to load custom patterns: %v", err)
			}

			var opts []any
			for _, p := range customPatterns {
				opts = append(opts, p)
			}
			opts = append(opts, WithTracer(symgo.TracerFunc(visit)))
			analyzer, err := NewAnalyzer(s, logger, opts...)
			if err != nil {
				t.Fatalf("failed to create analyzer: %v", err)
			}

			if err := analyzer.Analyze(context.Background(), apiPath, "main"); err != nil {
				t.Fatalf("failed to analyze package: %+v", err)
			}
			apiSpec := analyzer.OpenAPI

			// Assert that the tracer visited all nodes in the target handler.
			scannedPkg, err := s.ScanPackage(context.Background(), ".")
			if err != nil {
				t.Fatalf("could not re-scan package to find handler AST: %v", err)
			}
			var handlerFunc *goscan.FunctionInfo
			for _, f := range scannedPkg.Functions {
				if f.Name == "GetResource" {
					handlerFunc = f
					break
				}
			}
			if handlerFunc == nil {
				t.Fatalf("could not find handler function 'GetResource' to verify tracer")
			}
			var targetNode ast.Node
			ast.Inspect(handlerFunc.AstDecl, func(n ast.Node) bool {
				if targetNode != nil {
					return false
				}
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.Ident); ok && sel.Name == "GetPathValue" {
						targetNode = call
						return false
					}
				}
				return true
			})
			if targetNode == nil {
				t.Fatalf("could not find the target GetPathValue call in the AST")
			}
			if !tracer.visitedNodePositions[targetNode.Pos()] {
				t.Fatalf("tracer did not visit the target GetPathValue call expression")
			}

			var got bytes.Buffer
			enc := json.NewEncoder(&got)
			enc.SetIndent("", "  ")
			if err := enc.Encode(apiSpec); err != nil {
				t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
			}

			if *update {
				if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
					t.Fatalf("failed to write golden file %s: %v", goldenFile, err)
				}
				t.Logf("golden file updated: %s", goldenFile)
				return
			}
			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
			}
			if diff := cmp.Diff(string(want), got.String()); diff != "" {
				t.Errorf("OpenAPI spec mismatch for full parameters (-want +got):\n%s", diff)
			}
		},
	})
}

func TestDocgen_newFeatures(t *testing.T) {
	const (
		gomod = `
module new-features

go 1.21

replace github.com/podhmo/go-scan => ../../../../
`
		helpersGo = `
package helpers

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is a generic error response structure.
type ErrorResponse struct {
	Error string `json:"error"`
}

// RenderError is a helper to write a JSON error response with a specific status code.
// Our custom pattern will target this function.
func RenderError(w http.ResponseWriter, r *http.Request, status int, err error) {
	w.WriteHeader(status)
	response := ErrorResponse{Error: err.Error()}
	json.NewEncoder(w).Encode(response)
}

// RenderJSON is a generic helper to write a JSON response.
// We will use this to test map and other struct responses.
func RenderJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// RenderCustomError is a helper for testing the CustomResponse pattern.
func RenderCustomError(w http.ResponseWriter, r *http.Request, err ErrorResponse) {
	w.WriteHeader(http.StatusBadRequest) // 400
	json.NewEncoder(w).Encode(err)
}
`
		apiGo = `
package main

import (
	"net/http"
	"new-features/helpers" // Use the correct module path
)

// User represents a user in the system.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Settings represents some configuration settings.
type Settings struct {
	Options map[string]any `json:"options"`
}

// GetSettingsHandler returns the current application settings.
// This handler is designed to test map[string]any support.
func GetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	settings := Settings{
		Options: map[string]any{
			"feature_a": true,
			"retries":   3,
			"theme":     "dark",
		},
	}
	helpers.RenderJSON(w, http.StatusOK, settings)
}

// GetUserHandler returns a user by ID.
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
	// This branch is for the error case, to be detected by our custom pattern.
	if r.URL.Query().Get("error") == "true" {
		helpers.RenderError(w, r, http.StatusNotFound, &helpers.ErrorResponse{Error: "User not found"})
		return
	}

	user := User{ID: "123", Name: "John Doe"}
	helpers.RenderJSON(w, http.StatusOK, user)
}

// CreateThingHandler is for testing custom status code responses.
func CreateThingHandler(w http.ResponseWriter, r *http.Request) {
	// some validation logic...
	if r.URL.Query().Get("fail") == "true" {
		helpers.RenderCustomError(w, r, helpers.ErrorResponse{Error: "Invalid input"})
		return
	}
	// Happy path not implemented for this example
}

// main is the entrypoint for the application.
// docgen will start its analysis from here.
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", GetSettingsHandler)
	mux.HandleFunc("GET /users/{id}", GetUserHandler) // Path parameter just for show
	mux.HandleFunc("POST /things", CreateThingHandler)
	http.ListenAndServe(":8080", mux)
}
`
		patternsGo = `
//go:build minigo
// +build minigo

package main

import (
	"new-features/helpers"

	"github.com/podhmo/go-scan/examples/docgen/patterns"
)

// Patterns defines a list of custom patterns for the docgen tool.
var Patterns = []patterns.PatternConfig{
	{
		Fn:       helpers.RenderError,
		Type:     patterns.DefaultResponse,
		ArgIndex: 3, // err error
	},
	{
		Fn:         helpers.RenderCustomError,
		Type:       patterns.CustomResponse,
		StatusCode: "400",
		ArgIndex:   2, // err helpers.ErrorResponse
	},
	{
		Fn:       helpers.RenderJSON,
		Type:     patterns.ResponseBody,
		ArgIndex: 2, // The `v any` argument
	},
}
`
	)

	scantest.Run(t, scantest.TestCase{
		Gomod: gomod,
		Files: map[string]string{
			"helpers/helpers.go": helpersGo,
			"main/api.go":        apiGo,
			"patterns.go":        patternsGo,
		},
		WorkDir: "main", // Set the working directory to the 'main' package folder
		Action: func(t *testing.T, s *scantest.Scanner) {
			logger := newTestLogger(io.Discard)
			// The golden file path is relative to the original test file location.
			goldenFile := filepath.Join("../testdata/new-features", "new-features.golden.json")
			apiPath := "new-features/main" // module name + package dir
			// Patterns file is now at the root of the temp module, but our WorkDir is 'main',
			// so we need to go up one level.
			patternsPath := "../patterns.go"

			customPatterns, err := LoadPatternsFromConfig(patternsPath, logger, s)
			if err != nil {
				t.Fatalf("failed to load custom patterns: %v", err)
			}

			var opts []any
			for _, p := range customPatterns {
				opts = append(opts, p)
			}
			analyzer, err := NewAnalyzer(s, logger, opts...)
			if err != nil {
				t.Fatalf("failed to create analyzer: %v", err)
			}

			if err := analyzer.Analyze(context.Background(), apiPath, "main"); err != nil {
				t.Fatalf("failed to analyze package: %+v", err)
			}
			apiSpec := analyzer.OpenAPI

			var got bytes.Buffer
			enc := json.NewEncoder(&got)
			enc.SetIndent("", "  ")
			if err := enc.Encode(apiSpec); err != nil {
				t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
			}

			if *update {
				if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
					t.Fatalf("failed to write golden file %s: %v", goldenFile, err)
				}
				t.Logf("golden file updated: %s", goldenFile)
				return
			}

			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
			}

			var wantNormalized, gotNormalized any
			if err := json.Unmarshal(want, &wantNormalized); err != nil {
				t.Fatalf("failed to unmarshal want JSON: %v", err)
			}
			if err := json.Unmarshal(got.Bytes(), &gotNormalized); err != nil {
				t.Fatalf("failed to unmarshal got JSON: %v", err)
			}

			if diff := cmp.Diff(wantNormalized, gotNormalized); diff != "" {
				gotJSON, _ := json.MarshalIndent(gotNormalized, "", "  ")
				t.Errorf("OpenAPI spec mismatch for new features (-want +got):\n%s\n\nGot:\n%s", diff, gotJSON)
			}
		},
	})
}

func TestDocgen_refAndRename(t *testing.T) {
	// This test verifies that:
	// 1. Reused structs result in a single schema in #/components/schemas with $ref used everywhere else.
	// 2. Structs with the same name in different packages are treated as distinct schemas.
	// 3. Handlers with the same name in different packages get unique operation IDs.
	const apiPath = "ref-and-rename/api" // The module name + package dir to analyze
	moduleDir := "testdata/ref-and-rename"
	goldenFile := filepath.Join(moduleDir, "api.golden.json") // Golden file inside the test module

	logger := newTestLogger(io.Discard)

	// Create a scanner configured to find the new module.
	s, err := goscan.New(
		goscan.WithWorkDir(moduleDir), // Point scanner to the module directory
		goscan.WithGoModuleResolver(),
		goscan.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	analyzer, err := NewAnalyzer(s, logger)
	if err != nil {
		t.Fatalf("failed to create analyzer: %v", err)
	}

	// Analyze the package. The entrypoint is the main function.
	ctx := context.Background()
	if err := analyzer.Analyze(ctx, apiPath, "NewServeMux"); err != nil {
		t.Fatalf("failed to analyze package: %+v", err)
	}
	apiSpec := analyzer.OpenAPI

	// Marshal the result to JSON.
	var got bytes.Buffer
	enc := json.NewEncoder(&got)
	enc.SetIndent("", "  ")
	if err := enc.Encode(apiSpec); err != nil {
		t.Fatalf("failed to marshal OpenAPI spec to json: %v", err)
	}

	// Compare with the golden file.
	if *update {
		if err := os.WriteFile(goldenFile, got.Bytes(), 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenFile, err)
		}
		t.Logf("golden file updated: %s", goldenFile)
		return
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file not found: %s. Run with -update to create it.", goldenFile)
		}
		t.Fatalf("failed to read golden file %s: %v", goldenFile, err)
	}

	// Normalize JSON to avoid cosmetic diffs
	var wantNormalized, gotNormalized any
	if err := json.Unmarshal(want, &wantNormalized); err != nil {
		t.Fatalf("failed to unmarshal want JSON: %v", err)
	}
	if err := json.Unmarshal(got.Bytes(), &gotNormalized); err != nil {
		t.Fatalf("failed to unmarshal got JSON: %v", err)
	}

	if diff := cmp.Diff(wantNormalized, gotNormalized); diff != "" {
		gotJSON, _ := json.MarshalIndent(gotNormalized, "", "  ")
		t.Errorf("OpenAPI spec mismatch (-want +got):\n%s\n\nGot:\n%s", diff, gotJSON)
	}
}
