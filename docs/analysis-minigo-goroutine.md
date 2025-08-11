# Analysis of Goroutine Support in minigo

## 1. Introduction

This document analyzes the feasibility and potential implementation plan for adding support for goroutines and channels to the `minigo` interpreter. The goal is to determine if this can be achieved by simply leveraging the host language's (Go) concurrency primitives or if it would require substantial changes to the interpreter's architecture.

The primary constraint for this analysis is whether the implementation would be straightforward. If it requires a significant architectural redesign, the recommendation would be to not proceed.

## 2. Current Architecture of `minigo`

`minigo` is an AST-walking interpreter. Its core logic resides in the `minigo/evaluator/evaluator.go` file. The key components of its current architecture are:

*   **`Evaluator` Struct**: This is the main object that drives the interpretation process. It holds the state for a *single, sequential* execution, including:
    *   `callStack`: A stack of `CallFrame` objects that tracks the current function call hierarchy. This is a crucial piece of state for managing function scopes, `defer`, and `return`.
    *   `packages`: A cache of loaded packages.
    *   `registry`: A registry for symbols (functions, variables) injected from the host Go application.

*   **`Eval()` Function**: The `Eval(node, env, fscope)` function is the heart of the interpreter. It's a large, recursive function that traverses the Abstract Syntax Tree (AST) of a `minigo` script. For each node type, it performs the corresponding action (e.g., evaluating an expression, executing a statement).

*   **`object.Environment`**: This represents the lexical scope, mapping variable names to their `object.Object` values. Environments are chained together to represent nested scopes (e.g., a function call creates a new environment enclosed by the parent environment).

This architecture is inherently single-threaded. A single `Evaluator` instance represents a single thread of execution with its own stack and state. It is not designed for concurrent access.

## 3. Challenges of Introducing Concurrency

Directly using Go's goroutines to execute `minigo` code concurrently (e.g., `go evaluator.Eval(...)`) is not possible without major changes. Any attempt to do so would lead to severe race conditions and unpredictable behavior, primarily due to:

*   **Shared Mutable State**: The `Evaluator` struct is stateful and not thread-safe. If multiple Go goroutines called `Eval` on the same `Evaluator` instance, they would concurrently modify the `callStack`, package caches, and other internal fields, leading to data corruption. The `callStack` is the most critical and obvious point of contention.

*   **Lack of an Interpreter-Level Scheduler**: Go's runtime scheduler manages Go goroutines. It has no knowledge of `minigo`'s internal state. To support concurrency within `minigo`, the interpreter would need its own scheduler to manage `minigo` "goroutines," pausing and resuming them. This is necessary to handle blocking operations (like reading from a channel) without blocking the underlying OS thread.

*   **State Isolation**: Each concurrent task in `minigo` would need its own independent execution stack. The single `callStack` in the current `Evaluator` cannot be shared.

## 4. Potential Implementation Strategies

To properly support goroutines, a fundamental redesign of the interpreter is required. A `go` statement in `minigo` would initiate a new concurrent execution context.

```minigo
func myTask() {
    // ... do work
}

// This would need to be supported
go myTask()
```

Here is a high-level overview of what a proper implementation would entail:

1.  **Concurrent Execution Context**: Instead of a single `Evaluator`, we would need a "supervisor" or "runtime" object. When a `go` statement is executed, this runtime would create a new, lightweight execution context (let's call it a `minigoRoutine`). Each `minigoRoutine` would have:
    *   Its own independent call stack.
    *   A reference to the shared environment (or a copy, depending on the memory model).

2.  **Scheduler**: The runtime would need a scheduler to manage these `minigoRoutine`s. This scheduler would map them onto a pool of host Go goroutines. When a `minigoRoutine` performs a blocking operation (e.g., `<-ch`), the scheduler would suspend it and run another ready `minigoRoutine` on the same host goroutine, mimicking the behavior of the Go runtime.

3.  **Thread-Safe Objects**:
    *   **Environments**: The `object.Environment` would need to be made thread-safe. Any `Set` or `Assign` operation would require locking to prevent race conditions between different `minigoRoutine`s accessing the same shared scope.
    *   **Channels**: A new `object.Channel` type would be required. This would likely be a wrapper around a Go `chan object.Object`, ensuring that communication between `minigoRoutine`s is thread-safe.

This approach is extremely complex. It is not a matter of a few small changes but amounts to building a custom cooperative multitasking system within the interpreter.

## 5. Required Language Changes

To provide a useful concurrency model, the `minigo` language itself would need to be extended with new keywords and operators, mirroring Go:

*   **`go` statement**: To start a new goroutine.
*   **`chan` type**: To declare channels.
*   **`<-` operator**: For sending and receiving on channels.
*   **`select` statement**: To wait on multiple channel operations.

Each of these features would require corresponding parsers, AST nodes, and evaluation logic in the interpreter.

## 6. Conclusion & Recommendation

Adding goroutine support to `minigo` is a significant architectural project, not a simple feature addition. The current single-threaded design of the AST-walking interpreter is fundamentally incompatible with concurrent execution without a major redesign.

The work required involves implementing a scheduler, redesigning state management for thread safety, and introducing new language primitives for concurrency. This effort is substantial and goes far beyond the initial hope of simply "using the host language's goroutines."

**Recommendation:** Given the constraint that the implementation should be straightforward, **it is recommended not to proceed** with adding goroutine support to `minigo` at this time. The complexity and scope of the required changes are too high.
