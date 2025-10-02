package evaluator

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalAssignStmt(ctx context.Context, n *ast.AssignStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// Handle multi-value assignment, e.g., x, y := f() or x, y = f()
	if len(n.Rhs) == 1 && len(n.Lhs) > 1 {
		// Special case for two-value type assertions: v, ok := x.(T)
		if typeAssert, ok := n.Rhs[0].(*ast.TypeAssertExpr); ok {
			if len(n.Lhs) != 2 {
				return e.newError(ctx, n.Pos(), "type assertion with 2 values on RHS must have 2 variables on LHS, got %d", len(n.Lhs))
			}

			// Evaluate the source expression to trace calls
			e.Eval(ctx, typeAssert.X, env, pkg)

			// Resolve the asserted type (T).
			if pkg == nil || pkg.Fset == nil {
				return e.newError(ctx, n.Pos(), "package info or fset is missing, cannot resolve types for type assertion")
			}
			file := pkg.Fset.File(n.Pos())
			if file == nil {
				return e.newError(ctx, n.Pos(), "could not find file for node position")
			}
			astFile, ok := pkg.AstFiles[file.Name()]
			if !ok {
				return e.newError(ctx, n.Pos(), "could not find ast.File for path: %s", file.Name())
			}
			importLookup := e.scanner.BuildImportLookup(astFile)

			fieldType := e.scanner.TypeInfoFromExpr(ctx, typeAssert.Type, nil, pkg, importLookup)
			if fieldType == nil {
				var typeNameBuf bytes.Buffer
				printer.Fprint(&typeNameBuf, pkg.Fset, typeAssert.Type)
				return e.newError(ctx, typeAssert.Pos(), "could not resolve type for type assertion: %s", typeNameBuf.String())
			}
			resolvedType := e.resolver.ResolveType(ctx, fieldType)

			// If the type was unresolved, we can now infer its kind to be an interface.
			if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
				resolvedType.Kind = scan.InterfaceKind
			}

			// Create placeholders for the two return values.
			valuePlaceholder := &object.SymbolicPlaceholder{
				Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			}

			okPlaceholder := &object.SymbolicPlaceholder{
				Reason: "ok from type assertion",
				BaseObject: object.BaseObject{
					ResolvedTypeInfo: nil, // Built-in types do not have a TypeInfo struct.
					ResolvedFieldType: &scan.FieldType{
						Name:      "bool",
						IsBuiltin: true,
					},
				},
			}

			// Assign the placeholders to the LHS variables.
			if ident, ok := n.Lhs[0].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ctx, ident, valuePlaceholder, n.Tok, env)
				}
			}
			if ident, ok := n.Lhs[1].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ctx, ident, okPlaceholder, n.Tok, env)
				}
			}
			return nil
		}

		rhsValue := e.Eval(ctx, n.Rhs[0], env, pkg)
		if isError(rhsValue) {
			return rhsValue
		}

		// The result of a function call might be wrapped in a ReturnValue.
		if ret, ok := rhsValue.(*object.ReturnValue); ok {
			rhsValue = ret.Value
		}

		// If the result is a single symbolic placeholder, but we expect multiple return values,
		// we can expand it into a MultiReturn object with the correct number of placeholders.
		// This handles calls to unscannable functions that are expected to return multiple values.
		if sp, isPlaceholder := rhsValue.(*object.SymbolicPlaceholder); isPlaceholder {
			if len(n.Lhs) > 1 {
				placeholders := make([]object.Object, len(n.Lhs))
				for i := 0; i < len(n.Lhs); i++ {
					// The first placeholder inherits the reason from the original.
					if i == 0 {
						placeholders[i] = sp
					} else {
						placeholders[i] = &object.SymbolicPlaceholder{
							Reason: fmt.Sprintf("inferred result %d from multi-value assignment to %s", i, sp.Reason),
						}
					}
				}
				rhsValue = &object.MultiReturn{Values: placeholders}
			}
		}

		multiRet, ok := rhsValue.(*object.MultiReturn)
		if !ok {
			// This can happen if a function that is supposed to return multiple values
			// is not correctly modeled. We fall back to assigning placeholders.
			e.logc(ctx, slog.LevelWarn, "expected multi-return value on RHS of assignment", "got_type", rhsValue.Type(), "value", inspectValuer{rhsValue})
			for _, lhsExpr := range n.Lhs {
				if ident, ok := lhsExpr.(*ast.Ident); ok && ident.Name != "_" {
					v := &object.Variable{
						Name:  ident.Name,
						Value: &object.SymbolicPlaceholder{Reason: "unhandled multi-value assignment"},
					}
					env.Set(ident.Name, v)
				}
			}
			return nil
		}

		if len(multiRet.Values) != len(n.Lhs) {
			return e.newError(ctx, n.Pos(), "assignment mismatch: %d variables but %d values", len(n.Lhs), len(multiRet.Values))
		}

		for i, lhsExpr := range n.Lhs {
			if ident, ok := lhsExpr.(*ast.Ident); ok {
				if ident.Name == "_" {
					continue
				}
				val := multiRet.Values[i]
				e.assignIdentifier(ctx, ident, val, n.Tok, env) // Use the statement's token (:= or =)
			}
		}
		return nil
	}

	// Handle single assignment: x = y or x := y
	if len(n.Lhs) == 1 && len(n.Rhs) == 1 {
		switch lhs := n.Lhs[0].(type) {
		case *ast.Ident:
			if lhs.Name == "_" {
				// Evaluate RHS for side-effects even if assigned to blank identifier.
				return e.Eval(ctx, n.Rhs[0], env, pkg)
			}
			return e.evalIdentAssignment(ctx, lhs, n.Rhs[0], n.Tok, env, pkg)
		case *ast.SelectorExpr:
			// This is an assignment to a field, like `foo.Bar = 1`.
			// We need to evaluate the `foo` part (lhs.X) to trace any calls within it.
			e.Eval(ctx, lhs.X, env, pkg)
			// Then evaluate the RHS.
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		case *ast.IndexExpr:
			// This is an assignment to a map or slice index, like `m[k] = v`.
			// We need to evaluate all parts to trace calls.
			// 1. Evaluate the map/slice expression (e.g., `m`).
			e.Eval(ctx, lhs.X, env, pkg)
			// 2. Evaluate the index expression (e.g., `k`).
			e.Eval(ctx, lhs.Index, env, pkg)
			// 3. Evaluate the RHS value (e.g., `v`).
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		case *ast.StarExpr:
			// This is an assignment to a pointer dereference, like `*p = v`.
			// Evaluate the pointer expression (e.g., `p`).
			e.Eval(ctx, lhs.X, env, pkg)
			// Evaluate the RHS value (e.g., `v`).
			e.Eval(ctx, n.Rhs[0], env, pkg)
			return nil
		default:
			return e.newError(ctx, n.Pos(), "unsupported assignment target: expected an identifier, selector or index expression, but got %T", lhs)
		}
	}

	// Handle parallel assignment: x, y = y, x
	if len(n.Lhs) == len(n.Rhs) {
		// First, evaluate all RHS expressions before any assignments are made.
		// This is crucial for correctness in cases like `x, y = y, x`.
		rhsValues := make([]object.Object, len(n.Rhs))
		for i, rhsExpr := range n.Rhs {
			val := e.Eval(ctx, rhsExpr, env, pkg)
			if isError(val) {
				return val
			}
			rhsValues[i] = val
		}

		// Now, perform the assignments.
		for i, lhsExpr := range n.Lhs {
			if ident, ok := lhsExpr.(*ast.Ident); ok {
				if ident.Name == "_" {
					continue
				}
				e.assignIdentifier(ctx, ident, rhsValues[i], n.Tok, env)
			} else {
				// Handle other LHS types like selectors if needed in the future.
				e.logc(ctx, slog.LevelWarn, "unsupported LHS in parallel assignment", "type", fmt.Sprintf("%T", lhsExpr))
			}
		}
		return nil
	}

	return e.newError(ctx, n.Pos(), "unsupported assignment statement")
}
