# Trouble Report: `encoding/json` Support in MiniGo

This document outlines the issues encountered while trying to implement and test support for the `encoding/json` standard library package in MiniGo.

## Goal

The goal was to generate bindings for the `encoding/json` package and verify that `json.Marshal` and `json.Unmarshal` could work with a user-defined struct within a MiniGo script.

## Test Case

The following test was added to `minigo/minigo_stdlib_test.go` to verify the functionality.

```go
func TestStdlib_json(t *testing.T) {
	script := `
package main
import "encoding/json"

type Point struct {
	X int
	Y int
}

var p1 = Point{X: 10, Y: 20}
var data, err1 = json.Marshal(p1)

var p2 Point
var err2 = json.Unmarshal(data, &p2)
`
	interp, err := minigo.NewInterpreter()
	if err != nil {
		t.Fatalf("failed to create interpreter: %+v", err)
	}
	stdjson.Install(interp) // stdjson is from "github.com/podhmo/go-scan/minigo/stdlib/encoding/json"

	if err := interp.LoadFile("test.mgo", []byte(script)); err != nil {
		t.Fatalf("failed to load script: %+v", err)
	}
	if _, err := interp.Eval(context.Background()); err != nil {
		t.Fatalf("failed to evaluate script: %+v", err)
	}

    // ... (assertions to check the values of err1, err2, data, and p2)
}
```

## Problem & Error Message

The test failed during the evaluation of the script. The script failed at the `json.Marshal(p1)` call.

The error message from the MiniGo interpreter was:

```
runtime error: argument 1 type mismatch: unsupported conversion from STRUCT_INSTANCE to interface{}
```

## Analysis of Discrepancy

*   **Expected Behavior:** The `json.Marshal` function should accept the MiniGo struct instance `p1`, treat it as a Go `any` (or `interface{}`), and successfully serialize it into a JSON byte slice. Subsequently, `json.Unmarshal` should be able to deserialize the data back into another struct instance.

*   **Actual Behavior:** The MiniGo interpreter's interoperability layer, which handles calls from MiniGo script to native Go functions, does not know how to convert a MiniGo `object.StructInstance` into a Go `interface{}`. The `json.Marshal` function in Go has the signature `func(v any) ([]byte, error)`, and the MiniGo runtime correctly identifies that `p1` (an `object.StructInstance`) is not directly assignable to the `any` type that the Go function expects.

## Missing Features in `go-scan` and `minigo`

This failure highlights a fundamental limitation in the current implementation of MiniGo's foreign function interface (FFI). To fully support packages like `encoding/json`, the following features are needed:

1.  **Bi-directional Type Conversion:** The interpreter needs a robust mechanism to convert values between the MiniGo object system (`minigo/object`) and Go's native types, especially for `interface{}`.
    *   **MiniGo to Go:** When a MiniGo object (like `object.StructInstance`) is passed to a Go function expecting `any`, the interpreter should be able to "unbox" the MiniGo object into a corresponding Go value (e.g., a `map[string]interface{}` or a dynamically created Go struct instance via `reflect`).
    *   **Go to MiniGo:** When a Go function returns a value, the interpreter must "box" it into a corresponding MiniGo object. This part is already implemented for basic types, but it would need to handle complex types like structs and slices returned from functions.

2.  **Pointer and Addressability Support:** The `json.Unmarshal` function requires a pointer to a variable (`&p2`). The MiniGo interpreter needs to be able to handle taking the address of a variable and passing that pointer to a Go function. The Go function would then need to be able to write back to the memory location of the MiniGo variable. This is a complex feature that requires careful memory management.

3.  **Struct Field Tag Support:** For `encoding/json` to be truly useful, the MiniGo struct definition would need to support field tags (e.g., `` `json:"x_coordinate"` ``). The current `go-scan` parser and `minigo` object system do not have a concept of struct field tags. The `json.Marshal` function relies on these tags for customizing the output.

In summary, while the bindings for the functions in `encoding/json` can be generated, their practical use is blocked by the MiniGo runtime's current inability to bridge the gap between its own type system and Go's reflection-based `interface{}` and pointer semantics.
