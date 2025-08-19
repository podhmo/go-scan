# Technical Hurdles in Using `symgo` for `docgen`

This document outlines the technical reasons why the initial approach of using the `symgo` symbolic execution engine for the `docgen` tool was abandoned in favor of manual AST traversal.

## Initial Goal

The original plan was for `docgen` to be a consumer of the `symgo` library. The `docgen` analyzer would instantiate the `symgo` evaluator and register an "intrinsic" function for `net/http.HandleFunc`. When the evaluator encountered a call to `http.HandleFunc`, it would execute the intrinsic, which would then extract the route and handler information.

## Problems Encountered

When attempting to implement this, several compilation errors arose due to the design of the `symgo` public API. The engine appears to be designed as a self-contained system, and it does not currently expose the necessary components for an external tool like `docgen` to effectively interact with it.

The key issues were:

1.  **No Public Method for Intrinsic Registration**:
    *   The `symgo/evaluator.Evaluator` struct contains an unexported `intrinsics` field (`*intrinsics.Registry`).
    *   There is no public method on the `Evaluator` (e.g., `RegisterIntrinsic()`) to add a new intrinsic function from an external package.
    *   This made it impossible for `docgen` to teach the evaluator how to handle `http.HandleFunc`.

2.  **Internal `scanner` Type vs. Public `goscan` Type**:
    *   The `symgo/evaluator.New()` function expected an argument of type `*scanner.Scanner`.
    *   However, the main `go-scan` package provides a `*goscan.Scanner`, which is a wrapper. While a helper method `ScannerForSymgo()` exists, it signals that the two components are not yet cleanly integrated.

3.  **Unexported Core Types**:
    *   The callback function for an intrinsic required a `*evaluator.Scope` argument, but this type was not exported, making it impossible to write a valid callback signature.

4.  **Lack of Public API for Core Operations**:
    *   Methods that seemed necessary for writing an intrinsic, such as `EvalExpr()` to evaluate function arguments, were not public methods on the `Evaluator`.
    *   Error creation helpers (`NewError`) and core object types (`Void`) were also not exposed in a way that was easily usable from the outside.

## Final Approach: Manual AST Traversal

Given these API limitations, it was clear that using the `symgo` evaluator as a black box was not feasible in its current state. The path of least resistance and greatest stability was to pivot to a simpler approach:

1.  Use `go-scan` to parse the target package and get the `*ast.File` and `*scanner.PackageInfo`.
2.  Manually find the `RegisterHandlers` function declaration (`*ast.FuncDecl`).
3.  Use the standard library's `ast.Inspect` function to walk the function's body.
4.  Inside the AST walker, look for `*ast.CallExpr` nodes that correspond to `http.HandleFunc`.
5.  Extract the arguments (path and handler name) directly from the AST nodes.

This approach bypasses the `symgo` engine entirely and relies only on the stable `go-scan` parsing functionality and the `go/ast` library.

## Future Work

To enable the original vision, the `symgo` package would need to be refactored to provide a clear, public API for consumers. This would likely involve:
-   Adding a `RegisterIntrinsic(fn types.Object, handlerFunc ...)` method to the `Evaluator`.
-   Exporting necessary types like `Scope` or providing an abstraction so that external intrinsics can be written safely.
-   Providing public helper methods on the evaluator for common operations needed within an intrinsic, like evaluating an argument expression.
