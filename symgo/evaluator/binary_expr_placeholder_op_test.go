package evaluator

import (
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
)

func TestEvalIntegerInfixExpression_Placeholders(t *testing.T) {
	tests := []struct {
		op    token.Token
		opStr string
	}{
		{token.REM, "%"},
		{token.SHL, "<<"},
		{token.SHR, ">>"},
		{token.AND, "&"},
		{token.OR, "|"},
		{token.XOR, "^"},
	}

	for _, tt := range tests {
		t.Run(tt.opStr, func(t *testing.T) {
			left := &object.Integer{Value: 10}
			right := &object.Integer{Value: 5}
			// For this specific unit test, we don't need a fully configured evaluator.
			e := New(nil, nil, nil, func(s string) bool { return false })

			ctx := t.Context()
			result := e.evalIntegerInfixExpression(ctx, token.NoPos, tt.op, left, right)

			placeholder, ok := result.(*object.SymbolicPlaceholder)
			if !ok {
				t.Fatalf("expected *object.SymbolicPlaceholder, got %T", result)
			}

			expectedReason := "integer operation: " + tt.op.String()
			if placeholder.Reason != expectedReason {
				t.Errorf("expected reason %q, got %q", expectedReason, placeholder.Reason)
			}
		})
	}
}
