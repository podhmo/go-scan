package evaluator

import (
	"context"
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

			if varName != "" {
				if caseClause.List == nil { // default case
					clonedObj := originalObj.Clone()
					v := &object.Variable{
						Name:        varName,
						Value:       clonedObj,
						IsEvaluated: true,
						BaseObject: object.BaseObject{
							ResolvedTypeInfo:  clonedObj.TypeInfo(),
							ResolvedFieldType: clonedObj.FieldType(),
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

					clonedObj := originalObj.Clone()
					clonedObj.SetFieldType(fieldType)
					clonedObj.SetTypeInfo(resolvedType)

					v := &object.Variable{
						Name:        varName,
						Value:       clonedObj,
						IsEvaluated: true,
						BaseObject: object.BaseObject{
							ResolvedTypeInfo:  resolvedType,
							ResolvedFieldType: fieldType,
						},
					}
					caseEnv.Set(varName, v)
				}
			}

			for _, stmt := range caseClause.Body {
				if res := e.Eval(ctx, stmt, caseEnv, pkg); isError(res) {
					e.logc(ctx, slog.LevelWarn, "error evaluating statement in type switch case", "error", res)
					if isInfiniteRecursionError(res) {
						return res
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

	for i := 0; i < len(n.Body.List); i++ {
		pathEnv := object.NewEnclosedEnvironment(switchEnv)

		for j := i; j < len(n.Body.List); j++ {
			caseClause, ok := n.Body.List[j].(*ast.CaseClause)
			if !ok {
				continue
			}

			for _, expr := range caseClause.List {
				if res := e.Eval(ctx, expr, pathEnv, pkg); isError(res) {
					return res
				}
			}

			hasFallthrough := false
			for _, stmt := range caseClause.Body {
				result := e.Eval(ctx, stmt, pathEnv, pkg)

				if result != nil {
					switch result.Type() {
					case object.FALLTHROUGH_OBJ:
						hasFallthrough = true
					case object.BREAK_OBJ:
						goto endPath
					case object.RETURN_VALUE_OBJ, object.ERROR_OBJ, object.CONTINUE_OBJ:
						return result
					}
				}
			}

			if !hasFallthrough {
				break
			}
		}
	endPath:
	}

	return &object.SymbolicPlaceholder{Reason: "switch statement"}
}