# `symgo` Fails to Warn on Member Access via Embedded Pointer to Out-of-Policy Type

## 1. Problem Description

The `symgo` symbolic execution engine is designed to be resilient to incomplete information, particularly when analyzing code that depends on packages outside the defined scan policy. When a method or field is accessed on a struct that embeds a type from an out-of-policy package, `symgo` should log a `WARN` message and return a symbolic placeholder, allowing analysis to continue.

This mechanism works correctly for directly embedded types but fails for **embedded pointers** to out-of-policy types.

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

When `symgo` analyzes `NewCLI`, it attempts to resolve `a.UsageTemplate()`. Because `cli.Application` is from an out-of-policy package (`example.com/out-of-policy/cli`), its definition is not available. The expected behavior is a warning.

Instead, `symgo` produces a fatal error, halting the analysis:

```
level=ERROR msg="undefined method or field: UsageTemplate for pointer type INSTANCE" in_func=NewCLI
```

## 2. Root Cause Analysis

The root cause of this bug lies in the `accessor` component (`symgo/evaluator/accessor.go`), which is responsible for resolving method and field lookups. The current implementation has two key flaws:

1.  **Incomplete Policy Check for Pointers**: When checking an embedded field, the accessor inspects `field.Type.FullImportPath`. However, if the embedded field is a pointer (e.g., `*cli.Application`), the `FullImportPath` on the pointer type itself is often empty. The accessor fails to look "through" the pointer to the element type (`field.Type.Elem`) to get the correct import path for the policy check.

2.  **Premature Termination**: The accessor's recursive search for a member uses a "fail-fast" approach. The moment it encounters an out-of-policy embedded type, it immediately returns `ErrUnresolvedEmbedded`. If a struct embeds multiple types, and the out-of-policy one is checked first, the search terminates before checking other, potentially valid, in-policy embedded types. The correct behavior is to exhaust all in-policy options before concluding that the member might exist on an unresolved type.

Because of these issues, the accessor returns `nil, nil` (member not found, no error) instead of the expected `nil, ErrUnresolvedEmbedded`. The `evaluator` then receives this result and correctly, but undesirably, concludes that the member is truly undefined, leading to the fatal error.

## 3. Proposed Solution

The `accessor` logic will be refactored to be more robust and exhaustive. The "fail-fast" logic will be replaced with a "search-and-record" strategy.

The `findFieldRecursive` and `findMethodRecursive` functions in `symgo/evaluator/accessor.go` will be modified as follows:

1.  **Introduce a State Variable**: A new boolean variable, `unresolvedEmbeddedEncountered`, will be added to the recursive search functions. This flag will be set to `true` whenever the search encounters an embedded type that is out of policy.

2.  **Enhance Policy Check**: The policy check will be updated to correctly handle pointers. It will check `field.Type.FullImportPath` for non-pointers and `field.Type.Elem.FullImportPath` for pointers.

3.  **Exhaustive Search**: The search will no longer return immediately upon finding an out-of-policy type. Instead, it will:
    a.  Set `unresolvedEmbeddedEncountered = true`.
    b.  Skip the recursive search on that specific out-of-policy type.
    c.  Continue searching through the remaining embedded fields.

4.  **Deferred Error Return**: After the search loop over all embedded fields is complete, the function will check the state variable. If a matching member was **not** found during the entire search, *and* `unresolvedEmbeddedEncountered` is `true`, only then will it return `nil, ErrUnresolvedEmbedded`.

This change ensures that `symgo` performs a complete search of all resolvable types first. It correctly signals the "unresolved" condition to the evaluator only when the member is not found in any in-policy type but could potentially exist on an out-of-policy one. This will restore the desired "warn and continue" behavior for these cases.