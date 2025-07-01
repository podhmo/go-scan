package main

import (
	"fmt"
	"strings"
)

func builtinStringsJoin(args ...Object) (Object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("strings.Join: expected 2 arguments, got %d", len(args))
	}

	arr, ok := args[0].(*Array) // Assuming Array type exists
	if !ok {
		// Temporary fallback for testing if Array is not ready:
		// If it's a string, assume it's a placeholder for elements "e1 e2 e3"
		if strElements, okStr := args[0].(*String); okStr {
			sepObj, okSep := args[1].(*String)
			if !okSep {
				return nil, fmt.Errorf("strings.Join: second argument must be a string separator, got %s", args[1].Type())
			}
			// This is a placeholder behavior: split the string by space and join with new separator
			elements := strings.Split(strElements.Value, " ")
			return &String{Value: strings.Join(elements, sepObj.Value)}, nil
		}
		return nil, fmt.Errorf("strings.Join: first argument must be an array (or a placeholder string for now), got %s", args[0].Type())
	}

	sep, ok := args[1].(*String)
	if !ok {
		return nil, fmt.Errorf("strings.Join: second argument must be a string separator, got %s", args[1].Type())
	}

	if len(arr.Elements) == 0 {
		return &String{Value: ""}, nil
	}

	strElements := make([]string, len(arr.Elements))
	for i, el := range arr.Elements {
		sEl, ok := el.(*String)
		if !ok {
			return nil, fmt.Errorf("strings.Join: all elements in the array must be strings, got %s", el.Type())
		}
		strElements[i] = sEl.Value
	}

	return &String{Value: strings.Join(strElements, sep.Value)}, nil
}

func builtinStringsToUpper(args ...Object) (Object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("strings.ToUpper: expected 1 argument, got %d", len(args))
	}
	s, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("strings.ToUpper: argument must be a string, got %s", args[0].Type())
	}
	return &String{Value: strings.ToUpper(s.Value)}, nil
}

func builtinStringsTrimSpace(args ...Object) (Object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("strings.TrimSpace: expected 1 argument, got %d", len(args))
	}
	s, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("strings.TrimSpace: argument must be a string, got %s", args[0].Type())
	}
	return &String{Value: strings.TrimSpace(s.Value)}, nil
}
