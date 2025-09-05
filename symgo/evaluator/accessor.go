package evaluator

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// accessor provides methods for finding fields and methods on types,
// handling embedded structs and method resolution.
type accessor struct {
	eval *Evaluator
}

// newAccessor creates a new accessor associated with an evaluator.
func newAccessor(eval *Evaluator) *accessor {
	return &accessor{eval: eval}
}

// findFieldOnType recursively finds a field on a type or its embedded types.
func (a *accessor) findFieldOnType(ctx context.Context, typeInfo *scanner.TypeInfo, fieldName string) (*scanner.FieldInfo, error) {
	if typeInfo == nil {
		return nil, nil // Cannot find field without type info
	}

	visited := make(map[string]bool)
	return a.findFieldRecursive(ctx, typeInfo, fieldName, visited)
}

func (a *accessor) findFieldRecursive(ctx context.Context, typeInfo *scanner.TypeInfo, fieldName string, visited map[string]bool) (*scanner.FieldInfo, error) {
	if typeInfo == nil || typeInfo.Struct == nil {
		return nil, nil
	}

	typeKey := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if visited[typeKey] {
		return nil, nil // Cycle detected
	}
	visited[typeKey] = true

	// 1. Search for a direct field on the current type.
	for _, field := range typeInfo.Struct.Fields {
		if !field.Embedded && field.Name == fieldName {
			return field, nil
		}
	}

	// 2. If not found, search in embedded structs.
	for _, field := range typeInfo.Struct.Fields {
		if field.Embedded {
			// If the embedded field itself has the name we're looking for (promoted field)
			if field.Name == fieldName {
				return field, nil
			}

			var embeddedTypeInfo *scanner.TypeInfo
			if field.Type.FullImportPath != "" && !a.eval.resolver.ScanPolicy(field.Type.FullImportPath) {
				embeddedTypeInfo = scanner.NewUnresolvedTypeInfo(field.Type.FullImportPath, field.Type.TypeName)
			} else {
				embeddedTypeInfo, _ = field.Type.Resolve(ctx)
			}

			if embeddedTypeInfo != nil {
				if foundField, err := a.findFieldRecursive(ctx, embeddedTypeInfo, fieldName, visited); err != nil || foundField != nil {
					return foundField, err
				}
			}
		}
	}

	return nil, nil // Not found
}

// findMethodOnType recursively finds a method on a type or its embedded types.
// It returns a callable Function object if found.
func (a *accessor) findMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object) (*object.Function, error) {
	if typeInfo == nil {
		return nil, nil // Cannot find method without type info
	}

	// Use a map to track visited types and prevent infinite recursion.
	visited := make(map[string]bool)
	return a.findMethodRecursive(ctx, typeInfo, methodName, env, receiver, visited)
}

func (a *accessor) findMethodRecursive(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object, visited map[string]bool) (*object.Function, error) {
	if typeInfo == nil {
		return nil, nil
	}

	// Create a unique key for the type to track visited nodes.
	typeKey := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if visited[typeKey] {
		return nil, nil // Cycle detected
	}
	visited[typeKey] = true

	// 1. Search for a direct method on the current type.
	if method, err := a.findDirectMethodOnType(ctx, typeInfo, methodName, env, receiver); err != nil || method != nil {
		return method, err
	}

	// 2. If not found, search in embedded structs.
	if typeInfo.Struct != nil {
		for _, field := range typeInfo.Struct.Fields {
			if field.Embedded {
				var embeddedTypeInfo *scanner.TypeInfo
				if field.Type.FullImportPath != "" && !a.eval.resolver.ScanPolicy(field.Type.FullImportPath) {
					embeddedTypeInfo = scanner.NewUnresolvedTypeInfo(field.Type.FullImportPath, field.Type.TypeName)
				} else {
					embeddedTypeInfo, _ = field.Type.Resolve(ctx)
				}

				if embeddedTypeInfo != nil {
					// If the embedded type is from a package outside the scan policy, it will be marked
					// as Unresolved. We should not attempt to find methods on it, as we don't have
					// the source code.
					if embeddedTypeInfo.Unresolved {
						continue
					}

					// Recursive call, passing the original receiver.
					if foundFn, err := a.findMethodRecursive(ctx, embeddedTypeInfo, methodName, env, receiver, visited); err != nil || foundFn != nil {
						return foundFn, err
					}
				}
			}
		}
	}

	return nil, nil // Not found
}

func (a *accessor) findDirectMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object) (*object.Function, error) {
	if typeInfo == nil || typeInfo.PkgPath == "" {
		return nil, nil
	}

	if !a.eval.resolver.ScanPolicy(typeInfo.PkgPath) {
		return nil, nil
	}

	pkgObj, err := a.eval.getOrLoadPackage(ctx, typeInfo.PkgPath, env)
	if err != nil || pkgObj.ScannedInfo == nil {
		if err != nil && strings.Contains(err.Error(), "cannot find package") {
			return nil, nil
		}
		a.eval.logc(ctx, slog.LevelWarn, "could not get or load package for method resolution", "package", typeInfo.PkgPath, "error", err)
		return nil, nil
	}
	methodPkg := pkgObj.ScannedInfo

	for _, fn := range methodPkg.Functions {
		if fn.Receiver == nil || fn.Name != methodName {
			continue
		}

		recvTypeName := fn.Receiver.Type.TypeName
		if recvTypeName == "" {
			recvTypeName = fn.Receiver.Type.Name
		}
		baseRecvTypeName := strings.TrimPrefix(recvTypeName, "*")
		baseTypeName := strings.TrimPrefix(typeInfo.Name, "*")

		if baseRecvTypeName == baseTypeName {
			return &object.Function{
				Name:       fn.AstDecl.Name,
				Parameters: fn.AstDecl.Type.Params,
				Body:       fn.AstDecl.Body,
				Env:        pkgObj.Env, // Use the canonical environment from the cached package object.
				Decl:       fn.AstDecl,
				Package:    methodPkg,
				Receiver:   receiver,
				Def:        fn,
			}, nil
		}
	}

	return nil, nil // Not found
}
