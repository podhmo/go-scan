package evaluator

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/printer"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalTypeAssertExpr(ctx context.Context, n *ast.TypeAssertExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// This function handles the single-value form: v := x.(T)
	// The multi-value form (v, ok := x.(T)) is handled specially in evalAssignStmt.

	// First, evaluate the expression whose type is being asserted (x).
	// This is important to trace any function calls that produce the value.
	val := e.Eval(ctx, n.X, env, pkg)
	if isError(val) {
		return val
	}

	// Next, resolve the asserted type (T).
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

	fieldType := e.scanner.TypeInfoFromExpr(ctx, n.Type, nil, pkg, importLookup)
	if fieldType == nil {
		var typeNameBuf bytes.Buffer
		printer.Fprint(&typeNameBuf, pkg.Fset, n.Type)
		return e.newError(ctx, n.Pos(), "could not resolve type for type assertion: %s", typeNameBuf.String())
	}
	resolvedType := e.resolver.ResolveType(ctx, fieldType)

	// If the type was unresolved, we can now infer its kind to be an interface.
	if resolvedType != nil && resolvedType.Kind == scan.UnknownKind {
		resolvedType.Kind = scan.InterfaceKind
	}

	// In the single-value form, the result is just a value of the asserted type.
	// We create a symbolic placeholder for it.
	return &object.SymbolicPlaceholder{
		Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
		BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
	}
}
