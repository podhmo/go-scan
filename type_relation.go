package goscan

import (
	"context"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// ImplementsContext checks if a struct type implements an interface type.
// It uses a PackageResolver to look up packages and resolve types, making it suitable for cross-package analysis.
func ImplementsContext(ctx context.Context, structCandidate *scanner.TypeInfo, interfaceDef *scanner.TypeInfo, resolver scanner.PackageResolver) bool {
	if structCandidate == nil || structCandidate.Kind != scanner.StructKind {
		return false
	}
	if interfaceDef == nil || interfaceDef.Kind != scanner.InterfaceKind || interfaceDef.Interface == nil {
		return false
	}
	if resolver == nil {
		return false // Resolver is required for cross-package method lookup.
	}

	// Get the package of the struct candidate to find its methods.
	structPkg, err := resolver.ScanPackageByImport(ctx, structCandidate.PkgPath)
	if err != nil {
		// Cannot resolve the package, so we cannot determine its methods.
		return false
	}

	structMethods := make(map[string]*scanner.FunctionInfo)
	for _, fn := range structPkg.Functions {
		if fn.Receiver == nil || fn.Receiver.Type == nil {
			continue
		}

		// Check if the receiver type matches the struct candidate.
		// We need to handle both pointer and value receivers.
		// fn.Receiver.Type is the receiver's type. Let's compare it with structCandidate.
		// A simple name check is not enough for cross-package types.
		// Let's resolve the receiver type and compare its definition with the structCandidate.
		receiverType := fn.Receiver.Type
		if receiverType.IsPointer {
			// If the receiver is a pointer, its element type should match the struct.
			receiverType = receiverType.Elem
		}

		if receiverType.TypeName == structCandidate.Name && receiverType.FullImportPath == structCandidate.PkgPath {
			structMethods[fn.Name] = fn
		}
	}

	for _, interfaceMethod := range interfaceDef.Interface.Methods {
		structMethod, found := structMethods[interfaceMethod.Name]
		if !found {
			return false // Method not found
		}

		// Compare signatures (parameters and results)
		if !compareSignaturesContext(ctx, interfaceMethod, structMethod) {
			return false
		}
	}

	return true
}

func compareSignaturesContext(ctx context.Context, interfaceMethod *scanner.MethodInfo, structMethod *scanner.FunctionInfo) bool {
	// Compare parameters
	if len(interfaceMethod.Parameters) != len(structMethod.Parameters) {
		return false
	}
	for i, intParam := range interfaceMethod.Parameters {
		strParam := structMethod.Parameters[i]
		if !compareFieldTypesContext(ctx, intParam.Type, strParam.Type) {
			return false
		}
	}

	// Compare results
	if len(interfaceMethod.Results) != len(structMethod.Results) {
		return false
	}
	for i, intResult := range interfaceMethod.Results {
		strResult := structMethod.Results[i]
		if !compareFieldTypesContext(ctx, intResult.Type, strResult.Type) {
			return false
		}
	}
	return true
}

func compareFieldTypesContext(ctx context.Context, type1, type2 *scanner.FieldType) bool {
	if type1 == nil && type2 == nil {
		return true
	}
	if type1 == nil || type2 == nil {
		return false
	}

	// First, check the basic structure (pointer, slice, map).
	if type1.IsPointer != type2.IsPointer || type1.IsSlice != type2.IsSlice || type1.IsMap != type2.IsMap {
		return false
	}

	if type1.IsSlice {
		return compareFieldTypesContext(ctx, type1.Elem, type2.Elem)
	}
	if type1.IsMap {
		return compareFieldTypesContext(ctx, type1.MapKey, type2.MapKey) && compareFieldTypesContext(ctx, type1.Elem, type2.Elem)
	}

	// For non-composite types, resolve them to their definitions.
	def1, err1 := type1.Resolve(ctx)
	def2, err2 := type2.Resolve(ctx)

	// If there was an error resolving, we can't be sure they are the same.
	// (Note: could log these errors for debugging)
	if err1 != nil || err2 != nil {
		// If resolution fails, it might be because the type is defined in a package
		// that is not being scanned. As a fallback, we compare the string
		// representation of the types (e.g., "pkg.MyType").
		if err1 != nil && err2 != nil {
			return type1.String() == type2.String()
		}
		// If one resolved and the other didn't, they are not equal.
		return false
	}

	// Case 1: Both types resolve to a full TypeInfo definition.
	if def1 != nil && def2 != nil {
		// The most robust check: do they point to the exact same definition object?
		// The scanner should ensure a single canonical TypeInfo for each type.
		return def1 == def2
	}

	// Case 2: One resolves, the other doesn't. They can't be the same.
	if (def1 == nil) != (def2 == nil) {
		return false
	}

	// Case 3: Both resolve to nil. This happens for built-in types (e.g., 'string', 'int', 'error').
	// In this case, their names must match.
	if def1 == nil && def2 == nil {
		return type1.Name == type2.Name
	}

	// Should not be reached.
	return false
}

// Implements checks if a struct type implements an interface type within the context of a package.
// It requires the PackageInfo to look up methods of the structCandidate.
// Deprecated: Use ImplementsContext for cross-package analysis.
func Implements(structCandidate *scanner.TypeInfo, interfaceDef *scanner.TypeInfo, pkgInfo *scanner.PackageInfo) bool {
	if structCandidate == nil || structCandidate.Kind != StructKind {
		return false // Candidate must be a struct
	}
	if interfaceDef == nil || interfaceDef.Kind != InterfaceKind || interfaceDef.Interface == nil {
		return false // Interface definition must be a valid interface
	}
	if pkgInfo == nil {
		return false // Package context is needed to find struct methods
	}

	// Collect methods of the structCandidate from pkgInfo.Functions
	// This is a simplified way; a more robust way might involve caching methods on TypeInfo.
	structMethods := make(map[string]*scanner.FunctionInfo)
	for _, fn := range pkgInfo.Functions {
		if fn.Receiver != nil && fn.Receiver.Type != nil {
			receiverTypeName := fn.Receiver.Type.Name
			// Handle pointer receivers, e.g. "*MyStruct" vs "MyStruct"
			if fn.Receiver.Type.IsPointer && len(receiverTypeName) > 0 && receiverTypeName[0] == '*' {
				actualReceiverName := receiverTypeName
				if fn.Receiver.Type.IsPointer && strings.HasPrefix(receiverTypeName, "*") {
					actualReceiverName = strings.TrimPrefix(receiverTypeName, "*")
				}

				if actualReceiverName == structCandidate.Name {
					structMethods[fn.Name] = fn
				}
			} else if receiverTypeName == structCandidate.Name { // Value receiver
				structMethods[fn.Name] = fn
			}
		}
	}

	for _, interfaceMethod := range interfaceDef.Interface.Methods {
		structMethod, found := structMethods[interfaceMethod.Name]
		if !found {
			// fmt.Printf("Method %s not found on struct %s\n", interfaceMethod.Name, structCandidate.Name)
			return false // Method not found
		}

		// Compare signatures (parameters and results)
		if !compareSignatures(interfaceMethod, structMethod) {
			// fmt.Printf("Signature mismatch for method %s on struct %s\n", interfaceMethod.Name, structCandidate.Name)
			return false
		}
	}

	return true
}

// compareSignatures compares the parameters and results of two methods.
// This is a simplified comparison focusing on type names and counts.
// It does not handle complex type equivalences (e.g., type aliases across packages without full resolution).
func compareSignatures(interfaceMethod *scanner.MethodInfo, structMethod *scanner.FunctionInfo) bool {
	// Compare parameters
	if len(interfaceMethod.Parameters) != len(structMethod.Parameters) {
		return false
	}
	for i, intParam := range interfaceMethod.Parameters {
		strParam := structMethod.Parameters[i]
		if !compareFieldTypes(intParam.Type, strParam.Type) {
			return false
		}
	}

	// Compare results
	if len(interfaceMethod.Results) != len(structMethod.Results) {
		return false
	}
	for i, intResult := range interfaceMethod.Results {
		strResult := structMethod.Results[i]
		if !compareFieldTypes(intResult.Type, strResult.Type) {
			return false
		}
	}
	return true
}

// compareFieldTypes compares two FieldType instances.
// This is a simplified comparison. A robust solution needs full type resolution.
func compareFieldTypes(type1 *scanner.FieldType, type2 *scanner.FieldType) bool {
	if type1 == nil && type2 == nil {
		return true
	}
	if type1 == nil || type2 == nil {
		return false
	}
	if type1.IsPointer != type2.IsPointer {
		return false
	}
	if type1.IsSlice != type2.IsSlice {
		return false
	}
	if type1.IsSlice { // Both are slices
		return compareFieldTypes(type1.Elem, type2.Elem) // Compare element types
	}
	if type1.IsMap != type2.IsMap {
		return false
	}
	if type1.IsMap { // Both are maps
		if !compareFieldTypes(type1.MapKey, type2.MapKey) {
			return false
		}
		return compareFieldTypes(type1.Elem, type2.Elem)
	}
	if type1.Name != type2.Name {
		return false
	}
	return true
}
