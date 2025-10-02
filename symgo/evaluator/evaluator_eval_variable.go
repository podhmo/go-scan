package evaluator

import (
	"context"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// evalVariable evaluates a variable, triggering its initializer if it's lazy.
func (e *Evaluator) evalVariable(ctx context.Context, v *object.Variable, pkg *scan.PackageInfo) object.Object {
	e.logger.DebugContext(ctx, "evalVariable: start", "var", v.Name, "is_evaluated", v.IsEvaluated)
	if v.IsEvaluated {
		e.logger.DebugContext(ctx, "evalVariable: already evaluated, returning cached value", "var", v.Name, "value_type", v.Value.Type(), "value", inspectValuer{v.Value})
		return v.Value
	}

	// Prevent infinite recursion for variable initializers.
	if v.Initializer == nil {
		// This is a variable declared without a value, like `var x int`.
		// Its value is the zero value for its type. For symbolic execution,
		// we represent this with a specific placeholder that carries the type info.
		placeholder := &object.SymbolicPlaceholder{Reason: "zero value for uninitialized variable"}
		if ft := v.FieldType(); ft != nil {
			placeholder.SetFieldType(ft)
			placeholder.SetTypeInfo(e.resolver.ResolveType(ctx, ft))
		}
		v.Value = placeholder
		v.IsEvaluated = true
		return v.Value
	}

	if e.evaluationInProgress[v.Initializer] {
		e.logc(ctx, slog.LevelWarn, "cyclic dependency detected in variable initializer", "var", v.Name, "pos", v.Initializer.Pos())
		return e.newError(ctx, v.Initializer.Pos(), "cyclic dependency for variable %q", v.Name)
	}
	e.evaluationInProgress[v.Initializer] = true
	defer delete(e.evaluationInProgress, v.Initializer)

	e.logger.DebugContext(ctx, "evalVariable: evaluating initializer", "var", v.Name)
	// Evaluate the initializer in the environment where the variable was declared.
	val := e.Eval(ctx, v.Initializer, v.DeclEnv, v.DeclPkg)
	if isError(val) {
		return val
	}
	v.Value = val
	v.IsEvaluated = true
	e.logger.DebugContext(ctx, "evalVariable: finished evaluation", "var", v.Name, "value_type", val.Type(), "value", inspectValuer{val})
	return val
}
