package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalAssignStmt(ctx context.Context, stmt *ast.AssignStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if len(stmt.Lhs) > 1 && len(stmt.Rhs) == 1 {
		return e.evalMultiReturnAssign(ctx, stmt, env, pkg)
	}

	if len(stmt.Lhs) != len(stmt.Rhs) {
		return e.newError(ctx, stmt.Pos(), "assignment mismatch: %d variables but %d values", len(stmt.Lhs), len(stmt.Rhs))
	}

	rhsVals := make([]object.Object, len(stmt.Rhs))
	for i, rhsExpr := range stmt.Rhs {
		val := e.Eval(ctx, rhsExpr, env, pkg)
		if isError(val) {
			return val
		}
		if retVal, ok := val.(*object.ReturnValue); ok {
			val = retVal.Value
		}
		rhsVals[i] = val
	}

	for i, lhsExpr := range stmt.Lhs {
		val := rhsVals[i]
		if err := e.assign(ctx, lhsExpr, val, env, pkg, stmt.Tok); err != nil {
			return err
		}
	}
	return nil
}

func (e *Evaluator) evalMultiReturnAssign(ctx context.Context, stmt *ast.AssignStmt, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	var right object.Object

	switch rhsNode := stmt.Rhs[0].(type) {
	case *ast.TypeAssertExpr:
		x := e.Eval(ctx, rhsNode.X, env, pkg)
		if isError(x) {
			return x
		}

		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, rhsNode.Pos(), "package info or fset is missing for type assertion")
		}
		file := pkg.Fset.File(rhsNode.Pos())
		if file == nil {
			return e.newError(ctx, rhsNode.Pos(), "could not find file for node position")
		}
		astFile, fileOK := pkg.AstFiles[file.Name()]
		if !fileOK {
			return e.newError(ctx, rhsNode.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)
		assertedFieldType := e.scanner.TypeInfoFromExpr(ctx, rhsNode.Type, nil, pkg, importLookup)
		assertedTypeInfo := e.resolver.ResolveType(ctx, assertedFieldType)

		valPlaceholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("value from type assertion to %s", assertedFieldType.String()),
		}
		valPlaceholder.SetTypeInfo(assertedTypeInfo)
		valPlaceholder.SetFieldType(assertedFieldType)
		okVal := object.TRUE
		right = &object.MultiReturn{Values: []object.Object{valPlaceholder, okVal}}

	case *ast.IndexExpr:
		collection := e.Eval(ctx, rhsNode.X, env, pkg)
		if isError(collection) {
			return collection
		}
		if index := e.Eval(ctx, rhsNode.Index, env, pkg); isError(index) {
			return index
		}
		valPlaceholder := &object.SymbolicPlaceholder{Reason: "value from map/slice index access"}

		if collVar, ok := collection.(*object.Variable); ok {
			collection = collVar.Value
		}

		var elemFieldType *scanner.FieldType
		switch c := collection.(type) {
		case *object.Map:
			if c.MapFieldType != nil {
				elemFieldType = c.MapFieldType.Elem
			}
		case *object.Slice:
			if c.SliceFieldType != nil {
				elemFieldType = c.SliceFieldType.Elem
			}
		default:
			if ti := collection.TypeInfo(); ti != nil && ti.Underlying != nil {
				if ti.Underlying.IsMap || ti.Underlying.IsSlice {
					elemFieldType = ti.Underlying.Elem
				}
			}
		}

		if elemFieldType != nil {
			valPlaceholder.SetFieldType(elemFieldType)
			valPlaceholder.SetTypeInfo(e.resolver.ResolveType(ctx, elemFieldType))
		}

		okVal := object.TRUE
		right = &object.MultiReturn{Values: []object.Object{valPlaceholder, okVal}}

	default:
		right = e.Eval(ctx, stmt.Rhs[0], env, pkg)
	}

	if isError(right) {
		return right
	}

	if retVal, ok := right.(*object.ReturnValue); ok {
		right = retVal.Value
	}

	multiRet, ok := right.(*object.MultiReturn)
	if !ok {
		if _, isSymbolic := right.(*object.SymbolicPlaceholder); isSymbolic {
			for _, lhsExpr := range stmt.Lhs {
				newPlaceholder := &object.SymbolicPlaceholder{Reason: "value from multi-return symbolic call"}
				if err := e.assign(ctx, lhsExpr, newPlaceholder, env, pkg, stmt.Tok); err != nil {
					return err
				}
			}
			return nil
		}
		return e.newError(ctx, stmt.Rhs[0].Pos(), "assignment mismatch: %d variables but 1 value", len(stmt.Lhs))
	}

	if len(stmt.Lhs) != len(multiRet.Values) {
		return e.newError(ctx, stmt.Pos(), "assignment mismatch: %d variables but %d values from function call", len(stmt.Lhs), len(multiRet.Values))
	}

	for i, val := range multiRet.Values {
		if err := e.assign(ctx, stmt.Lhs[i], val, env, pkg, stmt.Tok); err != nil {
			return err
		}
	}
	return nil
}

func (e *Evaluator) assign(ctx context.Context, lhs ast.Expr, val object.Object, env *object.Environment, pkg *scanner.PackageInfo, tok token.Token) object.Object {
	ident, ok := lhs.(*ast.Ident)
	if !ok {
		e.logc(ctx, 1, "unsupported LHS in assignment", "type", fmt.Sprintf("%T", lhs))
		return nil
	}

	if ident.Name == "_" {
		return nil
	}

	if tok == token.DEFINE {
		if obj, exists := env.GetLocal(ident.Name); exists {
			if v, ok := obj.(*object.Variable); ok {
				e.updateVarOnAssignment(ctx, v, val)
			} else {
				return e.newError(ctx, ident.Pos(), "cannot reassign to %s, it is not a variable (got %T)", ident.Name, obj)
			}
		} else {
			v := &object.Variable{Name: ident.Name, Value: val, IsEvaluated: true, DeclPkg: pkg}
			// If the value comes from a function return, prioritize the static type from the signature.
			if rv, ok := val.(*object.ReturnValue); ok && rv.StaticType != nil {
				v.SetFieldType(rv.StaticType)
				v.SetTypeInfo(e.resolver.ResolveType(ctx, rv.StaticType))
				// When using static type, the actual value for assignment is the wrapped one.
				v.Value = rv.Value
			} else {
				v.SetTypeInfo(val.TypeInfo())
				v.SetFieldType(val.FieldType())
			}
			e.updateVarOnAssignment(ctx, v, v.Value) // Pass the unwrapped value for type tracking
			env.SetLocal(ident.Name, v)
		}
	} else { // `=`
		obj, exists := env.Get(ident.Name)
		if !exists {
			return e.newError(ctx, ident.Pos(), "identifier not found: %s", ident.Name)
		}
		if v, ok := obj.(*object.Variable); ok {
			e.updateVarOnAssignment(ctx, v, val)
		} else {
			return e.newError(ctx, ident.Pos(), "cannot assign to %s, it is not a variable (got %T)", ident.Name, obj)
		}
	}
	return nil
}

// updateVarOnAssignment updates a variable with a new value.
// A crucial part of this function is tracking the concrete types assigned to an interface variable.
// When a value is assigned to a variable that is of an interface type, this function
// determines the concrete type of the value (e.g., "*main.Dog") and stores its string
// representation in the variable's `PossibleTypes` map. This information is vital for
// later stages, such as method call resolution, where the evaluator needs to know
// which concrete methods could be called through the interface.
func (e *Evaluator) updateVarOnAssignment(ctx context.Context, v *object.Variable, val object.Object) {
	v.Value = val

	var isInterface bool
	if vft := v.FieldType(); vft != nil {
		if ti := e.resolver.ResolveType(ctx, vft); ti != nil && ti.Interface != nil {
			isInterface = true
		}
	} else if vti := v.TypeInfo(); vti != nil && vti.Interface != nil {
		isInterface = true
	}

	if !isInterface {
		return
	}

	var typeKey string
	var concreteTi *scanner.TypeInfo
	isPointer := false
	if p, ok := val.(*object.Pointer); ok {
		isPointer = true
		val = p.Value
	}

	concreteTi = val.TypeInfo()
	if concreteTi == nil && val.FieldType() != nil {
		concreteTi = e.resolver.ResolveType(ctx, val.FieldType())
	}

	if concreteTi != nil && concreteTi.PkgPath != "" && concreteTi.Name != "" {
		if isPointer {
			typeKey = fmt.Sprintf("*%s.%s", concreteTi.PkgPath, concreteTi.Name)
		} else {
			typeKey = fmt.Sprintf("%s.%s", concreteTi.PkgPath, concreteTi.Name)
		}
	}

	if typeKey != "" {
		if v.PossibleTypes == nil {
			v.PossibleTypes = make(map[string]struct{})
		}
		v.PossibleTypes[typeKey] = struct{}{}
	}
}