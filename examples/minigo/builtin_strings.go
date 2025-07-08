package main

import (
	"fmt"
	"strings"
)

// evalStringsJoin handles the execution of a strings.Join call.
// It expects two arguments:
// 1. A Slice of String objects.
// 2. A String object for the separator.
func evalStringsJoin(args ...Object) (Object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("strings.Join expects exactly two arguments (a slice of strings and a separator string), got %d", len(args))
	}

	elementsSlice, ok := args[0].(*Slice)
	if !ok {
		return nil, fmt.Errorf("first argument to strings.Join must be a SLICE, got %s", args[0].Type())
	}

	separatorObj, ok := args[1].(*String)
	if !ok {
		return nil, fmt.Errorf("second argument to strings.Join (separator) must be a STRING, got %s", args[1].Type())
	}
	separator := separatorObj.Value

	stringElements := make([]string, len(elementsSlice.Elements))
	for i, elemObj := range elementsSlice.Elements {
		strObj, ok := elemObj.(*String)
		if !ok {
			return nil, fmt.Errorf("element %d in the slice for strings.Join must be a STRING, got %s", i, elemObj.Type())
		}
		stringElements[i] = strObj.Value
	}

	result := strings.Join(stringElements, separator)
	return &String{Value: result}, nil
}

// GetBuiltinStringsFunctions returns a map of built-in functions related to strings.
func GetBuiltinStringsFunctions() map[string]*BuiltinFunction {
	return map[string]*BuiltinFunction{
		"strings.Join": {
			Fn: func(env *Environment, args ...Object) (Object, error) {
				// env is not used by strings.Join, but the signature requires it.
				// Error formatting for argument count mismatch or type mismatch will be handled by evalStringsJoin.
				// The BuiltinFunction wrapper in evalCallExpr doesn't need to know the specifics here.
				return evalStringsJoin(args...)
			},
			Name: "strings.Join",
		},
		// Add other strings functions here if needed, e.g., strings.HasPrefix, strings.Contains
		// Example for strings.Contains (if we were adding it here and not auto-generating)
		/*
			"strings.Contains": {
				Fn: func(env *Environment, args ...Object) (Object, error) {
					if len(args) != 2 {
						return nil, fmt.Errorf("strings.Contains expects two arguments, got %d", len(args))
					}
					s, okS := args[0].(*String)
					if !okS {
						return nil, fmt.Errorf("first argument to strings.Contains must be STRING, got %s", args[0].Type())
					}
					substr, okSubstr := args[1].(*String)
					if !okSubstr {
						return nil, fmt.Errorf("second argument to strings.Contains must be STRING, got %s", args[1].Type())
					}
					return nativeBoolToBooleanObject(strings.Contains(s.Value, substr.Value)), nil
				},
				Name: "strings.Contains",
			},
		*/
		// "strings.ToUpper": {
		// 	Fn: func(env *Environment, args ...Object) (Object, error) {
		// 		if len(args) != 1 {
		// 			return nil, fmt.Errorf("strings.ToUpper expects exactly one argument")
		// 		}
		// 		strObj, ok := args[0].(*String)
		// 		if !ok {
		// 			return nil, fmt.Errorf("argument to strings.ToUpper must be a STRING, got %s", args[0].Type())
		// 		}
		// 		return &String{Value: strings.ToUpper(strObj.Value)}, nil
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
//
// Post-refactor note: evalStringsJoin now implements the Go-idiomatic approach.
