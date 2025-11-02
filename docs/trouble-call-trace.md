# Design Doc: Tracing Interface Method Calls in `call-trace` (A Failed Attempt)

**Status**: Superseded
**Author**: Jules
**Date**: 2025-10-20

## 1. Introduction & Goal

This document outlines the design and implementation attempt to extend the `examples/call-trace` tool to trace function calls made through interfaces. The goal was to enable the tool to identify call stacks leading to a concrete method implementation, even when the call is dispatched via an interface.

This attempt was ultimately unsuccessful, but the investigation revealed critical challenges and architectural considerations in the `symgo` evaluation engine and its interaction with client tools. This document serves as a record of the obstacles encountered and proposes a path forward.

## 2. Proposed Design

The core design was based on a two-pronged approach: enhancing `symgo` to resolve interface calls and updating `call-trace` to interpret the results.

### 2.1. `symgo` Evaluator Enhancements

1.  **Track Concrete Types**: The `symgo` evaluator already has a mechanism to track the possible concrete types that an interface-typed variable can hold. This is stored in the `PossibleTypes` map on the `object.Variable`. This mechanism needed to be correctly leveraged. The first step was to ensure `PossibleTypes` was populated correctly during assignments, especially within struct literals, which was identified as a gap.
2.  **Resolve Concrete Implementations**: When `evalSelectorExpr` encounters a method call on an interface type (e.g., `myInterface.MyMethod()`), it should:
    a. Access the `PossibleTypes` map of the variable representing `myInterface`.
    b. For each concrete type string in the map, find the corresponding `*scanner.TypeInfo`.
    c. On that `TypeInfo`, find the method `MyMethod`.
    d. Collect all found `*scanner.FunctionInfo` objects for the concrete methods.
3.  **Enrich `SymbolicPlaceholder`**: The collected list of concrete `FunctionInfo` objects should be stored in a new field, `ConcreteImplementations`, on the `object.SymbolicPlaceholder` returned by `evalSelectorExpr`. This placeholder represents the unresolved interface call but is now enriched with potential targets.

### 2.2. `call-trace` Tool Update

The `defaultIntrinsic` handler in `call-trace`, which intercepts all function calls, would be modified:
1.  When the intercepted "function" is a `*object.SymbolicPlaceholder`, check if its `ConcreteImplementations` field is non-empty.
2.  Iterate through the `FunctionInfo` objects in the list.
3.  For each one, generate its fully qualified name and compare it against the `targetFunc` provided by the user.
4.  If a match is found, record the current call stack.

## 3. Obstacles and Analysis of Failure

The implementation of this design failed. While a failing test case was successfully created, making it pass proved impossible due to a series of interacting, deep-seated issues.

### 3.1. Issue #1: Analysis Scope (`analysisScope`) Mismatch

The most significant architectural challenge is the mismatch between the analysis scope required by `symgo` and the scope discovery logic in `call-trace`.

-   **The Problem**: `symgo`'s evaluator will only execute the body of a function if that function's package is within the `ScanPolicy` (derived from `analysisScope`). If a function's package is outside the scope, calls to it are treated as "out-of-policy," and the evaluator returns a `SymbolicPlaceholder` without analyzing the function's content. In our test case, the call chain is `myapp` -> `usecase` -> `infrastructure`. For `symgo` to trace into `usecase.GetUserByID`, the `usecase` package *must* be in the `analysisScope`.
-   **`call-trace`'s Original Logic**: The tool's scope discovery was designed to find *callers* of the target, not the entire dependency graph. It started with the target's package (`infrastructure`) and walked the reverse dependency graph *upwards*. It found `myapp` (which imports `infrastructure`) but had no mechanism to then walk *downwards* from `myapp` to discover its other dependencies like `usecase`.
-   **The Result**: The `usecase` package was never added to `analysisScope`. Consequently, when `symgo` evaluated the call to `usecase.GetUserByID` in `myapp`, it treated it as an out-of-policy call. It never analyzed the body of `GetUserByID`, and therefore never saw the call to `Repo.Find()`.

**Attempted Fix**: The scope logic was rewritten to build a complete dependency cone (both forward and reverse dependencies) starting from the user's input patterns. This seemed to fix the scope issue in theory, but the test still failed, indicating other underlying problems.

### 3.2. Issue #2: Fragility of `IntrinsicFunc` Signature

A subtle but critical bug was the misunderstanding and incorrect implementation of the `symgo.IntrinsicFunc` signature within the `call-trace` tool.

-   **The Problem**: `symgo.Interpreter`'s `RegisterDefaultIntrinsic` method takes a `symgo.IntrinsicFunc`, which is defined as `func(ctx context.Context, eval *Interpreter, args []Object) Object`. Inside the `symgo` package, this is wrapped to fit the internal evaluator's `intrinsics.IntrinsicFunc` signature, which is `func(ctx context.Context, args ...object.Object) object.Object`. During the long debugging session, the closure in `call-trace` was incorrectly defined with the internal signature. This caused the `*symgo.Interpreter` argument to be silently dropped, and the `calleeObj` was incorrectly assigned to the interpreter parameter.
-   **The Result**: Calls to `i.CallStack()` inside the handler were method calls on an `object.Object`, not the interpreter. This failed silently at runtime, `directHits` was never populated, and the tool produced "No calls found" even when logging showed a "MATCH FOUND". This created a major contradiction that took hours to resolve.

### 3.3. Issue #3: Complexity of Evaluator State

The process of tracking `PossibleTypes` proved to be fragile.

-   **The Problem**: The state of an interface variable is held in the `PossibleTypes` map on its corresponding `object.Variable`. This map must be correctly populated *at the moment of assignment*. The initial implementation failed because it did not handle assignments within struct literals (`evalCompositeLit`).
-   **The Fix**: Logic was added to `evalCompositeLit` to create a `*object.Variable` wrapper for interface-typed fields, correctly populating `PossibleTypes`.
-   **The Lingering Concern**: While this was fixed, it highlights a design challenge. State propagation in the evaluator is complex. An assignment isn't a simple value swap; it's a state update that must consider the static type of the LHS. This logic is currently spread across `evalAssignStmt` and now `evalCompositeLit`. A more centralized `updateVarOnAssignment` mechanism, as hinted at in memory, might be a more robust design.

## 4. Proposed Path Forward

This feature is achievable, but it requires a more robust architectural approach to scope management and a simplification of the intrinsic mechanism.

### 4.1. Proposal: Explicit Analysis Scopes in `symgo`

The core problem is that different tools require different analysis scopes. `find-orphans` needs to trace from entry points, while `call-trace` needs to analyze a whole "world" of packages. This should be a first-class concept in `symgo`.

A new option could be introduced to `symgo.NewInterpreter`:

```go
// In symgo.go
func WithAnalysisMode(mode AnalysisMode) Option { ... }

type AnalysisMode int
const (
    TraceFromEntrypoints // Current default, for tools like find-orphans
    TraceAllWithinScope  // New mode for tools like call-trace
)
```

When `TraceAllWithinScope` is active, the `scanPolicy` would simply be "is this package part of the initial set provided by the user?". This moves the responsibility of defining the "world" to the client tool, which is what `call-trace` was trying to do manually.

### 4.2. Proposal: Refine the Intrinsic API

The dual `IntrinsicFunc` signatures (public-facing in `symgo` vs. internal in `evaluator`) are a source of confusion. This should be unified or made clearer.

-   **Option A (Simplify)**: Make the public `symgo.IntrinsicFunc` the one and only signature. The `symgo.Interpreter` would pass itself to the `evaluator`, which then passes it to the intrinsic. This adds a dependency from the evaluator to the interpreter but makes the API foolproof for clients.
-   **Option B (Clarify)**: Keep the separation but improve documentation and examples significantly. Add a specific `symgo.DefaultIntrinsicFunc` type that matches the signature expected by `RegisterDefaultIntrinsic` to provide better compiler errors.

## 5. Conclusion

Tracing interface calls is not simply a local change in the evaluator; it is a whole-program analysis problem that is highly sensitive to the analysis scope. The failed attempt demonstrated that the current scope discovery mechanism in `call-trace` is insufficient and that the `symgo` API has subtle complexities.

Future work should focus on implementing the architectural proposals above to provide client tools with more explicit and robust control over the analysis scope, which is the key prerequisite for solving this task. The failing test case and the evaluator enhancements made during this process should be preserved as a baseline for this future work.
