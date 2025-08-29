# Analysis of `symgo` Warnings in `find-orphans`

This document analyzes a series of `WARN` logs observed during the symbolic execution of Go code by the `symgo` engine, likely triggered via the `find-orphans` tool.

## Observed Logs

```
level=WARN msg="infinite recursion detected, aborting" function=flattenComposite
level=WARN msg="error evaluating statement in type switch case" in_func=ServeError error="identifier not found: rw"
level=WARN msg="error evaluating statement in type switch case" in_func=rw.Header error="identifier not found: rw"
level=WARN msg="error evaluating statement in type switch case" in_func=rw.WriteHeader error="identifier not found: rw"
```

## 1. `infinite recursion detected`

### Cause

This warning indicates that the `symgo` symbolic execution engine has detected a function being called with the exact same arguments multiple times in the same call stack. This is a deliberate safety feature to prevent the analyzer itself from entering an infinite loop and crashing.

The provided example function, `flattenComposite`, is a recursive function.

```go
func flattenComposite(errs *errors.CompositeError) *errors.CompositeError {
	var res []error
	for _, er := range errs.Errors {
		switch e := er.(type) {
		case *errors.CompositeError:
			if len(e.Errors) > 0 {
				flat := flattenComposite(e) // Recursive call
				// ...
			}
		// ...
		}
	}
	return errors.CompositeValidationError(res...)
}
```

This warning will be triggered if `symgo` analyzes a call to `flattenComposite` where the input `*errors.CompositeError` has a cyclic reference. For example:

```go
// Cyclic error structure
err1 := &errors.CompositeError{}
err2 := &errors.CompositeError{Errors: []error{err1}}
err1.Errors = []error{err2}

// Analyzing this call would trigger the warning
flattenComposite(err1)
```

### Resolution

This is not a bug in `symgo` but rather a feature that highlights a potential issue or a complex pattern in the code being analyzed. The engine correctly aborts the analysis of that specific recursive path and continues with the rest of the program. There is no fix required for the engine itself.

## 2. `identifier not found: rw`

### Cause

This warning pointed to a critical bug in the `symgo` evaluator's handling of function scope.

The `extendFunctionEnv` function in `symgo/evaluator/evaluator.go` is responsible for setting up the environment for a function call by binding the call arguments to the parameter names. It was incorrectly using `env.Set(name, val)` instead of `env.SetLocal(name, val)`.

- `env.Set`: This method was implemented to mimic assignment (`=`). It searches up the scope chain for an existing variable to update. If it doesn't find one, it creates the variable in the *outermost* (package) scope it can reach.
- `env.SetLocal`: This method correctly creates a new variable in the *current* scope, which is the correct behavior for declaring function parameters (`:=` semantics).

Because `env.Set` was used, function parameters (like `rw` in the `ServeError` example) were not being created in the function's own local environment. Instead, they were being created in the global package-level environment.

When the evaluator later encountered a nested block (like a `case` within a `type switch`), it would correctly create a new nested environment. However, when it tried to look up the `rw` identifier from within this nested block, the lookup chain was broken because the function's own environment was empty.

### Resolution

The bug was fixed by changing the call in `extendFunctionEnv` from `env.Set(name.Name, v)` to `env.SetLocal(name.Name, v)`. This ensures that function parameters are always created in the correct local scope for the function call, making them accessible to all nested blocks as expected. A regression test was added to verify this fix.
