package evaluator

import (
	"context"
	"go/ast"
	"go/token"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalGenDecl(ctx context.Context, node *ast.GenDecl, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	switch node.Tok {
	case token.CONST:
		var lastValues []ast.Expr
		for iota, spec := range node.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Handle implicit value repetition.
			if len(valSpec.Values) > 0 {
				lastValues = valSpec.Values
			}

			for i, name := range valSpec.Names {
				if name.Name == "_" {
					continue
				}

				if i >= len(lastValues) {
					e.logc(ctx, slog.LevelWarn, "not enough values for constant declaration", "const", name.Name)
					continue
				}

				exprToEval := lastValues[i]

				// Create a temporary environment for this expression evaluation
				// to correctly handle `iota`.
				exprEnv := object.NewEnclosedEnvironment(env)
				exprEnv.SetLocal("iota", &object.Integer{Value: int64(iota)})

				val := e.Eval(ctx, exprToEval, exprEnv, pkg)
				if isError(val) {
					e.logc(ctx, slog.LevelWarn, "could not evaluate constant expression", "const", name.Name, "error", val)
					continue
				}

				// A const declaration is a local definition.
				env.SetLocal(name.Name, val)
			}
		}
	case token.VAR:
		if pkg == nil || pkg.Fset == nil {
			return e.newError(ctx, node.Pos(), "package info or fset is missing, cannot resolve types")
		}
		file := pkg.Fset.File(node.Pos())
		if file == nil {
			return e.newError(ctx, node.Pos(), "could not find file for node position")
		}
		astFile, ok := pkg.AstFiles[file.Name()]
		if !ok {
			return e.newError(ctx, node.Pos(), "could not find ast.File for path: %s", file.Name())
		}
		importLookup := e.scanner.BuildImportLookup(astFile)

		for _, spec := range node.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			var staticFieldType *scan.FieldType
			if valSpec.Type != nil {
				staticFieldType = e.scanner.TypeInfoFromExpr(ctx, valSpec.Type, nil, pkg, importLookup)
			}

			for i, name := range valSpec.Names {
				var val object.Object
				var resolvedTypeInfo *scan.TypeInfo
				if staticFieldType != nil {
					resolvedTypeInfo = e.resolver.ResolveType(ctx, staticFieldType)
				}

				if i < len(valSpec.Values) {
					val = e.Eval(ctx, valSpec.Values[i], env, pkg)
					if isError(val) {
						return val
					}
					if ret, ok := val.(*object.ReturnValue); ok {
						val = ret.Value
					}
				} else {
					placeholder := &object.SymbolicPlaceholder{Reason: "uninitialized variable"}
					if staticFieldType != nil {
						placeholder.SetFieldType(staticFieldType)
						placeholder.SetTypeInfo(resolvedTypeInfo)
					}
					val = placeholder
				}

				v := &object.Variable{
					Name:        name.Name,
					Value:       val,
					IsEvaluated: true,
					DeclPkg:     pkg,
				}
				v.SetFieldType(val.FieldType())
				v.SetTypeInfo(val.TypeInfo())

				if staticFieldType != nil {
					if v.FieldType() == nil {
						v.SetFieldType(staticFieldType)
					}
					if v.TypeInfo() == nil {
						v.SetTypeInfo(resolvedTypeInfo)
					}
				}
				env.Set(name.Name, v)
			}
		}
	case token.TYPE:
		e.evalTypeDecl(ctx, node, env, pkg)
	}
	return nil
}
