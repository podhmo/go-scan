# Technical Hurdles in Using `symgo` for `docgen`

This document outlines the technical reasons why the initial approach of using the `symgo` symbolic execution engine for the `docgen` tool was abandoned in favor of manual AST traversal.

## Root Cause: The `symgo` Package Doesn't Exist

The fundamental issue, which caused a series of misleading module resolution errors, is that `github.com/podhmo/go-scan/symgo` is not a Go package. It is only a directory containing other packages (`evaluator`, `object`, etc.).

The initial error message, `no required module provides package .../symgo`, was literally correct. The attempts to fix this with `replace` directives or `go.work` files were misguided because there was no package to resolve in the first place.

The correct path forward is to create a new "facade" package at `symgo/symgo.go`. This package would provide a clean, public API for external tools like `docgen`, hiding the internal complexity of the evaluator and its sub-packages.

## Required Features for a `symgo` Facade Package

To be usable by external tools, the new `symgo` package should expose the following:

1.  **A Public `Evaluator` Type**: An exported struct or interface that wraps the internal `symgo/evaluator.Evaluator`.
2.  **A Public Constructor**: A `NewEvaluator(scanner *goscan.Scanner)` function that handles the setup, including bridging the `*goscan.Scanner` and the internal `*scanner.Scanner`.
3.  **Public `Eval` Method**: A method on the public `Evaluator` to start the evaluation of a given AST node (e.g., `Eval(ctx context.Context, node ast.Node)`).
4.  **Public Intrinsic Registration**: A method like `RegisterIntrinsic(fn types.Object, handlerFunc MyIntrinsicFunc)` to allow external tools to register custom handlers for functions.
5.  **Exported Supporting Types**:
    *   The `MyIntrinsicFunc` callback signature must be defined with exported types (e.g., `func(eval *Evaluator, args []Object) Object`).
    *   Core types from `symgo/object` should be re-exported or aliased for convenience, such as `Object`, `Function`, `Error`, `String`, and `Void`.

## Original Problems Encountered (Symptoms of the Root Cause)

The lack of this facade package manifested as several API usability issues:

1.  **No Public Method for Intrinsic Registration**: The `symgo/evaluator.Evaluator` struct's `intrinsics` field is unexported, with no public method to add new intrinsics.
2.  **Internal `scanner` Type**: The `symgo/evaluator.New()` function required an internal `*scanner.Scanner`, not the public `*goscan.Scanner`.
3.  **Unexported Core Types**: The internal intrinsic callback signature used unexported types like `*evaluator.Scope`, making it impossible for external packages to implement.

## Final Approach: Manual AST Traversal

Given that the `symgo` facade package does not yet exist, the most practical solution was to bypass the `symgo` engine entirely. The final, working implementation relies only on the stable `go-scan` parsing functionality and the standard library's `go/ast` package to manually inspect the AST and find the necessary information.
