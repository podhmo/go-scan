package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"
	"strconv"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalIdent(ctx context.Context, n *ast.Ident, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if pkg != nil {
		key := pkg.ImportPath + "." + n.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			e.logger.Debug("evalIdent: found intrinsic, overriding", "key", key)
			return &object.Intrinsic{Fn: intrinsicFn}
		}
	}

	if val, ok := env.Get(n.Name); ok {
		e.logger.Debug("evalIdent: found in env", "name", n.Name, "type", val.Type(), "val", inspectValuer{val})
		if _, ok := val.(*object.Variable); ok {
			e.logger.Debug("evalIdent: identifier is a variable, evaluating it", "name", n.Name)
			// When an identifier is accessed, we must force its full evaluation.
			evaluatedValue := e.forceEval(ctx, val, pkg)
			e.logger.Debug("evalIdent: evaluated variable", "name", n.Name, "type", evaluatedValue.Type(), "value", inspectValuer{evaluatedValue})
			return evaluatedValue
		}
		return val
	}

	// If the identifier is not in the environment, it might be a package name.
	if pkg != nil && pkg.Fset != nil {
		file := pkg.Fset.File(n.Pos())
		if file != nil {
			if astFile, ok := pkg.AstFiles[file.Name()]; ok {
				for _, imp := range astFile.Imports {
					importPath, _ := strconv.Unquote(imp.Path.Value)

					// Case 1: The import has an alias.
					if imp.Name != nil {
						if n.Name == imp.Name.Name {
							pkgObj, _ := e.getOrLoadPackage(ctx, importPath)
							return pkgObj
						}
						continue
					}

					// Case 2: No alias. The identifier might be the package's actual name.
					pkgObj, _ := e.getOrLoadPackage(ctx, importPath) // Error is not fatal here.
					if pkgObj == nil {
						e.logc(ctx, slog.LevelDebug, "could not get package for ident", "ident", n.Name, "path", importPath)
						continue
					}

					// If the package was scanned, we can definitively match its name.
					if pkgObj.ScannedInfo != nil {
						if n.Name == pkgObj.ScannedInfo.Name {
							return pkgObj
						}
					} else {
						// If the package is just a placeholder (not scanned due to policy),
						// we can't know its real name for sure. Use our heuristic to guess it.
						assumedNames := guessPackageNameFromImportPath(importPath)
						for _, assumedName := range assumedNames {
							if n.Name == assumedName {
								return pkgObj
							}
						}
					}
				}
			}
		}
	}

	// Fallback to universe scope for built-in values, types, and functions.
	if obj, ok := universe.Get(n.Name); ok {
		return obj
	}
	if pkg != nil {
		for _, c := range pkg.Constants {
			if c.Name == n.Name {
				e.logger.Debug("evalIdent: found in package-level constants as fallback", "name", n.Name)
				return e.convertGoConstant(ctx, c.ConstVal, n.Pos())
			}
		}
	}

	e.logger.Debug("evalIdent: not found in env or intrinsics", "name", n.Name)

	if pkg != nil && !e.resolver.ScanPolicy(pkg.ImportPath) {
		e.logger.DebugContext(ctx, "treating undefined identifier as symbolic in out-of-policy package", "ident", n.Name, "package", pkg.ImportPath)
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("undefined identifier %s in out-of-policy package", n.Name)}
	}

	if val, ok := universe.Get(n.Name); ok {
		return val
	}
	return e.newError(ctx, n.Pos(), "identifier not found: %s", n.Name)
}

// convertGoConstant converts a go/constant.Value to a symgo/object.Object.
func (e *Evaluator) convertGoConstant(ctx context.Context, val constant.Value, pos token.Pos) object.Object {
	switch val.Kind() {
	case constant.String:
		return &object.String{Value: constant.StringVal(val)}
	case constant.Int:
		i, ok := constant.Int64Val(val)
		if !ok {
			// This might be a large integer that doesn't fit in int64.
			// For symbolic execution, this is an acceptable limitation for now.
			return e.newError(ctx, pos, "could not convert constant to int64: %s", val.String())
		}
		return &object.Integer{Value: i}
	case constant.Bool:
		return nativeBoolToBooleanObject(constant.BoolVal(val))
	case constant.Float:
		f, _ := constant.Float64Val(val)
		return &object.Float{Value: f}
	case constant.Complex:
		r, _ := constant.Float64Val(constant.Real(val))
		i, _ := constant.Float64Val(constant.Imag(val))
		return &object.Complex{Value: complex(r, i)}
	default:
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("unsupported constant kind: %s", val.Kind())}
	}
}
