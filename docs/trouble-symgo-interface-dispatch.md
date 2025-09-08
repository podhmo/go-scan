# Trouble Report: Implementing Conservative Interface Dispatch

**Objective:** Implement a conservative call graph analysis for interface method calls. When a method is called on an interface variable, the call should be propagated to all known concrete types that implement the interface.

## 1. Initial State & Test Case

A test case, `TestMultiImplementationResolution`, was created to demonstrate the feature gap.

```go
// main/main.go
var G iface.MultiGreeter
func run() {
	G = impl.One{}
	G = impl.Two{}
	G.Greet() // Should trace to both One.Greet and Two.Greet
}
```

This test fails on the `main` branch because the evaluator only considers the last assigned type (`impl.Two`), not the set of all possible types.

## 2. Implementation Strategy & Attempts

The core strategy was to make the `symgo` evaluator stateful with respect to interface assignments.

### Attempt 1: Modifying `evalSelectorExpr` and `applyFunction`

This was the main and most persistent attempt. The plan was:
1.  **Enhance `object.Variable`**: Add a `map[string]struct{}` named `PossibleTypes` to store the fully-qualified names of all concrete types assigned to an interface variable.
2.  **Modify `assignIdentifier`**: When an assignment to an interface variable occurs, *add* the concrete type of the right-hand-side value to the `PossibleTypes` map. A key part of this was to ensure the static type of the variable was not overwritten by the concrete type's info.
3.  **Modify `evalSelectorExpr`**: The crucial step. The idea was to intercept the method call *before* the interface variable was resolved to its concrete value. The logic was:
    - If the expression `X` in `X.M()` is an identifier.
    - Get the corresponding `*object.Variable` from the environment *without* evaluating it.
    - Check if the variable's static type is an interface.
    - If so, return a special `*object.SymbolicPlaceholder` that contains the variable itself as the `Receiver` and the `*scanner.MethodInfo` of the interface method as the `UnderlyingMethod`.
4.  **Modify `applyFunction`**: Add a new `case` to its main `switch` to handle this new `SymbolicPlaceholder`.
    - Get the `*object.Variable` from the placeholder's `Receiver`.
    - Iterate through its `PossibleTypes` map.
    - For each concrete type, resolve it, find the corresponding method, and recursively call `applyFunction`.

#### Outcome & Problem

This approach failed repeatedly. The logs consistently showed that the special logic block in `evalSelectorExpr` was never being triggered. The execution always fell through to the `left := e.forceEval(ctx, leftObj, pkg)` line, which resolves the variable to its last concrete value.

**Root Cause Analysis:** The fundamental flaw was in my understanding of `evalSelectorExpr`'s input. The expression `leftObj := e.Eval(ctx, n.X, env, pkg)` is executed *before* my custom logic block. The `evalIdent` function, when called on the variable `G`, immediately evaluates it and returns its contained value (the `*object.Instance` for `impl.Two`), not the `*object.Variable` for `G` itself.

Therefore, `leftObj` inside `evalSelectorExpr` is never a `*object.Variable`, and the check `if receiverVar, ok := leftObj.(*object.Variable); ok` always fails.

### Attempt 2: Reverting `evalSelectorExpr` and focusing on `applyFunction`

A subsequent idea was to revert `evalSelectorExpr` and put all the logic into `applyFunction`. The idea was that when `applyFunction` receives a call for a concrete method like `(impl.Two).Greet`, it could check if the receiver `*object.Instance` came from an interface variable.

**Outcome & Problem:** This was a dead end. The `*object.Instance` does not retain a back-pointer to the variable it was assigned to. The information about the original call being on an interface variable is lost by the time `applyFunction` is called with a concrete receiver.

## 3. Current Status & Unresolved Issue

- **The `PossibleTypes` map on `object.Variable` is being populated correctly.** The change to `assignIdentifier` works as intended.
- **The core problem remains unsolved:** How to intercept an interface method call in the evaluator *before* the interface variable is resolved to a single concrete type.

The logic must exist in `evalSelectorExpr`. The final attempt, which still failed but feels closest to correct, was to check if `n.X` is an `*ast.Ident` and look up the variable without evaluating it.

```go
// In evalSelectorExpr
if ident, ok := n.X.(*ast.Ident); ok {
    if obj, ok := env.Get(ident.Name); ok {
        if receiverVar, ok := obj.(*object.Variable); ok {
            if staticType := receiverVar.TypeInfo(); staticType != nil && staticType.Kind == scanner.InterfaceKind {
                // This block is where the logic should go.
                // It should return a placeholder that applyFunction can handle.
            }
        }
    }
}
// Fallback
leftObj := e.Eval(ctx, n.X, env, pkg)
// ...
```
This seems like the correct place to intercept the call. However, implementing this correctly without causing regressions in the many other tests for `evalSelectorExpr` (e.g., for package selectors, embedded fields, etc.) has proven to be extremely difficult and has resulted in multiple panics and test failures. A clean and correct implementation of this interception is the key to solving the feature.
