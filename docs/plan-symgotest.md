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

**Proposed API:**
```go
// Runner manages a single symbolic execution test case.
type Runner struct { ... }

// NewRunner creates a runner for a given source code.
// It automatically handles making the source a valid 'main' package.
func NewRunner(t *testing.T, source string) *Runner

// WithSetup registers intrinsics or performs other configuration
// on the interpreter before execution.
func (r *Runner) WithSetup(setupFunc func(interp *symgo.Interpreter)) *Runner

// Apply executes a function from the source and returns the result.
// This is the main entry point for running a test.
func (r *Runner) Apply(funcName string, args ...object.Object) object.Object
```

**Justification & Benefits:**
- **Solves Pattern B's Boilerplate:** A single `NewRunner(t, source).Apply("main")` call will replace ~20 lines of `scantest` setup and `ActionFunc` logic.
- **Clarity and Focus:** Tests become declarative, focusing on the source code under test and the assertions, not the mechanics of the test setup.
- **Handles Multi-File Setups:** `NewRunner` can be overloaded or extended to accept a `map[string]string` for multi-file tests.
- **Flexibility:** The `WithSetup` method provides a flexible escape hatch for complex test configuration (like registering multiple intrinsics) without cluttering the main API.
- **Addresses Pattern C:** While a dedicated `EvalBlock` function could be added, the `Runner` is flexible enough to handle this. A user can find the function body within the `WithSetup` closure and perform a custom `EvalWithEnv` call if needed, keeping the primary API lean.

### Standalone Helpers

To address the simpler patterns and standardize assertions, the library will include standalone helper functions.

**Proposed API:**
```go
// EvalExpr parses and evaluates a single expression string.
func EvalExpr(t *testing.T, expr string) object.Object

// --- Assertion Helpers ---

// AssertSuccess fails if the object is an error.
func AssertSuccess(t *testing.T, obj object.Object)

// AssertError fails if the object is not an error. Can also check for substrings.
func AssertError(t *testing.T, obj object.Object, contains ...string)

// AssertInteger checks for an integer object with a specific value.
func AssertInteger(t *testing.T, obj object.Object, expected int64)

// ... other helpers for String, Nil, Placeholder, etc. ...

// AssertEqual provides a generic comparison using go-cmp.
func AssertEqual(t *testing.T, want, got any)
```

**Justification & Benefits:**
- **Solves Pattern A's Verbosity:** `EvalExpr` provides a one-line solution for simple expression tests.
- **Standardizes Assertions:** The assertion helpers provide a consistent, readable way to validate test outcomes, fulfilling the user's request to avoid `testify`. This makes test failures easier to understand and debug.

### Design Decision: State Inspection

The analysis revealed that `evaluator` tests can inspect the environment directly, a powerful but internal capability.

**Decision:** The `symgotest` library will **not** provide a public API for direct environment inspection.

**Justification:**
- **Encapsulation:** Exposing internal environment details would create a leaky abstraction and tightly couple tests to the evaluator's implementation.
- **Better Test Practices:** The library should encourage testing based on observable behavior (return values and side effects). The `Runner`'s `WithSetup` method provides a robust way to check side effects by registering intrinsics that modify variables in the test's scope. This leads to more maintainable, black-box style tests.

By implementing this design, `symgotest` will provide a powerful and ergonomic testing solution that addresses the key pain points in the current test suite.
