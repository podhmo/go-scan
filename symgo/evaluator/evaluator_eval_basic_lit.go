package evaluator

import (
	"context"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalBasicLit(ctx context.Context, n *ast.BasicLit) object.Object {
	switch n.Kind {
	case token.INT:
		i, err := strconv.ParseInt(n.Value, 0, 64)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not parse %q as integer", n.Value)
		}
		return &object.Integer{Value: i}
	case token.STRING:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not unquote string %q", n.Value)
		}
		return &object.String{Value: s}
	case token.CHAR:
		s, err := strconv.Unquote(n.Value)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not unquote char %q", n.Value)
		}
		// A char literal unquotes to a string containing the single character.
		// We take the first (and only) rune from that string.
		if len(s) == 0 {
			return e.newError(ctx, n.Pos(), "invalid empty char literal %q", n.Value)
		}
		runes := []rune(s)
		return &object.Integer{Value: int64(runes[0])}
	case token.FLOAT:
		f, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not parse %q as float", n.Value)
		}
		return &object.Float{Value: f}
	case token.IMAG:
		// The value is like "123i", "0.5i", etc.
		// We need to parse the numeric part.
		imagStr := strings.TrimSuffix(n.Value, "i")
		f, err := strconv.ParseFloat(imagStr, 64)
		if err != nil {
			return e.newError(ctx, n.Pos(), "could not parse %q as imaginary", n.Value)
		}
		return &object.Complex{Value: complex(0, f)}
	default:
		return e.newError(ctx, n.Pos(), "unsupported literal type: %s", n.Kind)
	}
}
