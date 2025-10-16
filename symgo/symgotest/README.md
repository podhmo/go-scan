# symgotest

The `symgotest` package provides helpers to streamline testing of `symgo`-based analyses. It offers convenient functions to run the `symgo.Interpreter` on in-memory source code and make assertions on the resulting environment.

## Quick Start

The primary function is `symgotest.Run`, which takes a `*testing.T`, a `context.Context`, the source code to analyze, and the name of the package. It returns a `RunResult` which contains the interpreter instance and methods to easily look up objects from the final state of the global environment.

### Example

```go
package symgotest_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymgotest(t *testing.T) {
	source := `
package mypkg
func Add(x, y int) int {
    return x + y
}
func main() {
    Add(1, 2)
}
`
	ctx := context.Background()

	// symgotest.Run handles setting up the scanner and interpreter,
	// evaluating the code, and running the main function.
	r := symgotest.Run(t, ctx, source, "mypkg")

	// The RunResult provides helper methods for looking up objects.
	addFn, ok := r.Lookup("Add")
	if !ok {
		t.Fatal("Add function not found")
	}

	// You can then perform assertions on the returned object.
	if _, isFunc := addFn.(*object.Function); !isFunc {
		t.Errorf("expected 'Add' to be a function object, but got %T", addFn)
	}
}
```

By handling the boilerplate of creating and running the interpreter, `symgotest` allows you to write concise and focused tests for your symbolic analysis logic.