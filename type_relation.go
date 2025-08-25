package goscan

import (
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// Implements checks if a struct type implements an interface type within the context of a package.
// It requires the PackageInfo to look up methods of the structCandidate.
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
				// This comparison is simplistic. True type resolution is complex.
				// For now, assume Type.Name for pointer receiver is like "*StructName".
				// This might need adjustment based on how FieldType.Name for pointer types is structured.
				// Let's assume fn.Receiver.Type.Name for `*Foo` is `*Foo`, and for `Foo` is `Foo`.
				// The receiver type name might need stripping of '*' for comparison if structCandidate.Name doesn't have it.
				// Or, ensure structCandidate.Name is used consistently.
				// For now, let's assume fn.Receiver.Type.Name is the base name for pointer receivers after parsing.
				// This is a common point of failure if not handled carefully by the parser.
				// Let's assume fn.Receiver.Type.Name is "MyStruct" even for *MyStruct for simplicity here, needs verification.
				// Based on scanner.go, parseFuncDecl gets receiver type via parseTypeExpr.
				// FieldType.Name for *ast.StarExpr prepends "*" if not handled.
				// Let's assume for now fn.Receiver.Type.Name could be "*StructName" or "StructName"
				// And structCandidate.Name is "StructName".

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
func compareSignatures(interfaceMethod *scanner.FunctionInfo, structMethod *scanner.FunctionInfo) bool {
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

	// TODO: This needs to be much more robust.
	// It should handle qualified names, resolve types if necessary, etc.
	// For now, simple name and pointer check.
	// Also, consider IsSlice, IsMap, Elem, MapKey for more complex types.

	// Normalize names: if PkgName is present and type1/2 are from different packages,
	// we need to compare fully qualified names or ensure types are resolved to canonical forms.
	// For types within the same package or primitives, direct name comparison might work.
	// ft.Resolve() could be used here, but adds complexity of error handling and async operations.

	name1 := type1.Name
	name2 := type2.Name

	// Thus, we can directly compare IsPointer and then Name.

	if type1.IsPointer != type2.IsPointer {
		return false
	}

	// Handle slices
	if type1.IsSlice != type2.IsSlice {
		return false
	}
	if type1.IsSlice { // Both are slices
		return compareFieldTypes(type1.Elem, type2.Elem) // Compare element types
	}

	// Handle maps
	if type1.IsMap != type2.IsMap {
		return false
	}
	if type1.IsMap { // Both are maps
		// Compare key types AND value types
		if !compareFieldTypes(type1.MapKey, type2.MapKey) {
			return false
		}
		return compareFieldTypes(type1.Elem, type2.Elem)
	}

	// If not slices or maps, compare base names (IsPointer is already checked and equal)
	// This is where PkgName/ImportPath should be checked for non-primitive, non-builtin types.
	// For now, just comparing names.
	if name1 != name2 {
		// Consider logging here for debugging type mismatches:
		// fmt.Printf("Base name mismatch: T1: %s (pkg:%s) vs T2: %s (pkg:%s)\n", name1, type1.PkgName, name2, type2.PkgName)
		return false
	}

	// TODO: Enhance PkgName and fullImportPath comparison for robust cross-package type identity.
	// For example:
	// if type1.PkgName != type2.PkgName {
	//    // If PkgName is different, names must be fully qualified or resolved via import paths
	//    // This requires type1.FullImportPath and type2.FullImportPath to be populated and compared.
	//    // For now, if PkgName differs and names were identical (e.g. "MyType"), it's a mismatch unless they are built-in.
	//    isBuiltinOrPredeclared := func(name string) bool {
	//        // Add checks for "string", "int", "bool", "error", etc.
	//        // Or rely on PkgName being empty or a special value for builtins.
	//        // scanner.FieldType might need a field like IsBuiltin.
	// 	   return name == "string" || name == "int" // ... and so on
	//    }
	//    if !(isBuiltinOrPredeclared(name1) && type1.PkgName == "" && type2.PkgName == "") && /* more conditions */ {
	//        return false
	//    }
	// }

	return true
}
