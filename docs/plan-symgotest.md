# Design Document: `symgotest` Test Helper Library

This document outlines the design for a new test helper library, `symgotest`, intended to simplify and standardize the process of writing tests for the `symgo` symbolic execution engine.

## Part 1: Analysis of the Existing Test Suite

A thorough review of the test files in `symgo/` and `symgo/evaluator/` reveals several recurring patterns. Understanding these patterns is crucial for designing a helper library that provides maximum benefit.

### Pattern A: Expression/Statement Evaluation

- **Description:** These are fine-grained unit tests that evaluate a single Go expression or statement in isolation. They are used to verify the evaluator's core logic for handling language primitives.
- **Example Workflow (`evaluator_test.go`):**
  1. `input := "5 + 5"`
  2. `node, _ := parser.ParseExpr(input)`
  3. `eval := evaluator.New(...)`
  4. `result := eval.Eval(node, ...)`
  5. Assert the type and value of `result`.
- **Problem:** While simple, this still involves manual AST parsing and evaluator instantiation. For a table-driven test with many expressions, this setup is repeated.

### Pattern B: Full Program Simulation

- **Description:** This is the most common and important pattern, used for feature and integration testing. It simulates the execution of a complete, self-contained Go program.
- **Example Workflow (`features_test.go`, `symgo_test.go`):**
  1. Define Go source code for one or more files in a `map[string]string`.
  2. Use `scantest.WriteFiles` to create a temporary directory.
  3. Use `scantest.Run` to manage the test, passing it an `ActionFunc`.
  4. Inside the `ActionFunc`:
     a. Create a `symgo.Interpreter`.
     b. Register mock functions using `interp.RegisterIntrinsic`.
     c. Loop through all scanned files and call `interp.Eval()` on each to populate the environment.
     d. Find an entry point function (e.g., `main`) using `interp.FindObjectInPackage`.
     e. Execute the program with `interp.Apply()`.
     f. Assert the result or side effects captured by the intrinsics.
- **Problem:** This pattern suffers from significant boilerplate. The entire `scantest` setup and the multi-step execution logic within the `ActionFunc` are repeated in nearly every test, making them verbose and hard to read.

### Pattern C: Symbolic Block Evaluation

- **Description:** This is a specialized variant of Pattern B used to test the symbolic exploration of control-flow statements (`if`, `for`, `switch`).
- **Example Workflow (`symbolic_features_test.go`):**
  1. Follows the same `scantest` setup as Pattern B.
  2. Inside the `ActionFunc`, it creates a *new, empty environment*.
  3. It finds a function's AST declaration but executes only its body (`*ast.BlockStmt`) using `interp.EvalWithEnv()`.
  4. Assertions check that all logical branches were explored by inspecting boolean flags set by intrinsics.
- **Problem:** This shares the same boilerplate issue as Pattern B. The specific workflow of evaluating a function body in a clean environment is also a candidate for abstraction.

## Part 2: Proposed `symgotest` Library Design

Based on the analysis above, the `symgotest` library will be designed to directly address the identified problems by abstracting away boilerplate and standardizing common workflows.

### Core Component: The `Runner`

The central piece of the library will be a `Runner` struct that encapsulates the entire "Full Program Simulation" (Pattern B) workflow.

#### `scantest` Dependency and `go.mod`

It is important to note that `symgotest` is built upon the existing `scantest` utility. `scantest` uses `go-scan`, which is a module-aware tool for parsing and analyzing Go source code. For `go-scan` to correctly resolve types and package paths, especially in tests involving imports, it needs to operate within a valid Go module.

Therefore, **all tests using the `symgotest.Runner` must have a `go.mod` file.** The `Runner`'s constructors (`NewRunner` and `NewRunnerWithMultiFiles`) handle this automatically by either creating a default `go.mod` or requiring one to be present in the user-provided file map. This ensures that the underlying tools function correctly without the user needing to manage the `scantest` layer directly.

#### Proposed API

```go
// Runner manages a single symbolic execution test case.
type Runner struct { ... }

// NewRunner creates a runner for a simple, single-package test from a single source string.
// Use this for tests that are self-contained in one file.
func NewRunner(t *testing.T, source string) *Runner

// NewRunnerWithMultiFiles creates a runner for a complex, multi-file or multi-package test.
// The `files` map must contain a "go.mod" entry. Use this for integration tests
// that involve cross-package calls.
func NewRunnerWithMultiFiles(t *testing.T, files map[string]string) *Runner

// WithSetup registers intrinsics or performs other configuration
// on the interpreter before execution.
func (r *Runner) WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner

// Apply executes a function from the source and returns the result.
func (r *Runner) Apply(funcName string, args ...object.Object) object.Object
```

**Justification & Benefits:**
- **Solves Pattern B's Boilerplate:** A single `NewRunner(t, source).Apply("main")` call replaces ~20 lines of `scantest` setup and `ActionFunc` logic.
- **Clarity and Focus:** Tests become declarative, focusing on the source code under test and the assertions, not the mechanics of the test setup.
- **Clear Separation of Scopes:** The two constructors, `NewRunner` and `NewRunnerWithMultiFiles`, provide a clear distinction between simple tests and more complex integration tests, guiding the user to the correct tool for their needs.
- **Flexibility:** The `WithSetup` method provides a flexible escape hatch for complex test configuration.
- **Addresses Pattern C:** While a dedicated `EvalBlock` function is not exposed to keep the API lean, this pattern can still be achieved by using `WithSetup` to perform a custom `EvalWithEnv` call on a function body found via the interpreter.

### Standalone Helpers

To address the simpler patterns and standardize assertions, the library will include standalone helper functions.

**Proposed API:**
```go
// EvalExpr parses and evaluates a single expression string.
func EvalExpr(t *testing.T, expr string) object.Object

// --- Assertion Helpers ---
func AssertSuccess(t *testing.T, obj object.Object)
func AssertError(t *testing.T, obj object.Object, contains ...string)
func AssertInteger(t *testing.T, obj object.Object, expected int64)
// ... other helpers for String, Nil, Placeholder, etc. ...
func AssertEqual(t *testing.T, want, got any)
```

**Justification & Benefits:**
- **Solves Pattern A's Verbosity:** `EvalExpr` provides a one-line solution for simple expression tests.
- **Standardizes Assertions:** The helpers provide a consistent, readable way to validate test outcomes.

### Design Decision: State Inspection

**Decision:** The `symgotest` library will **not** provide a public API for direct environment inspection.

**Justification:**
- **Encapsulation:** Exposing internal environment details would tightly couple tests to the evaluator's implementation.
- **Better Test Practices:** The library encourages testing based on observable behavior (return values and side effects via intrinsics), which leads to more maintainable, black-box style tests.

## Part 3: How `symgotest` Improves the Debugging Experience

Beyond making tests easier to write, `symgotest` also makes them easier to debug.

1.  **Isolation of Failures:** By abstracting the complex `scantest` and `go-scan` setup, a test failure is much less likely to be caused by an error in the test's setup boilerplate. Failures will be more clearly isolated to either the source code being tested or the test's core logic (`WithSetup` and assertions), reducing the surface area a developer needs to inspect.

2.  **Clear and Consistent Error Messages:** The suite of `Assert` helpers ensures that failure messages are uniform and descriptive. An integer mismatch will always produce a message like `integer has wrong value. want=X, got=Y`, and a failed error check will always say `expected an error, but got <type>`. This consistency makes it faster to understand the nature of a failure at a glance, compared to parsing the output of `cmp.Diff` or a generic `fmt.Errorf` message for every test.

3.  **Readable Tests:** A debugger's first step is often to read the failing test to understand what it's trying to accomplish. The concise, declarative nature of tests written with `symgotest` makes this process much faster. The separation of source, setup, and execution is explicit, allowing a developer to quickly identify the relevant parts of the test.

By implementing this design, `symgotest` will provide a powerful and ergonomic testing solution that addresses the key pain points in the current test suite and improves the overall development and debugging workflow.
