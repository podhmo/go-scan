package evaluator_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestNilDereferenceDoesNotHaltAnalysis(t *testing.T) {
	source := `
package main

func inspect(v any) {}

type Writer struct {
	p *int
}

func main() {
	w := &Writer{} // w.p is nil
	_ = *w.p       // Dereferencing nil should not stop the analysis.
	inspect("sentinel")
}
`
	sentinelReached := false
	spy := func(ctx context.Context, interp *symgo.Interpreter, args []object.Object) object.Object {
		if s, ok := args[0].(*object.String); ok && s.Value == "sentinel" {
			sentinelReached = true
		}
		return nil
	}

	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/main",
			"main.go": source,
		},
		EntryPoint: "example.com/main.main",
		Options: []symgotest.Option{
			symgotest.WithIntrinsic("example.com/main.inspect", spy),
		},
		// We no longer expect an error. The analysis should complete successfully.
		ExpectError: false,
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("test failed unexpectedly: %+v", r.Error)
		}
		if !sentinelReached {
			t.Errorf("sentinel was not reached; analysis likely halted at nil dereference")
		}
	}

	symgotest.Run(t, tc, action)
}
