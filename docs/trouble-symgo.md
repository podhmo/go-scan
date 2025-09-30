# `symgo` Fails to Warn on Member Access via Embedded Pointer to Out-of-Policy Type

## 1. Problem Description

The `symgo` symbolic execution engine is designed to be resilient to incomplete information, particularly when analyzing code that depends on packages outside the defined scan policy. When a method or field is accessed on a struct that embeds a type from an out-of-policy package, `symgo` should log a `WARN` message and return a symbolic placeholder, allowing analysis to continue.

This mechanism works for directly embedded out-of-policy types but fails for structs that have a chain of embedded types where one in the middle is out-of-policy.

### Example Failure Case

Consider the following Go code structure:

```go
// In-policy package "main"
package main

import "example.com/out-of-policy/cli"

type Application struct {
	*cli.Application // Embedded pointer to an out-of-policy type
}

func NewCLI() *Application {
	a := &Application{}
	// This call should produce a warning, not a fatal error.
	a.UsageTemplate() // UsageTemplate is a method on cli.Application
	return a
}
```

When `symgo` analyzes `NewCLI`, it attempts to resolve `a.UsageTemplate()`. Because `cli.Application` is from an out-of-policy package, its definition is not available. The expected behavior is a warning.

Instead, `symgo` produces a fatal error, halting the analysis:

```
level=ERROR msg="undefined method or field: UsageTemplate for pointer type INSTANCE" in_func=NewCLI
```

## 2. Root Cause Analysis (Updated)

The initial fix was insufficient. The true root cause lies in how the recursive search functions in `symgo/evaluator/accessor.go` handle errors returned from nested recursive calls.

1.  **Incomplete Policy Check for Pointers**: The accessor failed to look "through" pointer types (`field.Type.Elem`) to get the correct import path for the policy check. This was addressed in the first attempt.

2.  **Premature Termination on Recursive Error**: This is the deeper issue. When `findMethodRecursive` or `findFieldRecursive` makes a recursive call on an embedded field, it checks the returned `err`. The previous fix correctly returned `ErrUnresolvedEmbedded` from the base case. However, the **caller** of the recursive function would see this error and immediately `return foundFn, err`, terminating its own search loop over other embedded fields.

The correct behavior is to treat `ErrUnresolvedEmbedded` as a signal to continue searching other sibling embedded types, not as a fatal error that should stop the entire lookup process for the current type.

## 3. Proposed Solution (Revised)

The `accessor` logic will be refactored to correctly handle the `ErrUnresolvedEmbedded` signal within its recursive search loops.

The `findFieldRecursive` and `findMethodRecursive` functions will be modified as follows:

1.  **Introduce a State Variable**: A boolean variable, `unresolvedEmbeddedEncountered`, will track if an out-of-policy type was seen anywhere in the search tree.

2.  **Continue on `ErrUnresolvedEmbedded`**: Inside the loop that iterates over embedded fields, the code will check the error returned from the recursive call.
    - If `err` is `ErrUnresolvedEmbedded`, the loop will **continue** to the next embedded field. It will *not* return immediately.
    - If `err` is any other non-nil error, it will be returned immediately as it represents an unexpected failure.

3.  **Deferred Error Return**: After the search loop over all embedded fields is complete, the function will check the state. If a matching member was **not** found *and* `unresolvedEmbeddedEncountered` is `true`, only then will it return `nil, ErrUnresolvedEmbedded`.

This change ensures that `symgo` performs a truly exhaustive search of all resolvable types. It correctly treats `ErrUnresolvedEmbedded` as a non-fatal condition during recursion, propagating it to the top-level caller only when no other resolution is possible. This will restore the desired "warn and continue" behavior.

---

## 4. Final Root Cause and Solution (Third Attempt)

The previous fixes to `accessor.go` were necessary pre-conditions but did not solve the problem entirely. The error persisted.

### Final Root Cause

The true root cause was a logical flaw in `symgo/evaluator/evaluator.go`, specifically within the `evalSelectorExpr` function for `*object.Instance` and `*object.Pointer` cases.

The code performed separate checks for errors from method and field lookups:

```go
// Simplified logic
method, methodErr := a.accessor.findMethodOnType(...)
field, fieldErr := a.accessor.findFieldOnType(...)

// ... other checks ...

if fieldErr == ErrUnresolvedEmbedded {
    // Log warning and return placeholder for field
}
if methodErr == ErrUnresolvedEmbedded {
    // Log warning and return placeholder for method
}

return e.newError(ctx, n.Pos(), "undefined method or field...")
```

This is incorrect. If `findMethodOnType` returns `ErrUnresolvedEmbedded` but `findFieldOnType` returns `nil, nil` (as nothing was found), the first `if` is false, the second is true (logging a warning), but execution **continues** and hits the final `newError` line, causing the fatal error.

### Correct Solution

The fix is to restructure the error handling into a mutually exclusive `if-else if` chain. This correctly handles all possibilities:

1.  **Ambiguity**: Both method and field lookups return `ErrUnresolvedEmbedded`.
2.  **Unresolved Field**: Only the field lookup returns `ErrUnresolvedEmbedded`.
3.  **Unresolved Method**: Only the method lookup returns `ErrUnresolvedEmbedded`.

The corrected logic looks like this:

```go
// ...
if methodErr == ErrUnresolvedEmbedded && fieldErr == ErrUnresolvedEmbedded {
    // Handle ambiguity
} else if fieldErr == ErrUnresolvedEmbedded {
    // Handle unresolved field
} else if methodErr == ErrUnresolvedEmbedded {
    // Handle unresolved method
}
// Only if none of the above are true do we fall through to the fatal error.
```

This change will be applied to the logic for both `*object.Instance` and `*object.Pointer` receivers within `evalSelectorExpr` to finally resolve the bug.