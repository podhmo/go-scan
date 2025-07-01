package main

import (
	"fmt"
	"strings"
)

// newError is a helper to create a new error object.
// We'll need to define ErrorObject if it's not already part of object.go
// For now, assume it returns a standard Go error, and the caller wraps it.
func newError(format string, a ...interface{}) error {
	return fmt.Errorf(format, a...)
}

var Builtins = map[string]*Builtin{
	"fmt_Sprintf": {
		Fn: func(args ...Object) (Object, error) {
			if len(args) < 1 {
				return nil, newError("wrong number of arguments for fmt.Sprintf: got=%d, want>=1", len(args))
			}
			formatStr, ok := args[0].(*String)
			if !ok {
				return nil, newError("first argument to fmt.Sprintf must be STRING, got %s", args[0].Type())
			}

			// Convert MiniGo objects to Go types for Sprintf
			goArgs := make([]interface{}, len(args)-1)
			for i, arg := range args[1:] {
				switch obj := arg.(type) {
				case *String:
					goArgs[i] = obj.Value
				case *Integer:
					goArgs[i] = obj.Value
				case *Boolean:
					goArgs[i] = obj.Value
				// Add other types as needed
				default:
					return nil, newError("unsupported type for fmt.Sprintf argument: %s", obj.Type())
				}
			}
			return &String{Value: fmt.Sprintf(formatStr.Value, goArgs...)}, nil
		},
		Name: "fmt.Sprintf",
	},
	"strings_Join": {
		Fn: func(args ...Object) (Object, error) {
			if len(args) != 2 {
				return nil, newError("wrong number of arguments for strings.Join: got=%d, want=2", len(args))
			}
			// First arg must be an array of strings. We'll need an Array object type.
			// For now, this will be a placeholder until Array is implemented.
			// arr, ok := args[0].(*Array) // Placeholder for future Array type
			// if !ok {
			// 	return nil, newError("first argument to strings.Join must be ARRAY, got %s", args[0].Type())
			// }
			// For now, let's assume it's passed as a String object with elements separated by a special char,
			// or we wait for Array implementation.
			arr, ok := args[0].(*Array)
			if !ok {
				return nil, newError("first argument to strings.Join must be ARRAY, got %s", args[0].Type())
			}

			sep, ok := args[1].(*String)
			if !ok {
				return nil, newError("second argument to strings.Join must be STRING, got %s", args[1].Type())
			}

			var elems []string
			for _, item := range arr.Elements {
				strItem, ok := item.(*String)
				if !ok {
					// If the evalCompositeLit ensures all elements are strings for []string,
					// this check might be redundant but good for safety if arrays can be constructed otherwise.
					return nil, newError("all elements in array for strings.Join must be STRING, got %s for element %s", item.Type(), item.Inspect())
				}
				elems = append(elems, strItem.Value)
			}
			return &String{Value: strings.Join(elems, sep.Value)}, nil
		},
		Name: "strings.Join",
	},
	"strings_ToUpper": {
		Fn: func(args ...Object) (Object, error) {
			if len(args) != 1 {
				return nil, newError("wrong number of arguments for strings.ToUpper: got=%d, want=1", len(args))
			}
			str, ok := args[0].(*String)
			if !ok {
				return nil, newError("argument to strings.ToUpper must be STRING, got %s", args[0].Type())
			}
			return &String{Value: strings.ToUpper(str.Value)}, nil
		},
		Name: "strings.ToUpper",
	},
	"strings_TrimSpace": {
		Fn: func(args ...Object) (Object, error) {
			if len(args) != 1 {
				return nil, newError("wrong number of arguments for strings.TrimSpace: got=%d, want=1", len(args))
			}
			str, ok := args[0].(*String)
			if !ok {
				return nil, newError("argument to strings.TrimSpace must be STRING, got %s", args[0].Type())
			}
			return &String{Value: strings.TrimSpace(str.Value)}, nil
		},
		Name: "strings.TrimSpace",
	},
}

// GetBuiltinByName retrieves a builtin function by its name.
// This can be used by the interpreter to look up builtins.
func GetBuiltinByName(name string) *Builtin {
	// For simplicity, direct map access. Could add prefix handling later if needed (e.g. "fmt.Sprintf")
	// The keys in Builtins map are already "package_Function" style.
	if builtin, ok := Builtins[name]; ok {
		return builtin
	}
	return nil
}
