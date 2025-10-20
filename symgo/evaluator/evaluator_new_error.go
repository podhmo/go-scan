package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"log/slog"

	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) newError(ctx context.Context, pos token.Pos, format string, args ...interface{}) *object.Error {
	frames := make([]*object.CallFrame, len(e.callStack))
	copy(frames, e.callStack)
	err := &object.Error{
		Message:   fmt.Sprintf(format, args...),
		Pos:       pos,
		CallStack: frames,
	}
	if e.scanner != nil {
		err.AttachFileSet(e.scanner.Fset())
	}

	msg := fmt.Sprintf(format, args...)
	posStr := fmt.Sprintf("%d", pos) // Default to raw number
	if e.scanner != nil && e.scanner.Fset() != nil && pos.IsValid() {
		posStr = e.scanner.Fset().Position(pos).String()
	}
	e.logcWithCallerDepth(ctx, slog.LevelError, 2, msg, "pos", posStr, "stack", err.Inspect())
	return err
}
