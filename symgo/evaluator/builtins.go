package evaluator

import scannerv2 "github.com/podhmo/go-scan/scanner"

var (
	// ErrorInterfaceTypeInfo is a pre-constructed TypeInfo for the built-in error interface.
	// This is necessary because the scanner may not always be able to resolve built-in types
	// to their full interface definition, especially in minimal test setups.
	ErrorInterfaceTypeInfo *scannerv2.TypeInfo
)

func init() {
	// Manually construct the TypeInfo for the `error` interface.
	// The `error` interface is defined as:
	// type error interface {
	//     Error() string
	// }
	stringFieldType := &scannerv2.FieldType{
		Name:      "string",
		IsBuiltin: true,
	}
	errorMethod := &scannerv2.MethodInfo{
		Name: "Error",
		Results: []*scannerv2.FieldInfo{
			{
				Type: stringFieldType,
			},
		},
	}
	ErrorInterfaceTypeInfo = &scannerv2.TypeInfo{
		Name: "error",
		Kind: scannerv2.InterfaceKind,
		Interface: &scannerv2.InterfaceInfo{
			Methods: []*scannerv2.MethodInfo{errorMethod},
		},
	}
}
