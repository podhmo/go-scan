package main

import "fmt"

// builtinFmtSprintf implements the fmt.Sprintf built-in function.
func builtinFmtSprintf(args ...Object) (Object, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("fmt.Sprintf: expected at least 1 argument, got %d", len(args))
	}
	formatStr, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("fmt.Sprintf: first argument must be a string, got %s", args[0].Type())
	}

	sArgs := make([]interface{}, len(args)-1)
	for i, arg := range args[1:] {
		switch a := arg.(type) {
		case *String:
			sArgs[i] = a.Value
		case *Integer:
			sArgs[i] = a.Value
		default:
			return nil, fmt.Errorf("fmt.Sprintf: unsupported argument type %s for format string", arg.Type())
		}
	}

	result := fmt.Sprintf(formatStr.Value, sArgs...)
	return &String{Value: result}, nil
}
