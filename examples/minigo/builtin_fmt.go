package main

import (
	"fmt"
	"strings" // Needed for strings.Join in evalFmtPrintln
)

// evalFmtSprintf handles the execution of a fmt.Sprintf call.
// It expects the first argument to be a format string (String object)
// and subsequent arguments to be the values to format.
func evalFmtSprintf(args ...Object) (Object, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("fmt.Sprintf expects at least one argument (format string)")
	}

	formatStringObj, ok := args[0].(*String)
	if !ok {
		return nil, fmt.Errorf("first argument to fmt.Sprintf must be a STRING, got %s", args[0].Type())
	}
	formatString := formatStringObj.Value

	// Convert remaining MiniGo objects to Go native types for Sprintf.
	// This is a simplification. A more robust solution would handle various MiniGo types.
	nativeArgs := make([]interface{}, len(args)-1)
	for i, arg := range args[1:] {
		switch obj := arg.(type) {
		case *String:
			nativeArgs[i] = obj.Value
		case *Integer:
			nativeArgs[i] = obj.Value
		case *Boolean:
			nativeArgs[i] = obj.Value
		// Add other types as needed
		default:
			return nil, fmt.Errorf("unsupported type %s for fmt.Sprintf argument at index %d", obj.Type(), i+1)
		}
	}

	// Perform the Sprintf operation using Go's native fmt.Sprintf.
	// Note: This is a direct call to Go's fmt.Sprintf. Security implications
	// (like format string vulnerabilities) from user-provided format strings
	// are relevant if MiniGo were used in a sensitive context.
	// For this example, we assume valid and safe usage.
	result := fmt.Sprintf(formatString, nativeArgs...)
	return &String{Value: result}, nil
}

// Helper to create a BuiltinFunction object for fmt.Sprintf
func newFmtSprintfBuiltin() *BuiltinFunction {
	return &BuiltinFunction{
		Fn: func(env *Environment, args ...Object) (Object, error) { // Modified signature
			return evalFmtSprintf(args...)
		},
		Name: "fmt.Sprintf",
	}
}

// Example of how it might be registered in the interpreter setup:
// interpreter.globalEnv.Define("fmt.Sprintf", newFmtSprintfBuiltin())
// (Actual registration will be handled in interpreter.go as per the plan)
//
// To make "fmt" itself a namespace or package-like structure,
// we might need a more complex object type, like a Module or Struct,
// that can hold other functions/variables.
// For now, we'll register "fmt.Sprintf" as a flat function name.
//
// A more advanced way could be:
// fmtModule := NewModule("fmt")
// fmtModule.Define("Sprintf", &BuiltinFunction{Fn: evalFmtSprintf, Name: "Sprintf"})
// env.Define("fmt", fmtModule)
// Then calls would be like `fmt.Sprintf(...)` parsed as `Ident{Name:"fmt"}` Dot `Ident{Name:"Sprintf"}`.
// Current plan is to treat "fmt.Sprintf" as a single identifier for simplicity.

func (i *Interpreter) registerBuiltinFmt(env *Environment) {
	// We need a way to make `fmt.Sprintf` callable.
	// This could be by defining a global variable `fmt.Sprintf` that holds a BuiltinFunction object.
	// Or, more elaborately, by handling `ast.SelectorExpr` (e.g., `fmt.Sprintf`)
	// where `fmt` is an object (perhaps a module or map) that has a `Sprintf` method/key.

	// For simplicity with the current plan (treating "fmt.Sprintf" as a single identifier for the CallExpr's Fun):
	// We'll need to ensure that when `ast.CallExpr` has `Fun` as an `ast.Ident` with name "fmt.Sprintf",
	// our `evalIdentifier` or `evalCallExpr` can find this built-in.

	// The plan step 4 mentions: "グローバル環境または適切な場所に、fmt.Sprintf ... を登録する処理を追加します。"
	// This registration will happen in NewInterpreter or a similar setup function.
	// This file (builtin_fmt.go) defines the *implementation* of the function.

	// No direct registration code here, as it depends on the BuiltinFunction object type
	// and the interpreter's environment structure, which are defined/modified in other steps.
	// This file provides the `evalFmtSprintf` function that will be wrapped.
}

// evalFmtPrintln handles the execution of a fmt.Println call.
// It converts arguments to strings using their Inspect() method,
// joins them with spaces, prints to standard output, and adds a newline.
func evalFmtPrintln(args ...Object) (Object, error) {
	outputParts := make([]string, len(args))
	for i, arg := range args {
		outputParts[i] = arg.Inspect()
	}
	fmt.Println(strings.Join(outputParts, " ")) // strings.Join is from Go's standard library
	return NULL, nil                             // Println returns no meaningful value
}

// GetBuiltinFmtFunctions returns a map of fmt built-in functions.
// This allows the interpreter to easily register them.
func GetBuiltinFmtFunctions() map[string]*BuiltinFunction {
	return map[string]*BuiltinFunction{
		"fmt.Sprintf": {
			Fn: func(env *Environment, args ...Object) (Object, error) { // Matching new signature
				return evalFmtSprintf(args...)
			},
			Name: "fmt.Sprintf",
		},
		"fmt.Println": {
			Fn: func(env *Environment, args ...Object) (Object, error) {
				return evalFmtPrintln(args...)
			},
			Name: "fmt.Println",
		},
	}
}

// Notes on original longer comments that were removed:
// - The removed comments discussed potential fmt.Println implementation,
//   BuiltinFunction signatures, error handling, type checking, string representation
//   for Sprintf, and how "fmt.Sprintf" might be parsed (Ident vs SelectorExpr).
// - These are useful design considerations but were causing build errors as
//   they were not fully commented out or were placed outside function bodies.
// - For brevity and to fix the build, they have been removed. The core logic
//   of GetBuiltinFmtFunctions and evalFmtSprintf remains.
