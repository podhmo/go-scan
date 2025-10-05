package evaluator

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
)

func TestGotoAsNoOp(t *testing.T) {
	// The testEval helper is defined in basic_test.go and is available
	// to other test files in the same package.
	// We just need to ensure that evaluating a statement containing a goto
	// does not produce an error.
	evaluated := testEval(t, `goto End`)

	// The key assertion is that no error occurred.
	// The `goto` should be ignored (return nil), and `testEval` should not
	// wrap it in an error. The evaluator should just proceed.
	if _, ok := evaluated.(*object.Error); ok {
		t.Fatalf("evaluation of goto statement failed with an unexpected error: %+v", evaluated)
	}
}