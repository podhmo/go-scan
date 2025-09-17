package symgo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgotest"
)

func TestInterfaceBinding(t *testing.T) {
	var intrinsicCalled bool

	source := map[string]string{
		"go.mod": "module myapp\n\ngo 1.22",
		"main.go": `
package main
import "io"

// TargetFunc is the function we will analyze.
func TargetFunc(writer io.Writer) {
	writer.WriteString("hello")
}`,
	}

	setup := symgotest.WithSetup(func(interp *symgo.Interpreter) error {
		// Action: Bind the interface `io.Writer` to the concrete type `*bytes.Buffer`.
		// This must happen after the interpreter is created but before analysis runs.
		if err := interp.BindInterface(context.Background(), "io.Writer", "*bytes.Buffer"); err != nil {
			return err
		}

		// Action: Register an intrinsic for the method on the concrete type.
		interp.RegisterIntrinsic("(*bytes.Buffer).WriteString", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
			intrinsicCalled = true
			return nil
		})
		return nil
	})

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "myapp.TargetFunc",
		// Args is nil; symgotest will create a symbolic placeholder for the io.Writer argument.
		Options: []symgotest.Option{setup},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}

		if !intrinsicCalled {
			t.Errorf("expected intrinsic for (*bytes.Buffer).WriteString to be called, but it was not")
		}
	})
}
