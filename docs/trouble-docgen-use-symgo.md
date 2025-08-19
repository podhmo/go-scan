# Technical Hurdles in Using `symgo` for `docgen`

This document outlines the technical reasons why the initial approach of using the `symgo` symbolic execution engine for the `docgen` tool was abandoned in favor of manual AST traversal.

## Root Cause: The `symgo` Package Doesn't Exist

The fundamental issue, which caused a series of misleading module resolution errors, is that `github.com/podhmo/go-scan/symgo` is not a Go package. It is only a directory containing other packages (`evaluator`, `object`, etc.).

The initial error message, `no required module provides package .../symgo`, was literally correct. The attempts to fix this with `replace` directives or `go.work` files were misguided because there was no package to resolve in the first place.

The correct path forward is to create a new "facade" package at `symgo/symgo.go`. This package would provide a clean, public API for external tools like `docgen`, hiding the internal complexity of the evaluator and its sub-packages.

## Proposal: A `symgo.Interpreter` Facade

The correct path forward is to create a new "facade" package at `symgo/symgo.go`. This package would provide a clean, public API for external tools like `docgen`, hiding the internal complexity of the evaluator.

Inspired by `minigo/minigo.go`, this new package should introduce a central `Interpreter` type.

```go
// In new file symgo/interpreter.go (or symgo.go)

package symgo

import (
    "context"
    "go/ast"
    "go/types"

    goscan "github.com/podhmo/go-scan"
    "github.com/podhmo/go-scan/symgo/evaluator"
    "github.com/podhmo/go-scan/symgo/object"
)

// Re-export core object types for convenience.
type Object = object.Object
type Function = object.Function
type Error = object.Error

// IntrinsicFunc defines the signature for a custom function handler.
type IntrinsicFunc func(eval *Interpreter, args []Object) Object

// Interpreter is the main public entry point for the symgo engine.
type Interpreter struct {
    scanner *goscan.Scanner
    eval    *evaluator.Evaluator
    // ... other internal fields
}

// NewInterpreter creates a new symgo interpreter.
func NewInterpreter(scanner *goscan.Scanner) (*Interpreter, error) {
    // ... handles creation of internal evaluator, etc.
}

// Eval evaluates a given AST node in a new, empty environment.
func (i *Interpreter) Eval(ctx context.Context, node ast.Node) (Object, error) {
    // ... handles environment setup and calls the internal evaluator
}

// RegisterIntrinsic registers a custom handler for a given function object.
func (i *Interpreter) RegisterIntrinsic(target *types.Func, handler IntrinsicFunc) {
    // ... registers the handler with the internal intrinsics registry.
}
```

This `Interpreter` would solve the key problems:
1.  **Abstraction**: It hides the internal `evaluator` and `scanner` complexities.
2.  **Clean API**: It provides clear, public methods like `NewInterpreter`, `Eval`, and `RegisterIntrinsic`.
3.  **Type Safety**: It defines an exported `IntrinsicFunc` type with other exported types (`Interpreter`, `Object`), making it possible for external packages to implement handlers correctly.

## Original Problems Encountered (Symptoms of the Root Cause)

The lack of this facade package manifested as several API usability issues:

1.  **No Public Method for Intrinsic Registration**: The `symgo/evaluator.Evaluator` struct's `intrinsics` field is unexported, with no public method to add new intrinsics.
2.  **Internal `scanner` Type**: The `symgo/evaluator.New()` function required an internal `*scanner.Scanner`, not the public `*goscan.Scanner`.
3.  **Unexported Core Types**: The internal intrinsic callback signature used unexported types like `*evaluator.Scope`, making it impossible for external packages to implement.

## Final Approach: Manual AST Traversal

Given that the `symgo` facade package does not yet exist, the most practical solution was to bypass the `symgo` engine entirely. The final, working implementation relies only on the stable `go-scan` parsing functionality and the standard library's `go/ast` package to manually inspect the AST and find the necessary information.
