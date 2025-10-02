package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalCallExpr(ctx context.Context, n *ast.CallExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if e.logger.Enabled(ctx, slog.LevelDebug) {
		stackAttrs := make([]any, 0, len(e.callStack))
		for i, frame := range e.callStack {
			posStr := ""
			if e.scanner != nil && e.scanner.Fset() != nil && frame.Pos.IsValid() {
				posStr = e.scanner.Fset().Position(frame.Pos).String()
			}
			stackAttrs = append(stackAttrs, slog.Group(fmt.Sprintf("%d", i),
				slog.String("func", frame.Function),
				slog.String("pos", posStr),
			))
		}
		e.logger.Log(ctx, slog.LevelDebug, "call", slog.Group("stack", stackAttrs...))
	}

	function := e.Eval(ctx, n.Fun, env, pkg)
	if isError(function) {
		return function
	}

	// If the function expression itself resolves to a return value (e.g., from an interface method call
	// that we intercept), we need to unwrap it before applying it.
	if ret, ok := function.(*object.ReturnValue); ok {
		function = ret.Value
	}

	args := e.evalExpressions(ctx, n.Args, env, pkg)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	// If the call includes `...`, the last argument is a slice to be expanded.
	// We wrap it in a special Variadic object to signal this to `applyFunction`.
	if n.Ellipsis.IsValid() {
		if len(args) == 0 {
			return e.newError(ctx, n.Ellipsis, "invalid use of ... with no arguments")
		}
		lastArg := args[len(args)-1]
		// The argument should be a slice, but we don't check it here.
		// `extendFunctionEnv` will handle the type logic.
		args[len(args)-1] = &object.Variadic{Value: lastArg}
	}

	// After evaluating arguments, check if any of them are function literals.
	// If so, we need to "scan" inside them to find usages. This must be done
	// before the default intrinsic is called, so the usage map is populated
	// before the parent function call is even registered.
	for _, arg := range args {
		if fn, ok := arg.(*object.Function); ok {
			e.scanFunctionLiteral(ctx, fn)
		}
	}

	if e.defaultIntrinsic != nil {
		// The default intrinsic is a "catch-all" handler that can be used for logging,
		// dependency tracking, etc. It receives the function object itself as the first
		// argument, followed by the regular arguments.

		// Pass the current call frame in the context so the intrinsic can know the caller.
		intrinsicCtx := ctx
		if len(e.callStack) > 0 {
			callerFrame := e.callStack[len(e.callStack)-1]
			intrinsicCtx = context.WithValue(ctx, callFrameKey, callerFrame)
		}
		e.defaultIntrinsic(intrinsicCtx, append([]object.Object{function}, args...)...)
	}

	result := e.applyFunction(ctx, function, args, pkg, n.Pos())
	if isError(result) {
		return result
	}
	return result
}
