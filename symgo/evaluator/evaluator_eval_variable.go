package evaluator

import (
	"context"
	"fmt"
	"go/token"
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

// forceEval recursively evaluates an object until it is no longer a variable or ambiguous selector.
// This is crucial for handling variables whose initializers are other variables and for resolving ambiguity.
func (e *Evaluator) forceEval(ctx context.Context, obj object.Object, pkg *scan.PackageInfo) object.Object {
	for i := 0; i < 100; i++ { // Add a loop limit to prevent infinite loops in weird cases
		switch o := obj.(type) {
		case *object.Variable:
			obj = e.evalVariable(ctx, o, pkg)
			if isError(obj) {
				return obj
			}
			// Loop again in case the result is another variable.
			continue
		case *object.AmbiguousSelector:
			// If forceEval encounters an ambiguous selector, it means the expression
			// is being used in a context where a value is expected (e.g., assignment,
			// variable access). We resolve the ambiguity by assuming it's a field.
			var typeName string
			if typeInfo := o.Receiver.TypeInfo(); typeInfo != nil {
				typeName = typeInfo.Name
			}
			e.logc(ctx, slog.LevelWarn, "assuming field exists on unresolved embedded type", "field_name", o.Sel.Name, "type_name", typeName)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed field %s on type with unresolved embedded part", o.Sel.Name)}
		default:
			// Not a variable or ambiguous selector, return as is.
			return obj
		}
	}
	return e.newError(ctx, token.NoPos, "evaluation depth limit exceeded, possible variable evaluation loop")
}
