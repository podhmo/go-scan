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

// ErrUnresolvedEmbedded is a sentinel error returned when a method or field
// search fails because it traverses into an embedded type from a package
// that is out of the scan policy.
var ErrUnresolvedEmbedded = fmt.Errorf("unresolved embedded type")

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
	var encounteredUnresolved bool
	for _, field := range typeInfo.Struct.Fields {
		if field.Embedded {
			// If the embedded field itself has the name we're looking for (promoted field)
			if field.Name == fieldName {
				return field, nil
			}

			// An embedded field is considered "unresolved" if its import path is missing
			// (indicating incomplete type info from the scanner) or if it's explicitly
			// outside the scan policy.
			isUnresolved := field.Type.FullImportPath == "" || !a.eval.resolver.ScanPolicy(field.Type.FullImportPath)
			if isUnresolved {
				encounteredUnresolved = true
				continue // Don't stop; continue searching other embedded fields.
			}

			embeddedTypeInfo, _ := field.Type.Resolve(ctx)
			if embeddedTypeInfo != nil {
				foundField, err := a.findFieldRecursive(ctx, embeddedTypeInfo, fieldName, visited)
				if err != nil {
					if err == ErrUnresolvedEmbedded {
						encounteredUnresolved = true // Propagate unresolved status from deeper calls.
					} else {
						return nil, err // Propagate other, unexpected errors.
					}
				}
				if foundField != nil {
					return foundField, nil // Found it, we're done.
				}
			}
		}
	}

	// 3. If we finish the loop without finding the field, check if we hit an unresolved path.
	if encounteredUnresolved {
		return nil, ErrUnresolvedEmbedded
	}

	return nil, nil // Not found and no unresolved paths encountered.
}

// findMethodOnType recursively finds a method on a type or its embedded types.
// It returns a callable Function object if found.
func (a *accessor) findMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object, receiverPos token.Pos) (*object.Function, error) {
	if typeInfo == nil {
		return nil, nil // Cannot find method without type info
	}

	// Use a map to track visited types and prevent infinite recursion.
	visited := make(map[string]bool)
	return a.findMethodRecursive(ctx, typeInfo, methodName, env, receiver, receiverPos, visited)
}

func (a *accessor) findMethodRecursive(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object, receiverPos token.Pos, visited map[string]bool) (*object.Function, error) {
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
	if method, err := a.findDirectMethodOnType(ctx, typeInfo, methodName, env, receiver, receiverPos); err != nil || method != nil {
		return method, err
	}

	// 2. If not found, search in embedded structs.
	var encounteredUnresolved bool
	if typeInfo.Struct != nil {
		for _, field := range typeInfo.Struct.Fields {
			if field.Embedded {
				// An embedded field is considered "unresolved" if its import path is missing
				// (indicating incomplete type info from the scanner) or if it's explicitly
				// outside the scan policy.
				isUnresolved := field.Type.FullImportPath == "" || !a.eval.resolver.ScanPolicy(field.Type.FullImportPath)
				if isUnresolved {
					encounteredUnresolved = true
					continue // Don't stop; continue searching other embedded fields.
				}

				embeddedTypeInfo, _ := field.Type.Resolve(ctx)
				if embeddedTypeInfo != nil {
					// Recursive call, passing the original receiver.
					foundFn, err := a.findMethodRecursive(ctx, embeddedTypeInfo, methodName, env, receiver, receiverPos, visited)
					if err != nil {
						if err == ErrUnresolvedEmbedded {
							encounteredUnresolved = true // Propagate unresolved status from deeper calls.
						} else {
							return nil, err // Propagate other, unexpected errors.
						}
					}
					if foundFn != nil {
						return foundFn, nil // Found it.
					}
				}
			}
		}
	}

	// 3. If we finish the loop without finding the method, check if we hit an unresolved path.
	if encounteredUnresolved {
		return nil, ErrUnresolvedEmbedded
	}

	return nil, nil // Not found and no unresolved paths encountered.
}

func (a *accessor) findDirectMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object, receiverPos token.Pos) (*object.Function, error) {
	methodInfo, err := a.findDirectMethodInfoOnType(ctx, typeInfo, methodName)
	if err != nil || methodInfo == nil {
		return nil, err
	}

	pkgObj, err := a.eval.getOrLoadPackage(ctx, typeInfo.PkgPath)
	if err != nil {
		return nil, fmt.Errorf("package for method not found: %w", err)
	}

	// Get the base function object (without a receiver).
	// This might be cached or resolved on the fly.
	baseFnObj := a.eval.getOrResolveFunction(ctx, pkgObj, methodInfo)

	var baseFn *object.Function
	switch fn := baseFnObj.(type) {
	case *object.Function:
		baseFn = fn
	case *object.Instance:
		// If it's an instance of a generic function, the actual function
		// might be in the 'Underlying' field.
		if underlyingFn, ok := fn.Underlying.(*object.Function); ok {
			baseFn = underlyingFn
		}
	}

	if baseFn == nil {
		a.eval.logc(ctx, slog.LevelError, "resolved method is not a function object", "method", methodName, "got", fmt.Sprintf("%T", baseFnObj))
		return nil, fmt.Errorf("resolved method %q is not a function object, but %T", methodName, baseFnObj)
	}

	// Create a new function object with the receiver and its position bound.
	boundFn := baseFn.WithReceiver(receiver, receiverPos)
	return boundFn, nil
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
				embeddedTypeInfo, _ := field.Type.Resolve(ctx)
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
	if typeInfo == nil || typeInfo.PkgPath == "" {
		return nil, nil
	}

	pkgObj, err := a.eval.getOrLoadPackage(ctx, typeInfo.PkgPath)
	if err != nil || pkgObj.ScannedInfo == nil {
		if pkgObj.ScannedInfo == nil {
			a.eval.logc(ctx, slog.LevelDebug, "could not get or load package for method resolution", "package", typeInfo.PkgPath)
			return nil, nil
		}
		if err != nil && strings.Contains(err.Error(), "cannot find package") {
			return nil, nil
		}
		a.eval.logc(ctx, slog.LevelWarn, "unexpected error for method resolution", "package", typeInfo.PkgPath, "error", err)
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
			return fn, nil
		}
	}
	return nil, nil
}
