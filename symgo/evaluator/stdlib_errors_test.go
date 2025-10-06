package evaluator_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestStdlib_Errors(t *testing.T) {
	t.Run("Errorf with %w and Is", func(t *testing.T) {
		source := `
package main
import "errors"
import "fmt"

func inspect(v any) {}

var ErrBase = errors.New("base error")
var ErrOther = errors.New("other error")

func main() {
	err := fmt.Errorf("wrapper: %w", ErrBase)

	isBase := errors.Is(err, ErrBase)
	isOther := errors.Is(err, ErrOther)
	isNil := errors.Is(err, nil)

	inspect(isBase)
	inspect(isOther)
	inspect(isNil)
}
`
		captured := make([]object.Object, 0, 3)
		spy := func(ctx context.Context, interp *symgo.Interpreter, args []object.Object) object.Object {
			captured = append(captured, args[0])
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
		}

		action := func(t *testing.T, r *symgotest.Result) {
			if r.Error != nil {
				t.Fatalf("unexpected error: %+v", r.Error)
			}
			if len(captured) != 3 {
				t.Fatalf("expected 3 values to be inspected, got %d", len(captured))
			}

			want := []object.Object{object.TRUE, object.FALSE, object.FALSE}
			for i, wantVal := range want {
				gotVal := captured[i]
				if gotVal != wantVal {
					t.Errorf("inspected value #%d: want %s, got %s", i, wantVal.Inspect(), gotVal.Inspect())
				}
			}
		}

		symgotest.Run(t, tc, action)
	})

	t.Run("As", func(t *testing.T) {
		source := `
package main
import "errors"
import "fmt"

func inspect(name string, v any) {}

type MyError struct {
	msg string
}
func (e *MyError) Error() string {
	return e.msg
}

func main() {
	var myErr *MyError
	err := fmt.Errorf("wrapper: %w", &MyError{msg: "my error"})

	asMyErr := errors.As(err, &myErr)
	inspect("asMyErr", asMyErr)
	inspect("myErr", myErr)
}
`
		captured := make(map[string]object.Object)
		spy := func(ctx context.Context, interp *symgo.Interpreter, args []object.Object) object.Object {
			name, ok := args[0].(*object.String)
			if !ok {
				t.Fatalf("spy expected string name, got %T", args[0])
			}
			captured[name.Value] = args[1]
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
		}

		action := func(t *testing.T, r *symgotest.Result) {
			if r.Error != nil {
				t.Fatalf("unexpected error: %+v", r.Error)
			}

			// Check asMyErr
			asMyErr, ok := captured["asMyErr"].(*object.SymbolicPlaceholder)
			if !ok {
				t.Fatalf("captured asMyErr is not a symbolic placeholder, got %T", captured["asMyErr"])
			}
			if asMyErr.Reason != "result of errors.As" {
				t.Errorf("asMyErr reason mismatch: want %q, got %q", "result of errors.As", asMyErr.Reason)
			}

			// Check myErr
			myErr, ok := captured["myErr"].(*object.SymbolicPlaceholder)
			if !ok {
				t.Fatalf("captured myErr is not a symbolic placeholder, got %T", captured["myErr"])
			}
			wantReason := "possible result of errors.As into myErr"
			if myErr.Reason != wantReason {
				t.Errorf("myErr reason mismatch: want %q, got %q", wantReason, myErr.Reason)
			}
		}

		symgotest.Run(t, tc, action)
	})
}