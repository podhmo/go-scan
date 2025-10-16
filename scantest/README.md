# scantest

The `scantest` package provides helpers for writing tests for tools built with the `go-scan` library. It simplifies the process of creating a `goscan.Scanner` instance with in-memory source files and running assertions against the scanned results.

## Quick Start

The main function is `scantest.Scan`, which takes a `*testing.T`, the source code to scan, and the name of the package. It returns a `ScanResult` containing the scanned packages and other relevant information.

### Example

```go
package scantest_test

import (
	"testing"
	"github.com/podhmo/go-scan/scantest"
)

func TestScantest(t *testing.T) {
	source := `
package mypkg
type Person struct {
    Name string
}`

	// scantest.Scan handles the boilerplate of setting up a scanner with an overlay.
	result := scantest.Scan(t, source, "mypkg")

	// You can then make assertions on the result using the standard testing package.
	if len(result.Packages) != 1 {
		t.Fatalf("expected one package to be scanned, but got %d", len(result.Packages))
	}

	pkg := result.Packages[0]
	person, ok := pkg.LookupType("Person")
	if !ok {
		t.Fatal("Person type not found in package")
	}

	if person.Name != "Person" {
		t.Errorf("expected type name to be Person, but got %s", person.Name)
	}
	if len(person.Struct.Fields) != 1 {
		t.Fatalf("Person should have one field, but got %d", len(person.Struct.Fields))
	}
	if person.Struct.Fields[0].Name != "Name" {
		t.Errorf("expected field name to be Name, but got %s", person.Struct.Fields[0].Name)
	}
}
```

The `ScanResult` object provides easy access to the `goscan.Scanner` instance, the list of scanned packages, and any errors that occurred, making it easy to write focused and readable tests.