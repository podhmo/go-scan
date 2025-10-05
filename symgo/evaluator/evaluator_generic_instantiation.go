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
func (e *Evaluator) evalGenericInstantiation(ctx context.Context, fn *object.Function, typeArgs []ast.Expr, pos token.Pos, pkg *scan.PackageInfo) object.Object {
	// Resolve type arguments into TypeInfo objects
	var resolvedArgs []*scan.TypeInfo
	if pkg != nil && pkg.Fset != nil {
		file := pkg.Fset.File(pos)
		if file != nil {
			if astFile, ok := pkg.AstFiles[file.Name()]; ok {
				importLookup := e.scanner.BuildImportLookup(astFile)
				for _, argExpr := range typeArgs {
					fieldType := e.scanner.TypeInfoFromExpr(ctx, argExpr, nil, pkg, importLookup)
					resolvedType := e.resolver.ResolveType(ctx, fieldType)
					resolvedArgs = append(resolvedArgs, resolvedType)
				}
			}
		}
	}

	return &object.InstantiatedFunction{
		Function:      fn,
		TypeArguments: typeArgs,
		TypeArgs:      resolvedArgs,
	}
}
