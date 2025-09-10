# Troubleshooting `symgo` Evaluator Panic

This document outlines the investigation and resolution of a `panic: runtime error: integer divide by zero` that occurred when running the `find-orphans` tool.

## 1. Problem Description

When running `make -C examples/find-orphans`, the process would fail with a long log of warnings and errors, culminating in a panic.

- **Command:** `go run ./examples/find-orphans --workspace-root ../.. ./...`
- **Panic:** `panic: runtime error: integer divide by zero`
- **Location:** `github.com/podhmo/go-scan/symgo/evaluator.(*Evaluator).evalIntegerInfixExpression` at `evaluator.go:675`

The stack trace indicated the panic occurred during a division operation (`token.QUO`) within the symbolic evaluator.

## 2. Log Analysis

The logs preceding the panic showed several important patterns:

1.  **`level=ERROR msg="identifier not found: isError"`**: These errors occurred frequently while `symgo` was analyzing the source code of the `minigo` evaluator (`minigo/evaluator/evaluator.go`). `isError` is a local helper function within that package. The failure of `symgo` to resolve this package-level function indicated a fundamental problem in its symbol resolution or environment setup.

2.  **`level=WARN msg="bounded recursion depth exceeded"`**: These warnings, along with a very deep call stack in the final panic, suggested that the evaluator was getting lost in a complex, recursive analysis path, likely triggered by the initial symbol resolution failures.

3.  **`level=WARN msg="expected multi-return value on RHS of assignment"`**: These warnings are another symptom of the analysis engine being in an inconsistent state, where it cannot correctly model the results of function calls.

The combination of these logs suggested that the failure to find `isError` led to `symgo` propagating an error object. Subsequent code in `symgo` likely did not handle this error object correctly, leading to a state where a variable was assigned a zero value. When this variable was later used in a division operation, the panic occurred.

## 3. Root Cause Analysis

There are two distinct issues at play:

1.  **Immediate Cause (The Panic):** The `evalIntegerInfixExpression` function in `symgo/evaluator/evaluator.go` performed a division (`/`) without checking if the denominator was zero. While `symgo` is a symbolic execution engine, it can and does perform concrete arithmetic on constant values. When it encountered a literal division by zero during its evaluation, it panicked. This is a robustness issue in the evaluator.

2.  **Underlying Cause (Symbol Resolution Failure):** The more fundamental problem is `symgo`'s inability to resolve the package-level `isError` helper function when analyzing the `minigo/evaluator` package. A symbolic evaluator analyzing a package should have access to all symbols within that package's scope. This failure indicates a bug in how `symgo` constructs and manages its evaluation environments. This bug is the likely source of the malformed state that eventually led to the division by zero.

## 4. Solution

The immediate panic was addressed by making the `symgo` evaluator more robust.

**File:** `symgo/evaluator/evaluator.go`

**Change:** Modified the `evalIntegerInfixExpression` function to handle division by zero gracefully.

```go
// ... inside evalIntegerInfixExpression
	case token.QUO:
		if rightVal == 0 {
			// Instead of panicking, return a symbolic placeholder.
			// This allows analysis to continue on other paths.
			return &object.SymbolicPlaceholder{Reason: "result of division by zero"}
		}
		return &object.Integer{Value: leftVal / rightVal}
// ...
```

This change prevents the crash by returning a `SymbolicPlaceholder` when a division by zero is attempted. This allows the analysis to continue without halting, which is the correct behavior for a static analysis tool that may encounter invalid operations in the code it is analyzing.

While this resolves the panic, the underlying issue with package-level symbol resolution (`isError` not found) remains and should be investigated separately to improve the overall correctness and stability of the `symgo` engine.
