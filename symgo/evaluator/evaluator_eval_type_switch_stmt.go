package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalTypeSwitchStmt(ctx context.Context, n *ast.TypeSwitchStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switchEnv := env
	if n.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, switchEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	var varName string
	var originalObj object.Object

	switch assign := n.Assign.(type) {
	case *ast.AssignStmt:
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return e.newError(ctx, n.Pos(), "expected one variable and one value in type switch assignment")
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return e.newError(ctx, n.Pos(), "expected identifier on LHS of type switch assignment")
		}
		varName = ident.Name

		typeAssert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
		if !ok {
			return e.newError(ctx, n.Pos(), "expected TypeAssertExpr on RHS of type switch assignment")
		}
		originalObj = e.Eval(ctx, typeAssert.X, switchEnv, pkg)
		if isError(originalObj) {
			return originalObj
		}

	case *ast.ExprStmt:
		typeAssert, ok := assign.X.(*ast.TypeAssertExpr)
		if !ok {
			return e.newError(ctx, n.Pos(), "expected TypeAssertExpr in ExprStmt of type switch")
		}
		// In `switch x.(type)`, there is no new variable, so varName remains empty.
		// We still need to evaluate the expression being switched on.
		originalObj = e.Eval(ctx, typeAssert.X, switchEnv, pkg)
		if isError(originalObj) {
			return originalObj
		}

	default:
		return e.newError(ctx, n.Pos(), "expected AssignStmt or ExprStmt in TypeSwitchStmt, got %T", n.Assign)
	}

	if n.Body != nil {
		file := pkg.Fset.File(n.Pos())
		if file == nil {
			return e.newError(ctx, n.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, n.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		for _, c := range n.Body.List {
			caseClause, ok := c.(*ast.CaseClause)
			if !ok {
				continue
			}
			caseEnv := object.NewEnclosedEnvironment(switchEnv)

			// If varName is set, we are in the `v := x.(type)` form.
			// We need to create a new variable `v` in the case's scope.
			if varName != "" {
				if caseClause.List == nil { // default case
					v := &object.Variable{
						Name:        varName,
						Value:       originalObj,
						IsEvaluated: true, // Mark as evaluated since originalObj is already set
						BaseObject: object.BaseObject{
							ResolvedTypeInfo:  originalObj.TypeInfo(),
							ResolvedFieldType: originalObj.FieldType(),
						},
					}
					caseEnv.Set(varName, v)
				} else {
					typeExpr := caseClause.List[0]
					fieldType := e.scanner.TypeInfoFromExpr(ctx, typeExpr, nil, pkg, importLookup)
					if fieldType == nil {
						if id, ok := typeExpr.(*ast.Ident); ok {
							fieldType = &scan.FieldType{Name: id.Name, IsBuiltin: true}
						} else {
							return e.newError(ctx, typeExpr.Pos(), "could not resolve type for case clause")
						}
					}

					var resolvedType *scan.TypeInfo
					if !fieldType.IsBuiltin {
						resolvedType = e.resolver.ResolveType(ctx, fieldType)
						if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
							resolvedType.Kind = scan.InterfaceKind
						}
					}

					val := &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("type switch case variable %s", fieldType.String()),
						BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
					}
					v := &object.Variable{
						Name:        varName,
						Value:       val,
						IsEvaluated: true,
						BaseObject: object.BaseObject{
							ResolvedTypeInfo:  resolvedType,
							ResolvedFieldType: fieldType,
						},
					}
					caseEnv.Set(varName, v)
				}
			}
			// If varName is empty, we are in the `x.(type)` form. No new variable is created.
			// The environment for the case body is just a new scope above the switch environment.

			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logc(ctx, slog.LevelWarn, "error evaluating statement in type switch case", "error", res)
					if isInfiniteRecursionError(res) {
						return res // Stop processing on infinite recursion
					}
				}
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "type switch statement"}
}

func (e *Evaluator) evalSwitchStmt(ctx context.Context, n *ast.SwitchStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switchEnv := env
	if n.Init != nil {
		switchEnv = object.NewEnclosedEnvironment(env)
		if initResult := e.Eval(ctx, n.Init, switchEnv, pkg); isError(initResult) {
			return initResult
		}
	}

	if n.Body == nil {
		return &object.SymbolicPlaceholder{Reason: "switch statement"}
	}

	// Iterate through each case clause as a potential starting point for a new execution path.
	for i := 0; i < len(n.Body.List); i++ {
		pathEnv := object.NewEnclosedEnvironment(switchEnv) // Each path gets its own environment to track state.

		// From this starting point `i`, trace the path until a break or the end of a case without fallthrough.
	pathLoop:
		for j := i; j < len(n.Body.List); j++ {
			caseClause, ok := n.Body.List[j].(*ast.CaseClause)
			if !ok {
				continue
			}

			// Evaluate case expressions to trace calls for their side-effects.
			for _, expr := range caseClause.List {
				if res := e.Eval(ctx, expr, pathEnv, pkg); isError(res) {
					return res // Propagate errors from case expressions.
				}
			}

			hasFallthrough := false
			for _, stmt := range caseClause.Body {
				result := e.Eval(ctx, stmt, pathEnv, pkg)

				if result != nil {
					switch result.Type() {
					case object.FALLTHROUGH_OBJ:
						hasFallthrough = true
						// The break was redundant here. The switch statement exits, and the
						// inner for-loop continues to the next statement in the case body.
						// The `hasFallthrough` flag is handled after the loop.
					case object.BREAK_OBJ:
						break pathLoop // This path is terminated by break.
					case object.RETURN_VALUE_OBJ, object.ERROR_OBJ, object.CONTINUE_OBJ:
						// Propagate these control flow changes immediately, terminating the whole switch evaluation.
						// This is a simplification but consistent with if-stmt handling.
						return result
					}
				}
			}

			if !hasFallthrough {
				break // End of this path. Start a new path from the next case.
			}
		}
	}

	return &object.SymbolicPlaceholder{Reason: "switch statement"}
}
