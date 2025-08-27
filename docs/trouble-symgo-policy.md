# Troubleshooting: Deferred Evaluation Policy in `symgo`

## Goal

The primary goal is to modify the `symgo` symbolic execution engine to handle types from external packages (including the standard library) without requiring a full source code scan upfront. The engine should treat these types as opaque placeholders until a method is actually called on them.

## Current State & Problem

The core logic for this "deferred evaluation" policy has been implemented.

-   A new object, `object.UnresolvedMethodCall`, is created when the evaluator encounters a method call on a variable whose type is from an unscanned package.
-   The core `symgo` tests, including a new `TestInterfaceBinding_Deferred` test, pass successfully, verifying this placeholder mechanism.

However, a suite of tests for the `examples/docgen` tool are persistently failing.

The root cause appears to be that when a method with multiple return values (e.g., `io.Writer.Write() (int, error)`) is called on a deferred type, the evaluator is incorrectly returning a single placeholder object instead of a multi-return object with the correct number of placeholders. This causes a `expected multi-return value on RHS of assignment` warning during evaluation and leads to the `docgen` tool failing to extract necessary API information.

## Attempts to Fix

My primary attempt to fix this was to make the `applyFunction` in the evaluator "smarter". When it receives an `UnresolvedMethodCall`, it's supposed to:

1.  Identify the receiver's type and package path (e.g., `net/http.ResponseWriter`).
2.  Trigger a "just-in-time" scan of that package using the `scanner.ScanPackageByImport` API.
3.  Look up the type and method information from the newly scanned package data.
4.  Use the method's signature to return the correct number of placeholder values.
5.  This logic was extended to look for methods on both interfaces and structs.

This approach has not worked, and the `docgen` tests continue to fail with the same error. The reason for the failure of this fix is unclear.

## Next Steps & Debugging

To diagnose why the just-in-time scanning and method lookup is failing, I have added a temporary debug test (`TestDebugNetHttp`) to `examples/docgen/main_test.go`.

This test explicitly calls `scanner.ScanPackageByImport("net/http")` and logs the methods it finds for key types like `ResponseWriter` and `Request`.

The immediate next step is to run `make test` and examine the output of this debug test to verify what information the scanner is actually providing for the `net/http` package. This should reveal if my assumptions about the scanner's output are correct or if there's a bug in the scanning/parsing logic itself.

## Files Changed So Far

-   `symgo/object/object.go`: Added `UnresolvedMethodCall` object type.
-   `symgo/evaluator/evaluator.go`: Main changes to implement the deferred evaluation policy and the failing "just-in-time" scan logic in `applyFunction`.
-   `symgo/symgo_interface_binding_test.go`: Split the original `TestInterfaceBinding` to test both resolved and deferred paths.
-   `examples/docgen/main_test.go`: Added the `TestDebugNetHttp` for diagnostics.
-   `examples/docgen/main.go`: Added `net/http` to `extraPackages` for the analyzer.
