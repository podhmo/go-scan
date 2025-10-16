package evaluator

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
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

			// Evaluate the source object `x`
			originalObj := e.forceEval(ctx, e.Eval(ctx, typeAssert.X, env, pkg), pkg)
			if isError(originalObj) {
				return originalObj
			}

			// Resolve the asserted type `T`.
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

			// For the success path of the assertion, the value for `v` is the original object itself.
			// We clone it to avoid modifying the original variable's value and to correctly set the
			// static type information for the new variable `v`.
			// The `if` statement evaluator is responsible for branching and handling the `!ok` case.
			valueForV := originalObj.Clone()
			valueForV.SetFieldType(fieldType) // Set static type for the new variable `v`.

			// For the success path, `ok` is true.
			valueForOk := object.TRUE

			// Assign the values to the LHS variables.
			if ident, ok := n.Lhs[0].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ctx, ident, valueForV, n.Tok, env)
				}
			}
			if ident, ok := n.Lhs[1].(*ast.Ident); ok {
				if ident.Name != "_" {
					e.assignIdentifier(ctx, ident, valueForOk, n.Tok, env)
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
				// Evaluate RHS for side-effects. If the RHS is a function call, it will
				// return an `*object.ReturnValue`. We must unwrap this to prevent `evalBlockStmt`
				// from treating it as an explicit `return` from the function being analyzed.
				res := e.Eval(ctx, n.Rhs[0], env, pkg)
				if ret, ok := res.(*object.ReturnValue); ok {
					return ret.Value
				}
				return res
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

func (e *Evaluator) evalIdentAssignment(ctx context.Context, ident *ast.Ident, rhs ast.Expr, tok token.Token, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	val := e.Eval(ctx, rhs, env, pkg)
	if isError(val) {
		return val
	}

	// If the value is a return value from a function call, unwrap it.
	if ret, ok := val.(*object.ReturnValue); ok {
		val = ret.Value
	}

	// Log the type info of the value being assigned.
	typeInfo := val.TypeInfo()
	typeName := "<nil>"
	if typeInfo != nil {
		typeName = typeInfo.Name
	}
	e.logger.Debug("evalIdentAssignment: assigning value", "var", ident.Name, "value_type", val.Type(), "value_typeinfo", typeName)

	return e.assignIdentifier(ctx, ident, val, tok, env)
}

func (e *Evaluator) assignIdentifier(ctx context.Context, ident *ast.Ident, val object.Object, tok token.Token, env *object.Environment) object.Object {
	// Before assigning, the RHS must be fully evaluated.
	val = e.forceEval(ctx, val, nil) // pkg is not strictly needed here as DeclPkg is used.
	if isError(val) {
		return val
	}

	// For `:=`, we always define a new variable in the current scope.
	if tok == token.DEFINE {
		// In Go, `:=` can redeclare a variable if it's in a different scope,
		// but in our symbolic engine, we'll simplify and just overwrite in the local scope.
		// A more complex implementation would handle shadowing more precisely.
		v := &object.Variable{
			Name:        ident.Name,
			Value:       val,
			IsEvaluated: true, // A variable defined with `:=` has its value evaluated immediately.
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  val.TypeInfo(),
				ResolvedFieldType: val.FieldType(),
			},
		}
		if val.FieldType() != nil {
			if resolved := e.resolver.ResolveType(ctx, val.FieldType()); resolved != nil && resolved.Kind == scan.InterfaceKind {
				v.PossibleTypes = make(map[string]struct{})
				if ft := val.FieldType(); ft != nil {
					v.PossibleTypes[ft.String()] = struct{}{}
				}
			}
		}
		e.logger.Debug("evalAssignStmt: defining var", "name", ident.Name)
		return env.SetLocal(ident.Name, v) // Use SetLocal for :=
	}

	// For `=`, find the variable and update it in-place.
	obj, ok := env.Get(ident.Name)
	if !ok {
		// This can happen for package-level variables not yet evaluated,
		// or if the code is invalid Go. We define it in the current scope as a fallback.
		return e.assignIdentifier(ctx, ident, val, token.DEFINE, env)
	}

	v, ok := obj.(*object.Variable)
	if !ok {
		// Not a variable, just overwrite it in the environment.
		e.logger.Debug("evalAssignStmt: overwriting non-variable in env", "name", ident.Name)
		return env.Set(ident.Name, val)
	}

	// If the variable's declared type is an interface, we should preserve that
	// static type information on the variable itself. The concrete type of the
	// assigned value is still available on `val` (which becomes `v.Value`).
	var isLHSInterface bool
	if ft := v.FieldType(); ft != nil {
		if ti := e.resolver.ResolveType(ctx, ft); ti != nil {
			isLHSInterface = ti.Kind == scan.InterfaceKind
		}
	}

	v.Value = val
	if !isLHSInterface {
		v.SetTypeInfo(val.TypeInfo())
		v.SetFieldType(val.FieldType())
	}
	newFieldType := val.FieldType()

	// Always accumulate possible types. Resetting the map can lead to lost
	// information, especially when dealing with interface assignments where the
	// static type of the variable might be unresolved.
	if v.PossibleTypes == nil {
		v.PossibleTypes = make(map[string]struct{})
	}
	if newFieldType != nil {
		key := newFieldType.String()

		// Workaround: If the default string representation of a pointer type is just "*",
		// it's likely because the underlying element's FieldType has an empty name.
		// In this case, we construct a more robust key using the TypeInfo from the
		// object the pointer points to. This makes the analysis resilient to
		// incomplete FieldType information from the scanner.
		if key == "*" {
			if ptr, ok := val.(*object.Pointer); ok {
				if inst, ok := ptr.Value.(*object.Instance); ok {
					if ti := inst.TypeInfo(); ti != nil && ti.PkgPath != "" && ti.Name != "" {
						key = fmt.Sprintf("%s.*%s", ti.PkgPath, ti.Name)
					}
				}
			}
		}

		v.PossibleTypes[key] = struct{}{}
		e.logger.Debug("evalAssignStmt: adding possible type to var", "name", ident.Name, "new_type", key)
	}

	return v
}
