package evaluator

import (
	"context"
	"fmt"
	"go/token"
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
// It returns a callable object (like a BoundMethod) if found.
func (a *accessor) findMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object, receiverPos token.Pos) (object.Object, error) {
	methodInfo := a.findMethodInfoOnType(ctx, typeInfo, methodName)
	if methodInfo == nil {
		a.eval.logc(ctx, slog.LevelDebug, "findMethodOnType: findMethodInfoOnType returned nil", "type", typeInfo.Name, "method", methodName)
		return nil, nil
	}
	a.eval.logc(ctx, slog.LevelDebug, "findMethodOnType: findMethodInfoOnType returned method", "type", typeInfo.Name, "method", methodName, "receiver", methodInfo.Receiver.Type.String())

	var pkgPath string
	if methodInfo.Owner != nil {
		pkgPath = methodInfo.Owner.PkgPath
	} else if methodInfo.Receiver != nil && methodInfo.Receiver.Type != nil {
		pkgPath = methodInfo.Receiver.Type.FullImportPath
	} else {
		// Fallback for functions without a clear owner/receiver package path
		pkgPath = a.eval.scanner.ModulePath()
	}

	pkgObj, err := a.eval.getOrLoadPackage(ctx, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("package for method not found: %w", err)
	}

	baseFnObj := a.eval.getOrResolveFunction(ctx, pkgObj, methodInfo)
	a.eval.logc(ctx, slog.LevelDebug, "findMethodOnType: getOrResolveFunction returned", "type", fmt.Sprintf("%T", baseFnObj), "val", inspectValuer{baseFnObj})
	fn, ok := baseFnObj.(*object.Function)
	if !ok {
		return baseFnObj, nil // It might be an intrinsic or other callable
	}

	// Do not attach the receiver to the shared function object.
	// Instead, wrap it in a BoundMethod.
	boundMethod := &object.BoundMethod{
		Function: fn,
		Receiver: receiver,
	}
	// The bound method still carries the type info of the underlying function.
	boundMethod.SetTypeInfo(fn.TypeInfo())
	boundMethod.SetFieldType(fn.FieldType())

	a.eval.logc(ctx, slog.LevelDebug, "findMethodOnType: returning bound method", "method", fn.Name.Name, "receiver_type", fmt.Sprintf("%T", receiver))
	return boundMethod, nil
}

// findMethodInfoOnType finds the scanner.FunctionInfo for a method on a type, handling embedding.
func (a *accessor) findMethodInfoOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string) *scanner.FunctionInfo {
	if typeInfo == nil {
		return nil
	}
	visited := make(map[string]bool)
	return a.findMethodInfoRecursive(ctx, typeInfo, methodName, visited)
}

func (a *accessor) findMethodInfoRecursive(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, visited map[string]bool) *scanner.FunctionInfo {
	if typeInfo == nil {
		return nil
	}
	typeKey := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if visited[typeKey] {
		return nil // Cycle detected
	}
	visited[typeKey] = true

	// 1. Search for a direct method on the current type.
	if methodInfo, err := a.findDirectMethodInfoOnType(ctx, typeInfo, methodName); err == nil && methodInfo != nil {
		return methodInfo
	}

	// 2. If not found, search in embedded structs.
	if typeInfo.Struct != nil {
		for _, field := range typeInfo.Struct.Fields {
			if field.Embedded {
				// The field.Type is the key. It might be a pointer type.
				// We need to resolve it to the actual TypeInfo to recurse.
				fieldType := field.Type
				if fieldType.IsPointer && fieldType.Elem != nil {
					fieldType = fieldType.Elem
				}
				embeddedTypeInfo, _ := fieldType.Resolve(ctx)

				if embeddedTypeInfo != nil {
					if foundMethod := a.findMethodInfoRecursive(ctx, embeddedTypeInfo, methodName, visited); foundMethod != nil {
						return foundMethod
					}
				}
			}
		}
	}

	return nil // Not found
}

func (a *accessor) findDirectMethodInfoOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string) (*scanner.FunctionInfo, error) {
	if typeInfo == nil {
		return nil, nil
	}

	// Handle interface types.
	if typeInfo.Interface != nil {
		// getAllInterfaceMethods handles embedded interfaces correctly.
		allMethods := a.eval.getAllInterfaceMethods(ctx, typeInfo, make(map[string]struct{}))
		for _, method := range allMethods {
			if method.Name == methodName {
				// We found the method in the interface definition.
				// We need to convert the MethodInfo to a FunctionInfo.
				// Interface methods don't have a body, so AstDecl will be nil.
				return &scanner.FunctionInfo{
					Name:       method.Name,
					Parameters: method.Parameters,
					Results:    method.Results,
					Owner:      typeInfo,
					Receiver: &scanner.FieldInfo{ // The receiver is the interface itself.
						Type: &scanner.FieldType{
							Name:           typeInfo.Name,
							PkgName:        typeInfo.PkgPath,
							FullImportPath: typeInfo.PkgPath,
							TypeName:       typeInfo.Name,
							Definition:     typeInfo,
						},
					},
					AstDecl: nil, // No AST declaration for interface methods.
				}, nil
			}
		}
		return nil, nil // Method not found in interface definition.
	}

	if typeInfo.PkgPath == "" {
		return nil, nil
	}

	if !a.eval.resolver.ScanPolicy(typeInfo.PkgPath) {
		return nil, nil
	}

	pkgObj, err := a.eval.getOrLoadPackage(ctx, typeInfo.PkgPath)
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
			fn.Owner = typeInfo
			return fn, nil
		}
	}
	return nil, nil
}
