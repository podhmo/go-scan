package goscan

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// Implements checks if a struct type implements an interface type.
// It uses a robust, scanner-based analysis to resolve methods and types,
// correctly handling pointer/value receivers and embedded types.
func Implements(ctx context.Context, s *Scanner, structCandidate *scanner.TypeInfo, interfaceDef *scanner.TypeInfo) bool {
	analyzer := NewTypeRelation(s, s.Logger)
	return analyzer.isImplementer(ctx, structCandidate, interfaceDef)
}

// TypeRelation provides methods for analyzing relationships between types,
// such as interface implementation. It holds the necessary context, like the scanner,
// to perform these analyses.
type TypeRelation struct {
	scanner *Scanner
	logger  *slog.Logger
}

// NewTypeRelation creates a new TypeRelation analyzer.
func NewTypeRelation(scanner *Scanner, logger *slog.Logger) *TypeRelation {
	if logger == nil {
		logger = slog.Default() // Avoid nil logger
	}
	return &TypeRelation{
		scanner: scanner,
		logger:  logger,
	}
}

// isImplementer checks if a given concrete type implements an interface.
// This is ported from the more robust symgo/evaluator.
func (tr *TypeRelation) isImplementer(ctx context.Context, concreteType *scanner.TypeInfo, interfaceType *scanner.TypeInfo) bool {
	if concreteType == nil || interfaceType == nil || interfaceType.Interface == nil {
		return false
	}
	if concreteType.Kind != StructKind {
		return false
	}
	if interfaceType.Kind != InterfaceKind {
		return false
	}

	// Get all methods from the interface, including from embedded interfaces.
	allInterfaceMethods := tr.getAllInterfaceMethods(ctx, interfaceType, make(map[string]struct{}))

	// For every method in the complete method set...
	for _, ifaceMethodInfo := range allInterfaceMethods {
		// ...find a matching method in the concrete type.
		// A concrete type T can implement an interface method with a *T receiver.
		// So we need to check both T and *T.
		concreteMethodInfo := tr.findMethodInfoOnType(ctx, concreteType, ifaceMethodInfo.Name)

		if concreteMethodInfo == nil && !strings.HasPrefix(concreteType.Name, "*") {
			// If not found on T, check on *T.
			// Create a synthetic pointer type for the check.
			pointerType := *concreteType
			pointerType.Name = "*" + concreteType.Name
			concreteMethodInfo = tr.findMethodInfoOnType(ctx, &pointerType, ifaceMethodInfo.Name)
		}

		if concreteMethodInfo == nil {
			return false // Method not found
		}

		// Compare signatures
		if len(ifaceMethodInfo.Parameters) != len(concreteMethodInfo.Parameters) {
			return false
		}
		if len(ifaceMethodInfo.Results) != len(concreteMethodInfo.Results) {
			return false
		}

		for i, p1 := range ifaceMethodInfo.Parameters {
			p2 := concreteMethodInfo.Parameters[i]
			if !tr.fieldTypeEquals(p1.Type, p2.Type) {
				return false
			}
		}

		for i, r1 := range ifaceMethodInfo.Results {
			r2 := concreteMethodInfo.Results[i]
			if !tr.fieldTypeEquals(r1.Type, r2.Type) {
				return false
			}
		}
	}
	return true
}

// getAllInterfaceMethods recursively collects all methods from an interface and its embedded interfaces.
// It handles cycles by keeping track of visited interface types.
func (tr *TypeRelation) getAllInterfaceMethods(ctx context.Context, ifaceType *scanner.TypeInfo, visited map[string]struct{}) []*scanner.MethodInfo {
	if ifaceType == nil || ifaceType.Interface == nil {
		return nil
	}

	// Cycle detection
	typeName := ifaceType.PkgPath + "." + ifaceType.Name
	if _, ok := visited[typeName]; ok {
		return nil
	}
	visited[typeName] = struct{}{}

	var allMethods []*scanner.MethodInfo
	allMethods = append(allMethods, ifaceType.Interface.Methods...)

	for _, embeddedField := range ifaceType.Interface.Embedded {
		embeddedTypeInfo, err := embeddedField.Resolve(ctx)
		if err != nil {
			tr.logger.WarnContext(ctx, "could not resolve embedded interface", "type", embeddedField.String(), "error", err)
			continue
		}

		if embeddedTypeInfo != nil && embeddedTypeInfo.Kind == InterfaceKind {
			// Recursively get methods from the embedded interface.
			embeddedMethods := tr.getAllInterfaceMethods(ctx, embeddedTypeInfo, visited)
			allMethods = append(allMethods, embeddedMethods...)
		}
	}

	return allMethods
}

// fieldTypeEquals compares two FieldType objects for equality.
// It uses the string representation for a robust comparison of the type structure.
func (tr *TypeRelation) fieldTypeEquals(a, b *scanner.FieldType) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.String() == b.String()
}

// findMethodInfoOnType finds the scanner.FunctionInfo for a method on a type, handling embedding.
// Ported from symgo/evaluator/accessor.go
func (tr *TypeRelation) findMethodInfoOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string) *scanner.FunctionInfo {
	if typeInfo == nil {
		return nil
	}
	visited := make(map[string]bool)
	return tr.findMethodInfoRecursive(ctx, typeInfo, methodName, visited)
}

func (tr *TypeRelation) findMethodInfoRecursive(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, visited map[string]bool) *scanner.FunctionInfo {
	if typeInfo == nil {
		return nil
	}
	typeKey := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	if visited[typeKey] {
		return nil // Cycle detected
	}
	visited[typeKey] = true

	// 1. Search for a direct method on the current type.
	if methodInfo, err := tr.findDirectMethodInfoOnType(ctx, typeInfo, methodName); err == nil && methodInfo != nil {
		return methodInfo
	}

	// 2. If not found, search in embedded structs.
	if typeInfo.Struct != nil {
		for _, field := range typeInfo.Struct.Fields {
			if field.Embedded {
				embeddedTypeInfo, _ := field.Type.Resolve(ctx)
				if embeddedTypeInfo != nil {
					if foundMethod := tr.findMethodInfoRecursive(ctx, embeddedTypeInfo, methodName, visited); foundMethod != nil {
						return foundMethod
					}
				}
			}
		}
	}

	return nil // Not found
}

func (tr *TypeRelation) findDirectMethodInfoOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string) (*scanner.FunctionInfo, error) {
	if typeInfo == nil || typeInfo.PkgPath == "" {
		return nil, nil
	}

	// Check the scanner's cache first to avoid re-scanning.
	tr.scanner.mu.RLock()
	methodPkg, exists := tr.scanner.packageCache[typeInfo.PkgPath]
	tr.scanner.mu.RUnlock()

	if !exists {
		// If not in cache, use the scanner to get the package info.
		var err error
		methodPkg, err = tr.scanner.ScanPackage(ctx, typeInfo.PkgPath)
		if err != nil {
			// Suppress errors that are expected for certain types (e.g., built-ins, unresolved packages).
			if strings.Contains(err.Error(), "cannot find package") || strings.Contains(err.Error(), "no such file or directory") {
				return nil, nil
			}
			tr.logger.WarnContext(ctx, "could not load package for method resolution", "package", typeInfo.PkgPath, "error", err)
			return nil, nil
		}
	}

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
