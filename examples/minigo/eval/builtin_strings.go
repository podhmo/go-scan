package eval

import (
	"fmt"
	"strings"

	"github.com/podhmo/go-scan/examples/minigo/object"
)

// evalStringsJoin handles the execution of a strings.Join call.
// For MiniGo, due to the lack of explicit array types yet, we'll adopt a convention:
// strings.Join(str1, str2, ..., strN, separator) will join str1 through strN using separator.
// This differs from Go's strings.Join(array, separator).
func evalStringsJoin(args ...object.Object) (object.Object, error) {
	if len(args) < 2 {
		// Needs at least one string to "join" (which would be itself) and a separator,
		// or effectively, at least two args for our convention: element1, separator.
		// Or, if we interpret it as str1, str2, ..., sep, then len must be >= 2.
		// Let's say: at least one element and one separator.
		return nil, fmt.Errorf("strings.Join expects at least two arguments (elements to join and a separator), got %d", len(args))
	}

	separatorObj, ok := args[len(args)-1].(*object.String)
	if !ok {
		return nil, fmt.Errorf("last argument to strings.Join (separator) must be a STRING, got %s", args[len(args)-1].Type())
	}
	separator := separatorObj.Value

	elementsToJoin := args[:len(args)-1]
	if len(elementsToJoin) == 0 {
		// This case means only a separator was provided, which is an invalid use
		// according to our convention (e.g. strings.Join(","))
		return nil, fmt.Errorf("strings.Join requires at least one element to join before the separator")
	}

	stringElements := make([]string, len(elementsToJoin))
	for i, arg := range elementsToJoin {
		strObj, ok := arg.(*object.String)
		if !ok {
			return nil, fmt.Errorf("argument %d to strings.Join (element to join) must be a STRING, got %s", i, arg.Type())
		}
		stringElements[i] = strObj.Value
	}

	result := strings.Join(stringElements, separator)
	return &object.String{Value: result}, nil
}

// GetBuiltinStringsFunctions returns a map of built-in functions related to strings.
func GetBuiltinStringsFunctions() map[string]*object.BuiltinFunction {
	return map[string]*object.BuiltinFunction{
		"strings.Join": {
			Fn: func(env object.Environment, args ...object.Object) (object.Object, error) { // Use object.Environment
				// env is not used by strings.Join, but the signature requires it.
				return evalStringsJoin(args...)
			},
			Name: "strings.Join",
		},
		// Add other strings functions here if needed, e.g., strings.HasPrefix, strings.Contains
		// "strings.ToUpper": {
		// 	Fn: func(env object.Environment, args ...object.Object) (object.Object, error) {
		// 		if len(args) != 1 {
		// 			return nil, fmt.Errorf("strings.ToUpper expects exactly one argument")
		// 		}
		// 		strObj, ok := args[0].(*object.String)
		// 		if !ok {
		// 			return nil, fmt.Errorf("argument to strings.ToUpper must be a STRING, got %s", args[0].Type())
		// 		}
		// 		return &object.String{Value: strings.ToUpper(strObj.Value)}, nil
		// 	},
		// 	Name: "strings.ToUpper",
		// },
	}
}

// Notes on original longer comments that were removed:
// - The removed comments discussed the variadic nature of the current strings.Join
//   implementation as a workaround for MiniGo's lack of array/slice types.
// - It also outlined how a more Go-idiomatic strings.Join (taking an array/slice
//   and a separator) would be implemented once MiniGo has those features.
// - These comments were causing build errors and have been removed for brevity.
//   The core logic of GetBuiltinStringsFunctions and evalStringsJoin remains.
