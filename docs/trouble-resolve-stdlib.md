# Troubleshooting: Resolving Standard Library Types in Tests

## 1. Summary

When running tests for tools that use the `go-scan` library, there is a persistent issue with resolving types from the standard library, particularly pointer types like `*time.Time`.

The scanner fails with a `mismatched package names` error when it attempts to parse the source files of a standard library package (e.g., `time`) from within a Go test binary. This document details the problem, the attempted solutions, and the current hypothesis about the cause.

## 2. The Problem

The issue was discovered while writing a test for the `convert-define` tool. The test, located in `examples/convert/cmd/convert-define/internal/interpreter_test.go`, attempts to parse a Go file that defines a conversion rule for `*time.Time`.

### Triggering Code

The test sets up a `minigo.Interpreter` with a `go-scan.Scanner`. The scanner is configured with `goscan.WithGoModuleResolver()` to find packages. The test then runs the interpreter on the following Go code:

```go
// testdata/mappings.go
package main

import (
	"github.com/podhmo/go-scan/examples/convert/convutil"
	"github.com/podhmo/go-scan/examples/convert/define"
)

func main() {
	define.Rule(convutil.TimeToString)
	define.Rule(convutil.PtrTimeToString) // This line triggers the error
}
```
The `convutil.PtrTimeToString` function has the signature `func(ctx context.Context, ec *model.ErrorCollector, t *time.Time) string`.

### Error Message

When the parser attempts to resolve the `*time.Time` type, the scanner produces the following error:

```
runner.Run() failed: evaluating define file: runtime error: could not resolve source type for rule: failed to scan package "time" for type "Time": ScanPackageByImport: scanning files for time failed: mismatched package names: time and main in directory /usr/local/go/src/time
```

## 3. Attempted Solutions & Analysis

This "mismatched package names" error is a known issue when scanning GOROOT packages from a test. The standard workaround is to use the `goscan.WithExternalTypeOverrides` scanner option.

### Attempt 1: Override `time.Time`

The first attempt was to provide an override for the base type, `time.Time`.

```go
// interpreter_test.go
overrides := scanner.ExternalTypeOverride{
    "time.Time": &scanner.TypeInfo{
        Name:    "Time",
        PkgPath: "time",
        Kind:    scanner.StructKind,
    },
}
runner, err := NewRunner(
    goscan.WithGoModuleResolver(),
    goscan.WithExternalTypeOverrides(overrides),
)
```
**Result:** This successfully resolved the `define.Rule(convutil.TimeToString)` call, but still failed on `PtrTimeToString` with the same error. This indicates the override works for the base type but not for a pointer to it.

### Attempt 2: Override `*time.Time`

The next attempt was to add an override for the pointer type directly.

```go
// interpreter_test.go
overrides := scanner.ExternalTypeOverride{
    "time.Time": &scanner.TypeInfo{...},
    "*time.Time": &scanner.TypeInfo{ // Added this
        Name:    "Time",
        PkgPath: "time",
        Kind:    scanner.StructKind,
    },
}
```
**Result:** This had no effect. The error remained identical. The `scanner.ExternalTypeOverride` map key is defined as `ImportPath + "." + TypeName`, so a key like `"*time.Time"` is likely ignored by the scanner, which does not expect a `*` prefix.

### Attempt 3: Test-local Modules

A third attempt involved creating a test-local `go.mod` and a mocked `convutil` package to remove the dependency on the real `time` package. While this allowed the parser logic to be tested in isolation, it did not solve the underlying problem of using the real types.

## 4. Hypothesis

The `go-scan` scanner's `ExternalTypeOverride` feature does not seem to be fully effective for pointer types of external packages. When `go-scan` resolves a `FieldType` for `*time.Time`, it correctly identifies it as a pointer to an element of type `time.Time`. However, when resolving that element, it does not appear to hit the override cache for `"time.Time"`. Instead, it proceeds to try and scan the `time` package from GOROOT, which fails inside a test binary.

The expected behavior is that the scanner should check the override map for `"time.Time"` before attempting to scan the package's source files, even when resolving the element of a pointer type.

This suggests a potential bug or limitation in the scanner's type resolution logic. A future task should be to debug the `go-scan` library to fix this behavior.
