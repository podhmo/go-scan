# Design Document: The Evolution of Interface Call Tracing in `symgo`

This document serves as a design retrospective for the implementation of interface call tracing within the `symgo` ecosystem, primarily for the `examples/call-trace` tool. It details the journey from an initial, flawed understanding to the final, robust solution, including the architectural dead ends explored and the key technical obstacles that shaped the final design.

The purpose is to provide context for future development, particularly for anyone considering adding similar deep-analysis features to `symgo` or its consuming tools.

## 1. The Core Challenge: Tracing Through Abstraction

The goal was to enable `call-trace` to identify call stacks leading to a target function, even when the call path traversed through an interface.

```go
// We want to trace the call from main() to Helper()
// The path goes through an interface: repo.Save()
//
// main() -> uc.Execute() -> repo.Save() -> (*ConcreteRepo).Save() -> Helper()

type Repository interface {
    Save(data string)
}

func (uc *UseCase) Execute() {
    uc.repo.Save("data") // How does symgo know which `Save` this is?
}
```

The fundamental problem is that at the `repo.Save()` call site, the concrete type of `repo` is not statically known. A symbolic execution engine must have a strategy to resolve this ambiguity.

## 2. Initial Hypothesis & A Deceptive Success

The implementation began with a simple, single-implementation test case.

-   **Observation:** The test passed immediately.
-   **Flawed Conclusion:** This led to the incorrect initial assumption that the `symgo` engine already possessed a mechanism to resolve simple interface-to-implementation cases automatically. This early success masked the true complexity of the problem.

When a more realistic test case with multiple possible implementations (`multi_impl`) was introduced, the trace failed. This proved that a dedicated design was necessary to handle ambiguity.

## 3. Design Fork #1: Resolution within the Tool (A Dead End)

The first architectural decision was whether to place the resolution logic in the core engine (`symgo`) or in the consuming tool (`call-trace`). The initial approach attempted the latter.

-   **Design:** `call-trace` would be responsible for resolving interfaces.
    1.  It would first use `go-scan` to build a complete map of all interfaces and their concrete implementations across all scanned packages.
    2.  `symgo` would run its analysis, stopping at the interface method call (e.g., `repo.Save()`).
    3.  The `call-trace` intrinsic would be triggered. It would then use the pre-built map to see if any of the possible concrete implementations matched the user's target function.

-   **Technical Obstacles & Rationale for Abandonment:**
    1.  **Fundamentally Incorrect for Call *Stack* Tracing:** This design has a fatal flaw. `symgo`'s evaluation would terminate at the interface boundary. The tool could determine that `repo.Save()` *could potentially* call `(*ConcreteRepo).Save()`, but it could not continue the trace *from* `(*ConcreteRepo).Save()` to its own subsequent calls (i.e., to `Helper()`). The call stack would be incomplete, defeating the primary purpose of the tool.
    2.  **Unnecessary Complexity:** It forced the tool to re-implement a significant amount of type resolution logic that philosophically belongs in the core analysis engine.

This approach was abandoned because it could not produce the required output (a full call stack) and violated the separation of concerns between the engine and the tool.

## 4. Design Fork #2: Resolution within the Engine (The Correct Path)

The responsibility for tracing *through* an interface must reside within the `symgo` evaluator itself. The engine must be able to resolve the concrete type(s) and continue the symbolic execution into each valid implementation.

Implementing this revealed two critical, independent bugs in the `symgo` engine that needed to be fixed before the core logic could work.

### 4.1. Prerequisite Fix: Cross-Package Evaluation Context

-   **Problem:** A realistic multi-package test (`ddd_scenario`) failed with "identifier not found" errors.
-   **Root Cause:** When `symgo` evaluated a method from another package (e.g., calling a method from `pkg/infra` inside `pkg/app`), it was incorrectly using the *caller's* (`pkg/app`) package context to look up symbols. It should have been using the *callee's* (`pkg/infra`) context.
-   **Solution:** A patch was applied to `symgo/evaluator/evaluator_apply_function.go`. This fix ensures that whenever a function or method is evaluated, the engine's context is authoritatively switched to the package where that function is defined (`fn.Def.PkgPath`). This was a fundamental correctness bug in `symgo`.

### 4.2. Prerequisite Fix: Robustness at the Tooling Boundary

-   **Problem:** Even with the context fix, the trace still failed. The `call-trace` intrinsic was receiving a `scanner.FunctionInfo` object from the engine that was missing its `PkgPath`.
-   **Root Cause:** The data structure being passed from the engine's core to the tool's intrinsic hook was incomplete.
-   **Solution:** The `call-trace` tool was made more resilient. A fallback was added to its `getFuncTargetName` function. If `PkgPath` was empty, it would attempt to reconstruct it from the receiver's fully-qualified type name (e.g., extracting `"ddd_scenario/pkg/infra"` from `"(*ddd_scenario/pkg/infra.repository)"`).
-   **Design Principle:** This highlights a key principle for the `symgo` ecosystem: tools should be designed defensively and be robust to receiving partially populated information from the engine, especially during complex analysis runs.

## 5. Final Architecture and Rationale

The final, working architecture is a two-part solution that addresses the issues discovered:

1.  **Core Engine Fix (`symgo`):** The evaluator is now able to correctly maintain its package context during cross-package calls. This is the foundation upon which any cross-package analysis rests.
2.  **Tool Resilience (`call-trace`):** The tool can now handle cases where the engine provides incomplete data, preventing silent failures.

This design correctly places the complex responsibility of flow analysis and context management within `symgo`, while ensuring that the consuming tool can reliably interpret the results.

## 6. Future Considerations and Alternative Designs

While the current solution is functional, the debugging journey revealed opportunities for future enhancements to `symgo`.

-   **Alternative Design: Post-Hoc Resolution with `Finalize()`**
    -   An alternative considered was to have the evaluator track all possible concrete types for an interface variable, and then resolve all possible calls in a final, post-processing step (e.g., in `Interpreter.Finalize()`).
    -   **Why it was rejected for this use case:** This is the same fundamental issue as Design Fork #1. It cannot build a complete, continuous call *stack*. It is suitable for asking "what *could* this interface call resolve to?", but not for "what is the full call stack from this `main` function to that `Helper` function?".

-   **Future `symgo` Enhancement: Deeper Analysis as an Option**
    -   The current implementation makes `symgo` trace through interfaces by default. This has performance implications, as it can significantly expand the analysis space.
    -   A future improvement would be to make this behavior optional, controlled by a new `symgo.Interpreter` option (e.g., `symgo.WithInterfaceResolution(true)`). This would allow users to choose between a faster, shallower analysis and a slower, deeper analysis depending on their needs.

-   **Future `symgo` Enhancement: Complete `FunctionInfo` Guarantee**
    -   The need for a fallback in `call-trace` indicates that `symgo` could be improved. The engine should ideally guarantee that any `FunctionInfo` object passed to an intrinsic is fully populated, removing this burden from tool authors.
