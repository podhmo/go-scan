# Design and Technical Investigation for `examples/call-trace`

This document outlines the design and technical feasibility of a new command-line tool, `examples/call-trace`. The tool's primary objective is to identify the command-line entry points (i.e., `main` functions) that result in a call to a specified target function.

This document also records the evolution of the design, particularly regarding the strategy for call interception, to provide context for the final recommended approach.

## 1. Objective

The goal of `call-trace` is to answer the question: "Which commands in my codebase call function `F`?"

This is valuable for understanding the impact of changing a shared function, or for discovering all the binaries that interact with a specific resource (e.g., a web API client method or an SQS queue).

## 2. High-Level Design

The tool will operate in two main phases.

### Phase 1: Scope Discovery (Dependency Analysis)

Before performing an expensive analysis, the tool must first identify a minimal set of packages that could possibly call the target function.

1.  **Input:** The user provides a target function (e.g., `example.com/mylib.MyFunction`).
2.  **Action:** The tool will use the reverse dependency walking logic, similar to that in `examples/deps-walk --direction=reverse`. It will start from the package containing the target function (`example.com/mylib`) and find all packages that import it, directly or transitively.
3.  **Output:** This phase produces a set of packages. This set defines the **Primary Analysis Scope**. Only code within this scope will be symbolically executed.

### Phase 2: Call Path Tracing (Symbolic Execution)

With a well-defined scope, the tool will use the `symgo` engine to trace execution paths from all `main` functions within the scope. The core of the design lies in how function calls are intercepted and analyzed. Three approaches were considered.

#### Approach A: Targeted Intrinsic (Efficient but Flawed)

*   **Mechanism:** Use `RegisterIntrinsic` to register a specific intrinsic handler **only for the target function**.
*   **Advantage:** Highly performant, as it ignores all other function calls.
*   **Critical Flaw:** This approach **fails to trace calls made through interfaces**. If `main` calls a method on an interface `I`, and the target function is a method on a concrete type `T` that implements `I`, the intrinsic for `T.MyMethod` will never be triggered. The trace is lost.

#### Approach B: Default Intrinsic with Post-Analysis (Robust Default)

This approach prioritizes correctness and completeness by inspecting all calls and resolving interface calls after the trace.

*   **Mechanism:**
    1. Use `RegisterDefaultIntrinsic` to intercept **all** function calls.
    2. Inside the intrinsic, classify calls into "Direct Hits" (calls to the target function) and "Potential Hits" (calls to an interface method), recording the call stack for each.
    3. After the trace, build an interface implementation map (like in `find-orphans`).
    4. Connect "Potential Hits" to the target function if its receiver type implements the called interface.
*   **Advantage:** Guarantees that all potential call paths through interfaces are discovered.
*   **Trade-off:** This approach is conservative and may produce false positives. It reports that a call *could* happen, but doesn't prove that it *will* for a specific execution. For example, if an interface `I` is implemented by types `T1` and `T2`, and the target is `T1.Method`, this approach will flag any call to `I.Method` as a potential call path, even if the runtime instance is always `T2`.

#### Approach C: Dynamic Configuration-Aware Binding (High-Precision, Future Goal)

This approach aims to eliminate the false positives of Approach B by inferring type bindings from the configuration phase of each command.

*   **Problem:** Different `main` functions can bind different concrete types to the same interface. A global, manual binding (e.g., via a command-line flag) is not flexible enough to handle this.
*   **Mechanism (Proposed `symgo` Enhancement):**
    1.  Enhance the `symgo` engine to recognize common dependency injection patterns or constructor functions (e.g., `wire.Build`, or `NewApp(db *sql.DB)`).
    2.  As `symgo` traces from a specific `main` function, it would analyze this initialization code to determine that, for *this specific trace*, `DBInterface` is bound to `*sql.DB`.
    3.  This binding context would be associated with the current trace. When `symgo` later encounters a call to a method on `DBInterface`, it would use the trace-specific context to resolve it to a call on `*sql.DB`.
*   **Advantage:** Eliminates false positives by understanding the specific configuration of each command, leading to highly accurate traces without manual user input.
*   **Trade-off:** This requires a very sophisticated enhancement to the `symgo` engine. The implementation complexity is high.

### Conclusion on Approach

**Approach B is the recommended design for the initial implementation.** It is robust, achievable with the current `symgo` capabilities (plus the minor `CallStack` addition), and provides comprehensive results. The risk of false positives is a known and acceptable trade-off for ensuring no potential call path is missed.

**Approach C represents the ideal future state of the tool.** It offers the highest precision and should be considered the long-term goal for `symgo` and `call-trace` to maximize their analytical power.

## 3. Technical Investigation & Feasibility

*   **`symgo` Capabilities:** `symgo` is well-suited for this task. Its ability to symbolically trace all paths and model interface calls is the foundation for the recommended approach.
*   **Call Stack Access:** The core of the tool relies on accessing the full call stack. The `symgo/evaluator.Evaluator` maintains an internal `callStack`, but it is not publicly exposed. A minor, non-breaking change to the `symgo` library is required to add a public `CallStack()` method to the `symgo.Interpreter`.

## 4. Open Questions and Future Considerations

1.  **Performance at Scale:** A key consideration is performance on large codebases. The recommended robust approach (Approach B) requires inspecting every function call. For very large projects, the overhead could be significant, and performance profiling will be important.

2.  **Handling Dynamic Calls:** The `symgo` engine operates on static source code, which imposes certain limitations.
    *   **Reflection:** Calls made via `reflect.ValueOf(fn).Call()` will likely not be detected. This should be documented as a known limitation of the tool.
    *   **Cgo:** Function calls that cross the Cgo boundary will not be traced.

3.  **Usability of Output:** The most effective way to present the results to the user should be considered. Options include a simple list of stack traces, a structured JSON format for machine processing, or a graph visualization for exploring call trees.

4.  **Target Function Specification:** The CLI will need a robust way to parse the target function signature, including methods on both value and pointer receivers (e.g., `(T).Method` vs. `(*T).Method`).