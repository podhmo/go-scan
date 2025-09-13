package symgo_test

import (
	"context"
	"fmt"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestRegression_UnresolvedTypePointerOperations_Simplified(t *testing.T) {
	mainSource := `
package main
import "example.com/external"
func main() {
	// This tests `new(unresolved.Type)`
	_ = new(external.ExtType)

	// This tests dereferencing a pointer to an unresolved type.
	var p *external.ExtType
	_ = *p
}`
	externalSource := `
package external
type ExtType struct { Name string }`

	overlay := scanner.Overlay{
		"go.mod":               []byte("module example.com"),
		"main/main.go":         []byte(mainSource),
		"external/external.go": []byte(externalSource),
	}

	// Define a scan policy that excludes the "external" package.
	scanPolicy := func(importPath string) bool {
		return importPath == "example.com/main"
	}

	ctx := context.Background()
	s, err := goscan.New(goscan.WithOverlay(overlay), goscan.WithGoModuleResolver())
	if err != nil {
		t.Fatalf("goscan.New() failed: %+v", err)
	}

	interp, err := symgo.NewInterpreter(s, symgo.WithScanPolicy(scanPolicy))
	if err != nil {
		t.Fatalf("NewInterpreter() failed: %+v", err)
	}

	// Scan all packages from the virtual root of the overlay.
	pkgs, err := s.Scan(ctx, "./...")
	if err != nil {
		t.Fatalf("Scan failed: %+v", err)
	}

	var mainPkg *goscan.Package
	for _, p := range pkgs {
		if p.ImportPath == "example.com/main" {
			mainPkg = p
			break
		}
	}
	if mainPkg == nil {
		t.Fatalf("main package not found in scan results")
	}

	// We need to evaluate the file first to load its declarations.
	_, err = interp.Eval(t.Context(), mainPkg.AstFiles["main/main.go"], mainPkg)
	if err != nil {
		t.Fatalf("interp.Eval(file) failed: %+v", err)
	}

	// Find and apply the main function
	mainFn, ok := interp.FindObjectInPackage(t.Context(), "example.com/main", "main")
	if !ok {
		t.Fatal("could not find main function")
	}

	// This call should not panic or return an error.
	_, err = interp.Apply(t.Context(), mainFn, []object.Object{}, mainPkg)
	if err != nil {
		t.Errorf("Apply() failed with unexpected error: %+v", err)
	}
}
