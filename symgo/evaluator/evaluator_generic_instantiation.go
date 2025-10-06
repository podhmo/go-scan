package evaluator

import (
	"context"
	"go/ast"
	"go/token"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// evalGenericInstantiation handles the creation of an InstantiatedFunction object
// from a generic function and its type arguments.
func (e *Evaluator) evalGenericInstantiation(ctx context.Context, fn *object.Function, typeArgs []ast.Expr, pos token.Pos, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Resolve type arguments into TypeInfo objects by evaluating the expressions.
	var resolvedArgs []*scan.TypeInfo
	for _, argExpr := range typeArgs {
		// Evaluate the type argument expression in the current environment.
		// This will correctly resolve identifiers like `int` from the universe scope
		// or other types from the local scope.
		evaluatedArg := e.Eval(ctx, argExpr, env, pkg)
		if isError(evaluatedArg) {
			// If evaluation fails, we can't proceed. Return a placeholder.
			return &object.SymbolicPlaceholder{Reason: "failed to evaluate generic type argument"}
		}

		// The result of evaluating a type expression should be an `object.Type`.
		if typeObj, ok := evaluatedArg.(*object.Type); ok {
			resolvedArgs = append(resolvedArgs, typeObj.ResolvedType)
		} else {
			// If it's not an object.Type, we can't use it as a type argument.
			// Add a nil to indicate resolution failure for this argument.
			resolvedArgs = append(resolvedArgs, nil)
		}
	}

	// Create the mapping from type parameter names to their resolved types.
	typeParamMap := make(map[string]*scan.TypeInfo)
	if fn.Def != nil && fn.Def.TypeParams != nil {
		for i, typeParam := range fn.Def.TypeParams {
			if i < len(resolvedArgs) && resolvedArgs[i] != nil {
				typeParamMap[typeParam.Name] = resolvedArgs[i]
			}
		}
	}

	return &object.InstantiatedFunction{
		Function:      fn,
		TypeArguments: typeArgs,
		TypeArgs:      resolvedArgs,
		TypeParamMap:  typeParamMap,
	}
}
