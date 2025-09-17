# Plan: `symgotest` - A Debugging-First Testing Library for `symgo`

This document outlines a plan for `symgotest`, a new testing library designed specifically for the `symgo` symbolic execution engine. The goal is to dramatically improve the testing experience by reducing boilerplate and, most critically, providing powerful, built-in debugging capabilities.

## 1. Core Philosophy & Goals

Testing a symbolic execution engine presents unique challenges. Tests often require significant setup, and failures—especially hangs, infinite recursions, or out-of-memory errors—can be extraordinarily difficult to debug. The current testing methodology for `symgo` relies on verbose boilerplate and manual state inspection, which exacerbates these issues.

`symgotest` is designed to solve these problems by adhering to a **debugging-first** philosophy.

Our primary goal is **not** to create another assertion helper library. The value of a testing framework for a tool as complex as `symgo` is not in syntactic sugar for assertions, but in its ability to provide **clarity and insight when a test fails**.

The goals of `symgotest` are:

1.  **Drastically Reduce Boilerplate:** Abstract away the repetitive setup of scanners, interpreters, and file systems, allowing developers to focus purely on the test logic.
2.  **Make Debugging Effortless:** Automatically capture execution traces. When a test fails, hangs, or times out, `symgotest` will provide a clear, concise report showing *exactly* what the engine was doing, pinpointing the source of the failure.
3.  **Eliminate Hangs and Infinite Loops:** Introduce configurable execution step limits to proactively catch runaway analysis, turning indefinite hangs into immediate, deterministic test failures with actionable stack traces.
4.  **Provide Expressive, High-Level APIs:** Offer intuitive functions for common testing scenarios, from evaluating a single expression to analyzing complex, multi-package interactions.

This library is not a replacement for `testing.T` or a collection of `assert.Equal` style functions. It is a powerful test runner that understands `symgo`'s execution model and is designed from the ground up to make the entire testing lifecycle—writing, running, and especially debugging—faster and more effective.

## 2. Core API Design

The core of `symgotest` is a single, powerful runner function: `symgotest.Run`. This function handles all the boilerplate of setting up a test environment, running the `symgo` interpreter, and providing a rich result object for assertions and debugging.

### The `symgotest.Run` Function

The main entry point for all tests. It sets up the environment, runs the symbolic execution, and provides the results to a user-defined action function.

```go
package symgotest

// Run executes a symgo test case. It handles all setup and teardown.
// If the execution fails due to an error, timeout, or exceeded step limit,
// it will call t.Fatal with a detailed report, including an execution trace.
func Run(t *testing.T, tc TestCase, action func(t *testing.T, r *Result))

// TestCase defines the inputs for a single symgo test.
type TestCase struct {
	// Source provides the file contents for the test, mapping filename to content.
	// A `go.mod` file is typically required.
	Source map[string]string

	// EntryPoint is the fully qualified name of the function to execute.
	// e.g., "example.com/me/main.main"
	EntryPoint string

	// Args are the symbolic objects to pass as arguments to the EntryPoint function.
	Args []object.Object

	// Options allow for customizing the test run's behavior.
	Options []Option
}

// Result contains the outcome of the symbolic execution.
type Result struct {
	// ReturnValue is the object returned from the EntryPoint function.
	ReturnValue object.Object

	// FinalEnv is the environment state after the EntryPoint function has completed.
	// This can be used to inspect the values of variables.
	FinalEnv *object.Environment

	// Trace is the detailed execution trace, useful for debugging.
	// (See "Debugging Features" section for details).
	Trace *ExecutionTrace

	// Error is any runtime error returned by the interpreter during execution.
	Error *object.Error

    // Interpreter provides access to the configured interpreter for advanced assertions.
    Interpreter *symgo.Interpreter
}
```

### Configuration with Options

The behavior of `symgotest.Run` can be customized using functional options.

```go
package symgotest

type Option func(*config)

// WithMaxSteps sets a limit on the number of evaluation steps to prevent
// infinite loops. If the limit is exceeded, the test fails.
// Default: 10,000
func WithMaxSteps(limit int) Option

// WithTimeout sets a time limit for the entire test run.
// Default: 5 seconds
func WithTimeout(d time.Duration) Option

// WithScanPolicy defines which packages are "in-policy" (evaluated recursively)
// versus "out-of-policy" (treated as symbolic placeholders).
func WithScanPolicy(policy symgo.ScanPolicyFunc) Option

// WithIntrinsic registers a custom handler for a specific function call,
// allowing for mocking or spying. This is a cleaner alternative to
// registering intrinsics on the interpreter manually.
func WithIntrinsic(name string, handler symgo.IntrinsicFunc) Option
```

## 3. Testing Scenarios

`symgotest` provides helpers tailored to different testing granularities, from a single expression to complex, multi-package applications.

### Expression-Level Testing

For quickly testing the symbolic evaluation of a single Go expression.

**Concept:** A helper function, `symgotest.RunExpression`, wraps the expression in a temporary `main` function, runs the interpreter, and returns the result.

```go
// RunExpression is a convenience wrapper around Run for testing a single expression.
func RunExpression(t *testing.T, expr string, action func(t *testing.T, r *Result))
```

### Statement-Level Testing

For testing one or more statements, focusing on their side effects on the environment.

**Concept:** A helper `symgotest.RunStatements` wraps the statements in a `main` function. The `action` function then inspects the `FinalEnv` to check for changes.

```go
// RunStatements is a convenience wrapper for testing a block of statements.
func RunStatements(t *testing.T, stmts string, action func(t *testing.T, r *Result))
```

### Single-Package Testing

This is the most common use case, for testing functions within a single Go package. It uses the standard `symgotest.Run` function.

**Example:**
```go
func TestSinglePackage_FunctionCall(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `
package main
type User struct { Name string }
func NewUser(name string) *User {
	return &User{Name: name}
}
`,
		},
		EntryPoint: "example.com/me.NewUser",
		Args:       []object.Object{object.NewString("Alice")},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}
		// ... assertions on r.ReturnValue
	}

	symgotest.Run(t, tc, action)
}
```

### Cross-Package and Multi-Module Testing

`symgotest` simplifies testing interactions between multiple packages, even across different Go modules. The `Source` map can be used to construct a complete virtual workspace.

**Example:**
```go
func TestCrossPackage_Import(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/app",
			"main.go": `
package main
import "example.com/app/helper"
func main() string {
	return helper.Greet("World")
}
`,
			"helper/helper.go": `
package helper
import "fmt"
func Greet(name string) string {
	return fmt.Sprintf("Hello, %s", name)
}
`,
		},
		EntryPoint: "example.com/app.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		_, ok := r.ReturnValue.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("Expected a symbolic placeholder, got %T", r.ReturnValue)
		}
	}

	symgotest.Run(t, tc, action)
}
```

## 4. Debugging-First Features

The primary value of `symgotest` comes from its built-in features designed to make debugging effortless. These tools are enabled by default and provide clear, actionable insights when tests fail.

### The Execution Tracer

When a test fails, the most important question is: "What was the engine doing right before it failed?" The Execution Tracer answers this question automatically.

**Concept:** `symgotest` records a chronological log of every significant evaluation step. If the test fails for any reason (error, timeout, step limit), a formatted summary of the last 50 steps is printed, showing the exact sequence of events that led to the failure.

> **Implementation Status (as of 2025-09-16):** A basic version of the tracer has been implemented. It captures the step number, source code position, and the AST node being evaluated. The more detailed event logging shown in the example below (e.g., distinguishing between `CALL`, `EVAL`, `ASSIGN`, `GET`) is a future enhancement and is not yet implemented.

**Failure Report Example:**
```
--- FAIL: TestMyFailingFeature (0.01s)
    symgotest: test failed: identifier not found: y

    Execution Trace (last 10 of 123 steps):
    ...
    [Step 114] CALL   -> myFunc at main.go:5:1
    [Step 115] ENTER  -> myFunc
    [Step 116] EVAL   -> decl x := 10 at main.go:6:2
    [Step 117] ASSIGN -> var x = 10
    [Step 118] EVAL   -> decl z := x + y at main.go:7:2  <-- FAILURE
    [Step 119] EVAL   -> binary_expr x + y at main.go:7:10
    [Step 120] GET    -> var x (value: 10)
    [Step 121] GET    -> var y (value: <not found>)       <-- ROOT CAUSE
    [Step 122] ERROR  -> identifier not found: y
    [Step 123] EXIT   -> myFunc with error
```

### Deterministic Timeout and Hang Prevention

Hangs from infinite recursion or complex loops are a common and frustrating problem when testing analysis tools. `symgotest` solves this by turning non-deterministic hangs into deterministic failures.

**Concept:** `symgotest` enforces a limit on the number of evaluation steps (e.g., 10,000 by default). If this limit is exceeded, the test fails immediately with a clear error message.

**Failure Report Example:**
```
--- FAIL: TestInfiniteRecursion (0.02s)
    symgotest: test failed: max execution steps (10000) exceeded.

    Execution Trace (last 10 of 10000 steps):
    ...
    [Step 9991] CALL   -> recursiveFunc at main.go:4:1
    ...
    [Step 9994] CALL   -> recursiveFunc at main.go:6:3
    ...
    [Step 9997] CALL   -> recursiveFunc at main.go:6:3
    ...
    [Step 10000] CALL  -> recursiveFunc at main.go:6:3
```
The trace immediately reveals the repeating pattern, making the cause of the "hang" obvious.

### Proposed Changes to `symgo` Engine

To enable these features, a minor, non-intrusive change to the `symgo` interpreter is required.

1.  **Introduce a Step Counter:** The core evaluation loop in `symgo.Interpreter.Eval` will be modified to accept an execution context object.
2.  **Increment and Check:** On each node evaluation, the interpreter will increment a counter within this context and check if it has exceeded the `maxSteps` limit.
3.  **Return Error on Exceeding Limit:** If the limit is reached, `Eval` will immediately stop and return a specific `*object.Error`.

## 5. Advanced Use Cases

`symgotest` is designed to handle the full range of `symgo`'s analysis capabilities.

### Testing Policy-Based Evaluation

Use the `symgotest.WithScanPolicy` option to define which packages are deeply analyzed versus those that are not, verifying that the interpreter behaves correctly at these boundaries.

**Example:**
```go
func TestPolicyBoundaries(t *testing.T) {
	tc := symgotest.TestCase{
		// ...
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(path string) bool {
				return path == "example.com/app/main" // Only main is in-policy
			}),
		},
	}
	// ...
	action := func(t *testing.T, r *symgotest.Result) {
		// Verify that a call to an out-of-policy function
		// returned a placeholder.
		_, ok := r.ReturnValue.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("Expected a SymbolicPlaceholder, got %T", r.ReturnValue)
		}
	}
	symgotest.Run(t, tc, action)
}
```

### Inspecting Symbol Declaration Scopes

The `symgotest.Result` struct contains the `Interpreter` instance, providing a hook for advanced assertions that need to inspect its internal state, such as its package cache.

**Example:**
```go
func TestSymbolLoading(t *testing.T) {
    // ...
	action := func(t *testing.T, r *symgotest.Result) {
		// Use the interpreter from the result to inspect loaded symbols.
		userType, ok := r.Interpreter.FindObjectInPackage(t.Context(), "example.com/app/models", "User")
		if !ok {
			t.Fatal("Type 'User' was not found in the 'models' package scope")
		}
	}
	symgotest.Run(t, tc, action)
}
```

## 6. Virtual Debugging Scenario: Solving the `minigo` Timeout

This scenario demonstrates how `symgotest` transforms the debugging of a complex, real-world failure.

### The Problem: A Timeout and an Ocean of Logs

A regression causes the `minigo_analysis_test` to hang. The test fails after 30 seconds with a generic `context deadline exceeded` error. The only recourse is to enable verbose logging, producing a massive, unreadable file that is difficult to parse for recursive patterns.

### The `symgotest` Solution: Instant, Actionable Failure

The same test is rewritten using `symgotest`. When the regression is introduced, the test **fails in under a second**.

The error message is not a timeout. It's a clear, deterministic error:
`symgotest: test failed: max execution steps (10000) exceeded.`

Below this error, `symgotest` automatically prints the execution trace, which immediately reveals the runaway loop, including the source code locations and call stack depth. What was a multi-hour debugging nightmare becomes a one-minute fix. This is the core value proposition of `symgotest`: it transforms debugging from a manual, painful process into an automated, insightful one.

## 7. Implementation Notes & Lessons Learned

During the initial refactoring of `symgo` tests to use `symgotest`, the following points were discovered:

*   **`RunExpression` Limitations**: The `RunExpression` convenience function is designed for simple, self-contained expressions that do not require external imports. It automatically wraps the expression in a `main` package, which does not include other files or import declarations. For tests that rely on specific package structures or imports (e.g., importing `fmt`), the main `symgotest.Run` function with a full `TestCase` struct must be used instead. The expression to be tested should be wrapped in a helper function which is then used as the `EntryPoint`.

*   **`WithIntrinsic` Bug**: A bug was discovered where the `WithIntrinsic` option was not being applied. The options were processed, but the collected intrinsic handlers were never registered with the `symgo.Interpreter` instance. This has been fixed by adding the necessary registration logic within the `runLogic` function in `symgotest.go`.

*   **Multi-Module Workspace Support**: The initial implementation of `symgotest.Run` did not correctly support multi-module workspaces, as it always set the scanner's working directory to the root of the virtual file system. This was fixed by adding a `WorkDir` field to `symgotest.TestCase`, allowing tests to specify the correct subdirectory for the main module.

*   **`WithScanPolicy` Bug**: A bug similar to the `WithIntrinsic` issue was found where `WithScanPolicy` was not being applied correctly. This was because `symgotest.Run` was unconditionally adding a `WithPrimaryAnalysisScope` option for the entry point's package, which conflicted with or overrode the custom policy. The logic has been updated to prioritize `WithScanPolicy` when it is provided, falling back to `WithPrimaryAnalysisScope` only when a custom policy is not set.

*   **Default Intrinsics**: A test case involving anonymous interfaces required capturing calls to unknown methods. This highlighted the need for a "catch-all" or default intrinsic handler. The `WithDefaultIntrinsic` option was added to `symgotest` to support this, allowing tests to provide a function that is executed for any call that doesn't have a specific intrinsic registered.

*   **Default Scope Behavior**: It was confirmed that the default analysis scope for a `symgotest` run (when no `ScanPolicy` is provided) is limited to the package of the `EntryPoint` function. This is a sensible default but means that tests involving intra-module calls (e.g., `main` calling `helper`) require an explicit `ScanPolicy` to include all necessary packages in the analysis.

*   **Inspecting Global Variables**: The `Result.FinalEnv` field contains the environment of the function *after* it has executed. This is useful for inspecting local variables, but it does not contain package-level (global) variables. To inspect the state of global variables after a test run, use the interpreter instance attached to the result: `r.Interpreter.FindObjectInPackage(ctx, "pkg/path", "varName")`.

*   **Return Value from `main`**: Be cautious when making assertions about the return value of an entry point that is a `main` function (a function with no return value). The interpreter may not return the `object.NIL` singleton in this case. In some tests, it was observed to return an `*object.SymbolicPlaceholder`. For robustness, it's best to avoid asserting on the specific return value of such functions unless the test is specifically designed to investigate that behavior.

*   **Scan-Order Dependency**: `symgotest.Run` abstracts away the details of package scanning, typically by scanning the entire module (`./...`). This is a valuable simplification, but it makes the library unsuitable for tests that need to verify that analysis is independent of package scanning order. Refactoring such tests would cause the loss of the core validation logic.

*   **Return Value Unwrapping**: The `symgotest.Result.ReturnValue` field contains the actual, unwrapped `object.Object` returned by the symbolic execution of the entry point function. It does *not* contain the `*object.ReturnValue` wrapper that the interpreter uses internally. Assertions should be written to expect the underlying object (e.g., `*object.String`, `*object.Integer`, etc.).

*   **Testing for Expected Errors**: The `symgotest.Run` function is designed for tests that are expected to complete successfully. If the underlying `symgo` interpreter returns an error, `symgotest` treats this as a fatal test failure and halts execution immediately. This makes it unsuitable for tests that need to verify the content of an expected error (e.g., testing that a specific runtime error occurs and includes a correct stack trace). For such cases, the lower-level `scantest` library should be used instead, as it provides the necessary control to manually invoke the interpreter and inspect the returned error object without causing a test failure.

*   **Tracer Integration**: The `symgotest` library does not currently offer a `WithTracer` option. This means that tests requiring inspection of the internal execution trace (by providing a custom `symgo.Tracer` implementation) cannot be refactored to use `symgotest`. They must continue to use the lower-level `scantest` library, which allows for manual creation of the `symgo.Interpreter` with a tracer attached.
