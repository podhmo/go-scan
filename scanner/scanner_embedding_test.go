package scanner_test

import (
	"context"
	"fmt"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
)

func TestInterfaceEmbedding(t *testing.T) {
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/test\n",
		"pkg_a/interfaces.go": `
package pkg_a

type Reader interface {
    Read(p []byte) (n int, err error)
}
`,
		"pkg_b/interfaces.go": `
package pkg_b

import "example.com/test/pkg_a"

type ReadCloser interface {
    pkg_a.Reader
    Close() error
}
`,
	})
	defer cleanup()

	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		pkg := pkgs[0]

		if pkg.ImportPath != "example.com/test/pkg_b" {
			return fmt.Errorf("unexpected package scanned: %s", pkg.ImportPath)
		}

		rcType := pkg.Lookup("ReadCloser")
		if rcType == nil {
			return fmt.Errorf("type ReadCloser not found")
		}

		if rcType.Kind != scanner.InterfaceKind {
			return fmt.Errorf("expected ReadCloser to be an interface, but got %v", rcType.Kind)
		}

		methods := make(map[string]bool)
		for _, m := range rcType.Interface.Methods {
			methods[m.Name] = true
		}

		var foundMethods []string
		for _, m := range rcType.Interface.Methods {
			methods[m.Name] = true
			foundMethods = append(foundMethods, m.Name)
		}

		if !methods["Read"] {
			return fmt.Errorf("expected interface to have method 'Read' from embedded interface, but found: %v", foundMethods)
		}
		if !methods["Close"] {
			return fmt.Errorf("expected interface to have method 'Close', but found: %v", foundMethods)
		}

		if len(rcType.Interface.Methods) != 2 {
			return fmt.Errorf("expected 2 methods, but got %d (methods: %v)", len(rcType.Interface.Methods), foundMethods)
		}
		return nil
	}

	_, err := scantest.Run(t, context.Background(), dir, []string{"./pkg_b"}, action)
	if err != nil {
		t.Fatalf("scantest.Run failed: %v", err)
	}
}
