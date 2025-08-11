# Troubleshooting: Resolving Standard Library Types in Tests

## 1. Summary

When running tests for tools that use the `go-scan` library, there was a persistent issue with resolving types from the standard library, particularly pointer types like `*time.Time`.

The scanner failed with a `mismatched package names` error when it attempted to parse the source files of a standard library package (e.g., `time`) from within a Go test binary. This document details the problem, the investigation, and the final solution.

## 2. The Problem

The issue was discovered while writing a test for the `convert-define` tool. The test, located in `examples/convert/cmd/convert-define/internal/interpreter_test.go`, attempts to parse a Go file that defines a conversion rule for `*time.Time`. A similar issue was also found in an integration test for the `convert` tool, which used a pointer to a stdlib type in a struct field.

### Triggering Code

The tests would fail when trying to process code that used `*time.Time`, either as a function argument or a struct field, when an `ExternalTypeOverride` was provided for `time.Time`.

```go
// Example 1: Function argument
define.Rule(convutil.PtrTimeToString) // PtrTimeToString is func(..., t *time.Time) string

// Example 2: Struct field
// @derivingconvert("Dst")
type Src struct {
	CreatedAt *time.Time // This field would cause the error
}
```

### Error Message

When the parser attempted to resolve the `*time.Time` type, the scanner produced the following error:

```
runner.Run() failed: evaluating define file: runtime error: could not resolve source type for rule: failed to scan package "time" for type "Time": ScanPackageByImport: scanning files for time failed: mismatched package names: time and main in directory /usr/local/go/src/time
```

## 3. Investigation & Root Cause

The "mismatched package names" error is a known issue when scanning GOROOT packages from a test. The standard workaround is to use the `goscan.WithExternalTypeOverrides` scanner option to provide a synthetic definition for stdlib types like `time.Time`.

While this worked for `time.Time`, it failed for `*time.Time`. The investigation revealed a two-part problem in the type resolution logic.

### Part 1: Pointer Resolution in `FieldType.Resolve`

The initial hypothesis was that the `FieldType.Resolve` method was not correctly handling pointers to overridden types. When `Resolve()` was called on a `FieldType` for `*time.Time`, it would see that the pointer type itself didn't have a `Definition` and would immediately try to scan the `"time"` package, causing the error.

This was fixed by adding logic to `FieldType.Resolve` in `scanner/models.go` to first check if the type is a pointer. If so, it recursively calls `Resolve()` on the pointer's element. If the element is resolved (e.g., via an override), the pointer itself is considered resolved, and the element's `TypeInfo` is returned. This prevented the unnecessary package scan for the case in `interpreter_test.go`.

### Part 2: Propagation of `IsResolvedByConfig`

After fixing Part 1, a regression appeared in another test (`TestIntegration_WithPointerAwareGlobalRule`). The `mismatched package names` error persisted.

Further debugging revealed that a downstream consumer of the resolved type, `parser.collectFields`, was performing its own check to avoid re-scanning packages:

```go
// parser/parser.go
if fieldTypeInfo != nil && ... && !f.Type.IsResolvedByConfig {
    fieldPkgInfo, err := s.ScanPackageByImport(ctx, fieldTypeInfo.PkgPath) // <-- Error here
    // ...
}
```
The problem was that while `*time.Time` was now being correctly *resolved* to the `TypeInfo` of `time.Time`, the original `FieldType` for the pointer (`f.Type`) did not have its `IsResolvedByConfig` flag set to `true`. The flag was `true` on the *element* type (`time.Time`), but this status was not being propagated to the pointer `FieldType` that wrapped it during the initial parsing phase.

This caused the check `!f.Type.IsResolvedByConfig` to pass, wrongly triggering another package scan.

## 4. Solution

The root cause was fixed in `scanner/scanner.go` within the `parseTypeExpr` function. When parsing a pointer (`*ast.StarExpr`), the `IsResolvedByConfig` field from the element type is now explicitly copied to the new pointer `FieldType`.

```go
// scanner/scanner.go

// ... in parseTypeExpr
case *ast.StarExpr:
    elemType := s.parseTypeExpr(ctx, t.X, currentTypeParams, info, importLookup)
    return &FieldType{
        // ... other fields
        IsResolvedByConfig: elemType.IsResolvedByConfig, // Propagate from element
    }
```

This ensures that any logic checking the resolution status of a `FieldType` will correctly identify that a pointer to an overridden type is also considered "resolved by config", preventing incorrect and failing package scans. This single change fixed both failing test cases.
