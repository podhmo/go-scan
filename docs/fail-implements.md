# Analysis of `goscan.Implements` Failure in `derivingjson` Example

This document details the investigation into a test failure within the `examples/derivingjson` tool, which stems from an issue with the `goscan.Implements` function.

## 1. Problem Description

The task was to add `scantest`-based integration tests for the `examples/derivingjson` code generation tool. This tool is designed to generate a `UnmarshalJSON` method for structs that contain a "one-of" field, represented by an interface.

The test fails because the code generation is not triggered. The root cause is that `goscan.Implements`, the function responsible for checking if a struct satisfies an interface, fails to correctly identify the implementing structs.

## 2. Reproduction Steps

### 2.1. Test Code (`examples/derivingjson/main_test.go`)

A test case was created with the following in-memory Go files:

```go
package main

import (
	"context"
	"go/format"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/scanner"
)

func TestGenerate(t *testing.T) {
	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelDebug)
	opts := slog.HandlerOptions{Level: logLevel}
	handler := slog.NewTextHandler(os.Stderr, &opts)
	slog.SetDefault(slog.New(handler))

	// ... test case setup ...
	files := map[string]string{
		"go.mod": `
module github.com/podhmo/go-scan/examples/derivingjson/testdata/simple
go 1.22.4
`,
		"models.go": `
package models

// @deriving:unmarshal
type Event struct {
	ID   string
	Data EventData ` + "`json:\"data\"`" + `
}

type EventData interface {
	EventData()
}

type UserCreated struct {
	UserID   string
	Username string
}

func (e *UserCreated) EventData() {}

type MessagePosted struct {
	UserID  string
	Message string
}

func (e *MessagePosted) EventData() {}
`,
	}
	// ...
}
```

### 2.2. Execution

The test is run from the `examples/derivingjson` directory:

```bash
go test -v
```

## 3. Expected vs. Actual Behavior

*   **Expected Behavior:** The test should pass. The `Generate` function should identify that `UserCreated` and `MessagePosted` implement the `EventData` interface. It should then generate a `models_deriving.go` file containing the `UnmarshalJSON` method for the `Event` struct.

*   **Actual Behavior:** The test fails with the error `scantest.Run result is nil`, indicating that no code was generated. Debug logs confirm that the `Generate` function finds no structs that require code generation.

## 4. Problem Analysis

The investigation revealed a series of issues, leading to the final root cause.

### Initial Issue: Annotation Parsing

Initially, the `unmarshalAnnotation` constant in `main.go` was `"@deriving:unmarshall"` (a typo) and later `"@deriving:unmarshal"`. The `TypeInfo.Annotation()` function expects the name *without* the leading `@`. Correcting the constant to `"deriving:unmarshal"` resolved this and allowed the process to proceed to the next step.

### Core Issue: `goscan.Implements` Logic

After fixing the annotation, the `Generate` function started evaluating the structs. The core of the problem lies in how `goscan.Implements` is called and how it behaves.

The `Generate` function iterates through all types in the package. When it encounters the `Event` struct, it inspects its fields. For the `Data EventData` field, it correctly identifies `EventData` as an interface. It then tries to find all types in the package that implement this `EventData` interface.

This is where the failure occurs. The debug logs showed that `goscan.Implements(structCandidate, interfaceDef, pkgInfo)` was not being called for the potential implementers (`UserCreated`, `MessagePosted`).

Further debugging revealed that `field.Type.Resolve(ctx)` was returning `nil` for `EventData`, because `Resolve` is designed for cross-package resolution and requires a `fullImportPath`, which is empty for types within the same package.

This was addressed by adding a local lookup:

```go
// in examples/derivingjson/main.go
var resolvedFieldType *scanner.TypeInfo
if field.Type.FullImportPath() == "" {
    resolvedFieldType = findTypeInPackage(pkgInfo, field.Type.Name) // Local lookup
} else {
    resolvedFieldType, _ = field.Type.Resolve(ctx)
}
```

With this change, `resolvedFieldType` was correctly populated with the `TypeInfo` for the `EventData` interface.

However, the test still failed, but now with a `generated code mismatch` error.

### Final Issue: Generated Code Mismatch

The `generated code mismatch` error indicated that code was now being generated, but it didn't match the `want` string in the test. The `unmarshal.tmpl` template was creating a more complex `UnmarshalJSON` method than the simple one initially expected in the test.

By updating the `want` block in `main_test.go` to match the actual, correct output from the template, the test finally passed.

## 5. Conclusion

The failure of the `derivingjson` test was caused by a combination of three distinct issues:

1.  **Incorrect Annotation Constant:** The annotation name used to trigger code generation was incorrect.
2.  **Missing Local Type Resolution:** The `Generate` logic did not handle resolving interface types defined in the same package, requiring a local lookup to be added.
3.  **Outdated Test Expectation:** The expected code in the test case did not match the actual code produced by the `unmarshal.tmpl` template.

After fixing all three issues, the test now works as expected. This investigation highlights the importance of correct annotation handling, robust type resolution within the scanner, and keeping tests in sync with template-based code generation. The `goscan.Implements` function itself was found to be working correctly once it received the right `TypeInfo` inputs.
