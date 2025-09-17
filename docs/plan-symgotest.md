# Design Document: `symgotest` for Debugging and Visibility

This document outlines the design for a new test helper library, `symgotest`. The primary goal of this library is not just to reduce test boilerplate, but to fundamentally improve the debugging experience for `symgo`, especially in complex, cross-package scenarios.

## 1. The Core Problem: Lack of Visibility in Cross-Package Tests

The `symgo` engine is powerful, but testing its behavior across multiple packages can be challenging. A common scenario involves a test that fails because of an unexpected interaction in an *indirect dependency* (e.g., package `A` calls a function in `B`, which in turn calls a function in `C`).

When such a test fails, it is often difficult to answer the question: "What was the actual execution path that led to this failure?" The developer must manually instrument the code with logs or use a debugger, which can be cumbersome. The existing test suite solves this by manually creating call-tracking mechanisms using `RegisterDefaultIntrinsic`, but this is verbose and must be re-implemented for each test.

The core design goal of `symgotest` is to solve this visibility problem by providing an automated execution trace as a first-class feature.

## 2. The Solution: A Trace-Focused Test Runner

`symgotest` introduces a `Runner` that abstracts away test setup and, most importantly, provides a rich `RunResult` object containing a detailed execution trace.

### The `RunResult` and `FunctionsCalled` Trace

Instead of returning a simple value, the `Runner`'s execution method will return a `RunResult` struct.

**Proposed API:**
```go
type RunResult struct {
    // The final return value from the function that was applied.
    ReturnValue object.Object

    // Any runtime error that occurred during the execution.
    Error error

    // An ordered list of the fully-qualified names of all functions
    // that were symbolically executed during the run.
    FunctionsCalled []string
}
```

The `FunctionsCalled` slice is the key to visibility. It provides a simple, clear record of the execution path, making it immediately obvious which functions were (or were not) called.

### The `Runner` API

The `Runner` API is designed to be fluent and declarative.

**Proposed API:**
```go
// Runner manages a single symbolic execution test case.
type Runner struct { ... }

// NewRunner: For simple, single-package tests.
// Takes a single source string and creates a self-contained module.
func NewRunner(t *testing.T, source string) *Runner

// NewRunnerWithMultiFiles: For complex, multi-file or cross-package tests.
// Takes a map of file paths to content. A `go.mod` file is required.
// This should be the default choice for integration-style tests.
func NewRunnerWithMultiFiles(t *testing.T, files map[string]string) *Runner

// WithSetup: A flexible hook to register custom intrinsics or perform other
// advanced configuration on the interpreter before execution.
func (r *Runner) WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner

// TrackCalls: An explicit opt-in to enable the execution tracing.
// When used, the RunResult.FunctionsCalled slice will be populated.
func (r *Runner) TrackCalls() *Runner

// Apply: Executes a function and returns the complete RunResult.
func (r *Runner) Apply(funcName string, args ...object.Object) *RunResult
```

## 3. Example: Debugging an Indirect Dependency

Consider a test for a `main` package that uses a `service` package, which in turn calls a `worker` package. We want to verify that the `worker.DoWork` function is ultimately called.

**Without `symgotest`,** this would require manually setting up a default intrinsic to track calls.

**With `symgotest`,** the process is much clearer.

**Test Code:**
```go
func TestCrossPackageInteraction(t *testing.T) {
    files := map[string]string{
        "go.mod": "module example.com/app",
        "main.go": `package main
import "example.com/app/service"
func main() {
    service.Run()
}`,
        "service/service.go": `package service
import "example.com/app/worker"
func Run() {
    worker.DoWork()
}`,
        "worker/worker.go": `package worker
func DoWork() {}`,
    }

    // 1. Setup the runner for a multi-file module.
    runner := symgotest.NewRunnerWithMultiFiles(t, files)

    // 2. Opt-in to call tracking.
    runner.TrackCalls()

    // 3. Execute the 'main' function.
    result := runner.Apply("main")

    // 4. Assert on the result and, crucially, the trace.
    symgotest.AssertSuccess(t, result.ReturnValue)

    // The key assertion for debugging:
    expectedCall := "example.com/app/worker.DoWork"
    var found bool
    for _, calledFunc := range result.FunctionsCalled {
        if calledFunc == expectedCall {
            found = true
            break
        }
    }
    if !found {
        t.Errorf("expected function %q to be called, but it was not.", expectedCall)
        // For debugging, we can print the entire trace:
        // t.Logf("Execution Trace:\n%#v", result.FunctionsCalled)
    }
}
```

This example demonstrates how `symgotest` directly addresses the visibility problem. If the test fails, the developer can immediately inspect the `result.FunctionsCalled` slice to see the exact execution path and pinpoint where the chain was broken.

## 4. Secondary Features

### Standalone Helpers & Assertions

For convenience, the library will still provide helpers for simple expression evaluation and basic assertions. However, these are considered secondary to the core tracing functionality.

- `EvalExpr(t *testing.T, expr string) object.Object`: For Pattern A tests.
- `AssertSuccess`, `AssertError`, etc.: A minimal set of helpers for common checks. The user is free to use standard `if` statements or `cmp.Diff` for more complex assertions. The goal is not to replace `testify`, but to provide simple, optional conveniences.

This design places the focus squarely on improving the debugging experience for complex `symgo` tests, as requested by the user, while still providing the boilerplate reduction benefits of a test helper library.
