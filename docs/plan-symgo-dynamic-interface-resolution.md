# Plan: Dynamic and Conservative Interface Resolution in `symgo`

## 1. Motivation

The `symgo` symbolic execution engine is designed to build a comprehensive call graph of a Go codebase. A critical part of this is correctly tracing method calls made on interface types. The current implementation has two key limitations that prevent this in common, real-world scenarios:

1.  **Lack of Conservative Analysis**: When a variable with an interface type is assigned different concrete types over its lifetime (e.g., in different branches of an `if` statement), the engine only tracks the most recently assigned type. A conservative static analysis requires that a subsequent method call on that interface variable be traced to *all possible* concrete types it could hold.

2.  **Brittle `Implements` Check**: The existing `goscan.Implements` utility is bound to the context of a single package. It cannot resolve interface implementations that span package boundaries (e.g., interface in package A, implementation in package B), which is a very common pattern.

This plan outlines the necessary changes to the `symgo` evaluator to address these issues, enabling robust, order-independent, and conservative analysis of interface method calls.

## 2. Task List & Progress

- [x] Create a robust test case (`TestMultiImplementationResolution`) that fails on the current `main` branch and correctly demonstrates the lack of conservative analysis.
- [x] Add a `PossibleTypes map[string]struct{}` field to `symgo/object/object.go`'s `Variable` struct to track all concrete types assigned to an interface variable.
- [ ] **TODO**: Modify `symgo/evaluator/evaluator.go:assignIdentifier` to correctly populate the `PossibleTypes` map upon assignment to an interface-type variable, ensuring the variable's static type information is preserved.
- [ ] **TODO**: Modify `symgo/evaluator/evaluator.go` to correctly dispatch a method call on an interface variable to all concrete types listed in its `PossibleTypes` map. The current blocker and debugging attempts are detailed in `docs/trouble-symgo-interface-dispatch.md`.
- [ ] **TODO**: Ensure all existing tests in `./symgo/evaluator/...` pass after the changes are implemented, fixing any regressions.
- [ ] **TODO**: Update `docs/analysis-symgo-implementation.md` to reflect the final, working design.
- [ ] **TODO**: Update `TODO.md` to close this task.

## 3. Proposed Changes

The core idea is to shift from tracking a single concrete type to tracking a *set* of possible concrete types for each interface variable, and to apply method calls to all types in that set.

### 3.1. `object.Variable` Enhancement

The `object.Variable` struct in `symgo/object/object.go` will be the primary store for this new information.

```go
// In symgo/object/object.go
type Variable struct {
	// ... existing fields: Name, Value, IsEvaluated, etc. ...

	// NEW: A set to store all possible concrete types that have been assigned to this variable.
	// The key is the fully qualified type name (e.g., "app/impl.S").
	// This will only be populated for variables whose static type is an interface.
	PossibleTypes map[string]struct{}
}
```

This `PossibleTypes` map will act as the memory for the variable, accumulating all concrete types it has been assigned.

### 3.2. `evaluator.assignIdentifier` Modification

The logic for handling assignments in `symgo/evaluator/evaluator.go` will be updated.

When an assignment `v = x` occurs, `assignIdentifier` will perform the following steps:

1.  **Check if `v` is an interface variable**: Resolve the static type of the variable `v`. If it is an interface, proceed.
2.  **Do not overwrite**: Instead of just overwriting `v.Value`, the new logic will be additive.
3.  **Add to `PossibleTypes`**: Get the concrete type of the right-hand side `x`. Add its fully qualified name to the `v.PossibleTypes` set.
4.  **Update `Value`**: The `v.Value` will still be updated to `x`. This preserves the existing behavior for tracking the *last* known value, which can be useful for other analyses, but it will no longer be the sole source of truth for method dispatch.

```go
// In symgo/evaluator/evaluator.go, inside assignIdentifier
// ...
v, ok := obj.(*object.Variable)
// ...

// NEW LOGIC
// Preserve the static type of the variable, don't overwrite it with the concrete type.
if v.TypeInfo() == nil {
    v.SetTypeInfo(val.TypeInfo())
}

staticType := v.TypeInfo()
if staticType != nil && staticType.Kind == scanner.InterfaceKind {
    if v.PossibleTypes == nil {
        v.PossibleTypes = make(map[string]struct{})
    }
    if concreteType := val.TypeInfo(); concreteType != nil {
        key := concreteType.PkgPath + "." + concreteType.Name
        v.PossibleTypes[key] = struct{}{}
    }
}

v.Value = val
// ...
```

### 3.3. `evaluator.evalSelectorExpr` & `evaluator.applyFunction` Modification

This is the most critical and currently blocked part of the implementation. The goal is to ensure that when `G.Greet()` is called, the evaluator acts on the `PossibleTypes` of `G`, not just its last assigned value.

The most promising approach is:
1.  **In `evalSelectorExpr`**: Before evaluating the receiver `X`, check if it is an identifier that refers to a `*object.Variable` with an interface type. If so, do not evaluate it further. Instead, find the method on the interface definition and return a `*object.SymbolicPlaceholder` containing the *unevaluated variable* as the `Receiver` and the `*scanner.MethodInfo` as the `UnderlyingMethod`.
2.  **In `applyFunction`**: Add a `case *object.SymbolicPlaceholder:`. If the placeholder's `Reason` is "interface method call", get the `Receiver` (which is the `*object.Variable`), iterate through its `PossibleTypes` map, and for each concrete type, find and recursively call the corresponding concrete method.

**Problem:** This logic is proving difficult to implement correctly without causing regressions. The state of the `Variable` object is not being correctly identified in `evalSelectorExpr`. The full debugging log for this is being created in `docs/trouble-symgo-interface-dispatch.md`.

## 4. Example Walkthrough (`TestMultiImplementationResolution`)

1.  **`G = impl.One{}`**: `assignIdentifier` is called for variable `G`.
    - It sees `G` is an `iface.MultiGreeter`.
    - It adds `"app/impl.One"` to `G.PossibleTypes`. `G.PossibleTypes` is now `{"app/impl.One"}`.
2.  **`G = impl.Two{}`**: `assignIdentifier` is called again for `G`.
    - It sees `G` is an `iface.MultiGreeter`.
    - It adds `"app/impl.Two"` to `G.PossibleTypes`. `G.PossibleTypes` is now `{"app/impl.One", "app/impl.Two"}`.
3.  **`G.Greet()`**: `applyFunction` is called on a placeholder representing the interface method call.
    - The receiver is the variable `G`. The engine sees its `PossibleTypes`.
    - It iterates through the set:
        - **For "app/impl.One"**: It finds `(impl.One).Greet` and calls `applyFunction` on it. The intrinsic for `(impl.One).Greet` fires, setting `oneGreetCalled = true`.
        - **For "app/impl.Two"**: It finds `(impl.Two).Greet` and calls `applyFunction` on it. The intrinsic for `(impl.Two).Greet` fires, setting `twoGreetCalled = true`.
4.  **Result**: Both boolean flags are true, and the test passes.
