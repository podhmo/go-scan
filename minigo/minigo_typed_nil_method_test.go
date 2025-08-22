package minigo_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo/minigotest"
	"github.com/podhmo/go-scan/scanner"
)

func TestTypedNilMethodValue(t *testing.T) {
	source := `
package main

import "my/api"

var V = (*api.API)(nil).ServeHTTP
`
	pkg := &minigotest.Package{
		Name: "main",
		Files: map[string]string{
			"main.go": source,
		},
	}

	// Run the test in an isolated environment with a custom module.
	r := minigotest.NewRunner()
	if err := r.AddModule(
		"my",
		"api",
		`
package api
import "net/http"
type API struct {}
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {}
`,
	); err != nil {
		t.Fatalf("failed to add module: %+v", err)
	}

	// Execute the script and get the global variable V.
	result, err := r.Run(pkg)
	if err != nil {
		t.Fatalf("minigo execution failed: %+v", err)
	}
	v, ok := result.Get("V")
	if !ok {
		t.Fatalf("variable V not found in the result")
	}

	// Assert that the result is a GoMethodValue with the correct properties.
	got, ok := v.(*minigo.GoMethodValue)
	if !ok {
		t.Fatalf("expected V to be a *minigo.GoMethodValue, but got %T", v)
	}

	// The receiver type should be a pointer to the API struct.
	// We can check this by inspecting its string representation.
	// Note: The pkg path might be a temporary path from scantest.
	// We only check the suffix.
	wantReceiverSuffix := "*my/api.API"
	if diff := cmp.Diff(wantReceiverSuffix, got.ReceiverType.Inspect()); diff != "" {
		t.Errorf("ReceiverType mismatch (-want +got):\n%s", diff)
	}

	// The method name should be correct.
	if got.Method.Name != "ServeHTTP" {
		t.Errorf("expected method name to be ServeHTTP, but got %s", got.Method.Name)
	}

	// Check the method's type signature.
	if len(got.Method.Signature.Params) != 2 {
		t.Errorf("expected 2 params, but got %d", len(got.Method.Signature.Params))
	}
	if want, got := "net/http.ResponseWriter", formatFieldType(got.Method.Signature.Params[0]); got != want {
		t.Errorf("param 0 type mismatch: want %q, got %q", want, got)
	}
	if want, got := "*net/http.Request", formatFieldType(got.Method.Signature.Params[1]); got != want {
		t.Errorf("param 1 type mismatch: want %q, got %q", want, got)
	}
}

// formatFieldType is a helper to get a consistent string representation of a field's type.
func formatFieldType(ft *scanner.FieldType) string {
	if ft.IsPointer {
		return "*" + formatFieldType(ft.Elem)
	}
	if ft.PkgPath != "" {
		return ft.PkgPath + "." + ft.Name
	}
	return ft.Name
}
