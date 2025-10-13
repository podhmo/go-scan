package evaluator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"

	"github.com/podhmo/go-scan/symgo/object"
)

// logc logs a message with the current function context from the call stack.
func (e *Evaluator) logc(ctx context.Context, level slog.Level, msg string, args ...any) {
	// usually depth is 2, because logc is called from other functions
	e.logcWithCallerDepth(ctx, level, 2, msg, args...)
}

// for user, use logc instead of this function
func (e *Evaluator) logcWithCallerDepth(ctx context.Context, level slog.Level, depth int, msg string, args ...any) {
	if !e.logger.Enabled(ctx, level) {
		return
	}

	// Get execution position (the caller of this function)
	_, file, line, ok := runtime.Caller(depth)
	if ok {
		// Prepend exec_pos so it appears early in the log output.
		args = append([]any{slog.String("exec_pos", fmt.Sprintf("%s:%d", file, line))}, args...)
	}

	// Add context from the current call stack frame.
	if len(e.callStack) > 0 {
		frame := e.callStack[len(e.callStack)-1]
		posStr := ""
		if e.scanner != nil && e.scanner.Fset() != nil && frame.Pos.IsValid() {
			posStr = e.scanner.Fset().Position(frame.Pos).String()
		}
		contextArgs := []any{
			slog.String("in_func", frame.Function),
			slog.String("in_func_pos", posStr),
		}
		// Prepend context args so they appear first in the log.
		args = append(contextArgs, args...)
	}

	// Prevent recursion: if an argument is an *object.Error, don't inspect it deeply.
	for i, arg := range args {
		if err, ok := arg.(*object.Error); ok {
			args[i] = slog.String("error", err.Message)
		}
	}

	e.logger.Log(ctx, level, msg, args...)

	// Optionally dump the call stack on errors for debugging.
	if dumpStackEnabled && level >= slog.LevelError {
		dumpStack(e, os.Stderr)
		panic("dump stack for debug")
	}
}

// dumpStackEnabled controls whether to dump the call stack on errors.
var dumpStackEnabled = os.Getenv("SYMGO_DUMP_STACK") != ""

func dumpStack(e *Evaluator, w io.Writer) {
	fmt.Fprintln(w, "----------------------------------------")
	fmt.Fprintln(w, "analysis target stack:")
	for i := len(e.callStack) - 1; i >= 0; i-- {
		frame := e.callStack[i]
		posStr := ""
		if e.scanner != nil && e.scanner.Fset() != nil && frame.Pos.IsValid() {
			posStr = e.scanner.Fset().Position(frame.Pos).String()
		}
		fmt.Fprintf(w, "  at %s (%s)\n", frame.Function, posStr)

		{
			if frame.Pos.IsValid() {
				position := e.scanner.Fset().Position(frame.Pos)
				f, err := os.Open(position.Filename)
				if err != nil {
					fmt.Fprintln(w, "       (failed to open source file:", err, ")")
					f.Close()
					continue
				}
				s := bufio.NewScanner(f)
				lineno := 1
				for s.Scan() {
					if lineno == position.Line {
						fmt.Fprintf(w, "       > %d: %s\n", lineno, s.Text())
					} else if lineno >= position.Line-2 && lineno <= position.Line+2 {
						fmt.Fprintf(w, "         %d: %s\n", lineno, s.Text())
					}
					lineno++
				}
				f.Close()
			}
		}
	}
	fmt.Fprintln(w, "----------------------------------------")
}
