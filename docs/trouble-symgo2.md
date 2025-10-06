# Trouble Report: `symgo` Evaluator Failures on Method Chaining and Named Slice Methods

This document details two related bugs in the `symgo` evaluator that cause symbolic execution to halt prematurely. These issues were discovered during the analysis of a large, real-world Go codebase.

## 1. Summary of Problems

The `symgo` evaluator currently fails when encountering two common Go programming patterns:

1.  **Fluent API / Method Chaining**: When a method call returns the receiver to allow for chained calls (e.g., `builder.Name("x").Version("v1")`), the evaluator fails on the second call.
2.  **Methods on Named Slice Types**: When a method is called on a variable whose type is a named slice (e.g., `type MySlice []int; ...; mySlice.MyMethod()`), the evaluator fails.

In both cases, the symbolic execution stops, preventing a complete analysis of the call graph.

## 2. Analysis of Failures

Both bugs stem from shortcomings in `symgo/evaluator/evaluator_eval_selector_expr.go`.

### 2.1. Fluent API / Method Chaining Failure

-   **Symptom**: The log shows the error `expected a package, instance, or pointer on the left side of selector, but got FUNCTION`.
-   **Example Code**:
    ```go
    // library code used by kingpin
    app := kingpin.New("my-app", "A command-line app.")
    app.Flag("debug", "Enable debug mode.").Bool() // Fails on .Bool()
    ```
-   **Root Cause**:
    1.  The evaluator processes `app.Flag("debug", "...")`.
    2.  This call correctly resolves to the `Flag` method, which is modeled by an intrinsic or a function.
    3.  The result of evaluating this call is an `*object.Function` representing the *next* method in the chain (e.g., `Bool`), with the receiver (`app`) bound to it.
    4.  The evaluator then tries to evaluate the `.Bool()` part of the chain. The left-hand side of the selector is now the `*object.Function` object from the previous step.
    5.  `evalSelectorExpr`'s main `switch` statement does not have a `case` for `*object.Function`. It falls through to the `default` case, which produces the "got FUNCTION" error.
-   **Alignment with Design**: This is a **bug**. The `symgo` engine, as a tracer, should be able to follow method chains. The failure is not due to a design limitation but an incomplete implementation in `evalSelectorExpr`. The evaluator should recognize that the `*object.Function` has a bound receiver and continue the evaluation on that receiver.

### 2.2. Method on Named Slice Failure

-   **Symptom**: The log shows the error `expected a package, instance, or pointer on the left side of selector, but got SLICE`.
-   **Example Code**:
    ```go
    type MySlice []int

    func (s MySlice) Sum() int { /* ... */ }

    func main() {
        var s MySlice
        s.Sum() // Fails here
    }
    ```
-   **Root Cause**:
    1.  The evaluator correctly identifies `s` as an `*object.Slice`.
    2.  It then attempts to evaluate the selector `.Sum`.
    3.  `evalSelectorExpr`'s main `switch` statement does not have a `case` for `*object.Slice`. It falls through to the `default` case, producing the "got SLICE" error.
    4.  A secondary bug was also identified in `evalCompositeLit`: when evaluating a literal of a named slice type (e.g., `MySlice{1, 2, 3}`), the resulting `*object.Slice` object was not correctly tagged with the `TypeInfo` for `MySlice`. This would prevent method resolution even if `evalSelectorExpr` were fixed.
-   **Alignment with Design**: This is a **bug**. Go allows defining methods on any named type, including slices. For `symgo` to accurately trace Go code, it must support this common language feature.

## 3. Proposed Solutions

To address these issues, the following fixes are proposed:

1.  **Fix `evalCompositeLit`**: Modify `evalCompositeLit` to correctly associate the type alias information (`aliasTypeInfo`) with the `*object.Slice` it creates. This ensures that the object carries the necessary metadata for method resolution.

2.  **Enhance `evalSelectorExpr`**:
    -   **For Fluent APIs**: Before the main `switch` statement, add a check: if the object being evaluated is an `*object.Function` with a non-nil `Receiver`, the evaluator should "unwrap" it and continue the evaluation using the receiver as the new left-hand side object.
    -   **For Named Slices**: Add a `case *object.Slice:` to the main `switch` statement. Inside this case, if the slice has `TypeInfo` (i.e., it's a named type), the evaluator should use the `accessor` to find the method (`Sum` in the example) on that type. It should also check for any matching intrinsics, which is crucial for testing. If no method is found, it should return a specific "undefined method" error. If the slice is anonymous (no `TypeInfo`), it should fall through to the default error, as anonymous slices cannot have methods.

Implementing these changes will make the `symgo` evaluator more robust and capable of analyzing a wider range of common Go code patterns, aligning it better with its goal of being a comprehensive static analysis tool.