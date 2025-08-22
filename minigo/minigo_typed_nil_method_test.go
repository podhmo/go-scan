package minigo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scanner"
)

func TestTypedNilMethodValue(t *testing.T) {
	source := `
package main

import "my/api"

var V = (*api.API)(nil).ServeHTTP
`
	apiSource := `
package api
import "net/http"
type API struct {}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
`

	// Create a scanner with an in-memory overlay for the 'my/api' module.
	// We need to simulate a realistic module structure.
	// Let's assume the module root is '/src' and our module is 'my'.
	// The file would be at '/src/my/api/api.go'.
	// go-scan needs a valid go.mod to resolve modules.
	tmpdir := t.TempDir()

	overlay := scanner.Overlay{
		"go.mod":    []byte("module my"),
		"api/api.go": []byte(apiSource),
	}

	s, err := goscan.New(
		goscan.WithWorkDir(tmpdir),
		goscan.WithOverlay(overlay),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %+v", err)
	}

	// Create an interpreter with our custom scanner.
	i, err := minigo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}

	// Register necessary stdlib types for the test to pass.
	// The parser needs to know about http.ResponseWriter and http.Request.
	// We can do this by pre-registering them.
	i.Register("net/http", map[string]any{
		"ResponseWriter": nil, // The value doesn't matter, just the type registration.
		"Request":        nil,
	})

	// Load the main source file.
	if err := i.LoadFile("main.go", []byte(source)); err != nil {
		t.Fatalf("failed to load main.go: %+v", err)
	}

	// Evaluate the loaded files.
	if _, err := i.Eval(context.Background()); err != nil {
		t.Fatalf("minigo execution failed: %+v", err)
	}

	// Get the global variable V from the interpreter's environment.
	v, ok := i.GlobalEnvForTest().Get("V")
	if !ok {
		t.Fatalf("variable V not found in the result")
	}

	// Assert that the result is a GoMethodValue with the correct properties.
	got, ok := v.(*object.GoMethodValue)
	if !ok {
		t.Fatalf("expected V to be a *object.GoMethodValue, but got %T", v)
	}

	// The receiver type should be a pointer to the API struct.
	wantReceiverInspect := "*my/api.API"
	if diff := cmp.Diff(wantReceiverInspect, got.ReceiverType.Inspect()); diff != "" {
		t.Errorf("ReceiverType mismatch (-want +got):\n%s", diff)
	}

	// The method name should be correct.
	if got.Method.Name != "ServeHTTP" {
		t.Errorf("expected method name to be ServeHTTP, but got %s", got.Method.Name)
	}

	// Check the method's type signature.
	if len(got.Method.Parameters) != 2 {
		t.Errorf("expected 2 params, but got %d", len(got.Method.Parameters))
	}
	if want, got := "net/http.ResponseWriter", formatFieldType(got.Method.Parameters[0].Type); got != want {
		t.Errorf("param 0 type mismatch: want %q, got %q", want, got)
	}
	if want, got := "*net/http.Request", formatFieldType(got.Method.Parameters[1].Type); got != want {
		t.Errorf("param 1 type mismatch: want %q, got %q", want, got)
	}
}

// formatFieldType is a helper to get a consistent string representation of a field's type.
func formatFieldType(ft *scanner.FieldType) string {
	if ft.IsPointer {
		return "*" + formatFieldType(ft.Elem)
	}
	if ft.FullImportPath != "" {
		return ft.FullImportPath + "." + ft.Name
	}
	return ft.Name
}
