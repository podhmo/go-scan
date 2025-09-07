package evaluator

import (
	"context"
	"fmt"
	"strings"

	"github.com/podhmo/go-scan/scanner"
)

// Implements checks if a struct type implements an interface type.
// It uses the evaluator's resolver to look up method definitions across packages.
func Implements(ctx context.Context, structCandidate *scanner.TypeInfo, interfaceDef *scanner.TypeInfo, eval *Evaluator) (bool, error) {
	if structCandidate == nil || structCandidate.Kind != scanner.StructKind {
		return false, nil // Candidate must be a struct
	}
	if interfaceDef == nil || interfaceDef.Kind != scanner.InterfaceKind || interfaceDef.Interface == nil {
		return false, nil // Interface definition must be a valid interface
	}
	if eval == nil {
		return false, fmt.Errorf("evaluator is required for cross-package implementation checks")
	}

	// In a cross-package scenario, structCandidate's methods might not be in the same package
	// as the interface. We need to load the struct's package to find its methods.
	structPkgPath := structCandidate.PkgPath
	pkgObj, err := eval.getOrLoadPackage(ctx, structPkgPath)
	if err != nil || pkgObj.ScannedInfo == nil {
		return false, fmt.Errorf("failed to load package for struct %s: %w", structCandidate.Name, err)
	}
	structPkg := pkgObj.ScannedInfo

	// Collect methods of the structCandidate from its package's functions.
	structMethods := make(map[string]*scanner.FunctionInfo)
	for _, fn := range structPkg.Functions {
		if fn.Receiver != nil && fn.Receiver.Type != nil {
			receiverTypeName := fn.Receiver.Type.Name
			actualReceiverName := receiverTypeName
			if fn.Receiver.Type.IsPointer && strings.HasPrefix(receiverTypeName, "*") {
				actualReceiverName = strings.TrimPrefix(receiverTypeName, "*")
			}

			if actualReceiverName == structCandidate.Name {
				structMethods[fn.Name] = fn
			}
		}
	}

	// Now check if all methods of the interface are implemented by the struct.
	for _, interfaceMethod := range interfaceDef.Interface.Methods {
		structMethod, found := structMethods[interfaceMethod.Name]
		if !found {
			return false, nil // Method not found
		}

		// Compare signatures (parameters and results)
		// This comparison must be able to resolve types across packages.
		match, err := compareSignatures(ctx, interfaceMethod, structMethod, eval)
		if err != nil {
			return false, fmt.Errorf("error comparing signatures for method %s: %w", interfaceMethod.Name, err)
		}
		if !match {
			return false, nil
		}
	}

	return true, nil
}

// compareSignatures compares the parameters and results of two methods.
// It uses the evaluator's resolver to handle type resolution across packages.
func compareSignatures(ctx context.Context, interfaceMethod *scanner.MethodInfo, structMethod *scanner.FunctionInfo, eval *Evaluator) (bool, error) {
	// Compare parameters
	if len(interfaceMethod.Parameters) != len(structMethod.Parameters) {
		return false, nil
	}
	for i, intParam := range interfaceMethod.Parameters {
		strParam := structMethod.Parameters[i]
		match, err := compareFieldTypes(ctx, intParam.Type, strParam.Type, eval)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}

	// Compare results
	if len(interfaceMethod.Results) != len(structMethod.Results) {
		return false, nil
	}
	for i, intResult := range interfaceMethod.Results {
		strResult := structMethod.Results[i]
		match, err := compareFieldTypes(ctx, intResult.Type, strResult.Type, eval)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

// compareFieldTypes compares two FieldType instances, resolving types if necessary.
func compareFieldTypes(ctx context.Context, type1, type2 *scanner.FieldType, eval *Evaluator) (bool, error) {
	if type1 == nil && type2 == nil {
		return true, nil
	}
	if type1 == nil || type2 == nil {
		return false, nil
	}

	// Basic kind checks
	if type1.IsPointer != type2.IsPointer || type1.IsSlice != type2.IsSlice || type1.IsMap != type2.IsMap {
		return false, nil
	}

	// Recursive comparison for complex types
	if type1.IsSlice {
		return compareFieldTypes(ctx, type1.Elem, type2.Elem, eval)
	}
	if type1.IsMap {
		keyMatch, err := compareFieldTypes(ctx, type1.MapKey, type2.MapKey, eval)
		if err != nil || !keyMatch {
			return false, err
		}
		return compareFieldTypes(ctx, type1.Elem, type2.Elem, eval)
	}

	// Core type comparison: resolve types to their canonical representation.
	resolvedType1 := eval.resolver.ResolveType(ctx, type1)
	resolvedType2 := eval.resolver.ResolveType(ctx, type2)

	if resolvedType1 == nil && resolvedType2 == nil {
		// This can happen for built-in types that don't have a full TypeInfo.
		// In this case, comparing their string representation is a good fallback.
		return type1.String() == type2.String(), nil
	}
	if resolvedType1 == nil || resolvedType2 == nil {
		return false, nil // One is resolved, the other is not.
	}

	// Compare the fully qualified names of the resolved types.
	id1 := fmt.Sprintf("%s.%s", resolvedType1.PkgPath, resolvedType1.Name)
	id2 := fmt.Sprintf("%s.%s", resolvedType2.PkgPath, resolvedType2.Name)

	if id1 != id2 {
		return false, nil
	}

	return true, nil
}
