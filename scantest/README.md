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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScantest(t *testing.T) {
	source := `
package mypkg
type Person struct {
    Name string
}`

	// scantest.Scan handles the boilerplate of setting up a scanner with an overlay.
	result := scantest.Scan(t, source, "mypkg")

	// You can then make assertions on the result.
	require.Len(t, result.Packages, 1, "expected one package to be scanned")

	pkg := result.Packages[0]
	person, ok := pkg.LookupType("Person")
	require.True(t, ok, "Person type not found in package")

	assert.Equal(t, "Person", person.Name)
	assert.Len(t, person.Struct.Fields, 1, "Person should have one field")
	assert.Equal(t, "Name", person.Struct.Fields[0].Name)
}
```

The `ScanResult` object provides easy access to the `goscan.Scanner` instance, the list of scanned packages, and any errors that occurred, making it easy to write focused and readable tests.