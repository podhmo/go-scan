package evaluator

import "github.com/podhmo/go-scan/scanner"

var (
	// ErrorInterfaceTypeInfo is a pre-constructed TypeInfo for the built-in error interface.
	// This is necessary because the scanner may not always be able to resolve built-in types
	// to their full interface definition, especially in minimal test setups.
	ErrorInterfaceTypeInfo *scanner.TypeInfo
)

func init() {
	// Manually construct the TypeInfo for the `error` interface.
	// The `error` interface is defined as:
	// type error interface {
	//     Error() string
	// }
	stringFieldType := &scanner.FieldType{
		Name:      "string",
		IsBuiltin: true,
	}
	errorMethod := &scanner.MethodInfo{
		Name: "Error",
		Results: []*scanner.FieldInfo{
			{
				Type: stringFieldType,
			},
		},
	}
	ErrorInterfaceTypeInfo = &scanner.TypeInfo{
		Name: "error",
		Kind: scanner.InterfaceKind,
		Interface: &scanner.InterfaceInfo{
			Methods: []*scanner.MethodInfo{errorMethod},
		},
	}
}
