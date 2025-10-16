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

With a well-defined scope, the tool will use the `symgo` engine to trace execution paths from all `main` functions within the scope. The core of the design lies in how function calls are intercepted and analyzed. Two approaches were considered.

#### Approach A: Targeted Intrinsic (Efficient but Flawed)

This approach prioritizes performance by focusing only on calls to the target function.

*   **Mechanism:** Use `RegisterIntrinsic` to register a specific intrinsic handler **only for the target function** (e.g., `example.com/mylib.MyFunction`).
*   **Action:** When the intrinsic is triggered, it signifies a direct call. The tool would then capture the current call stack via `interpreter.CallStack()`.
*   **Advantage:** Highly performant, as it ignores all other function calls during the analysis.
*   **Critical Flaw:** This approach **fails to trace calls made through interfaces**. If `main` calls a method on an interface `I`, and the target function is a method on a concrete type `T` that implements `I`, the intrinsic for `T.MyMethod` will never be triggered. The trace is lost at the interface boundary. This makes the approach unsuitable for most real-world Go codebases.

#### Approach B: Default Intrinsic with Post-Analysis (Robust and Recommended)

This approach prioritizes correctness and completeness by inspecting all calls and resolving interface calls after the trace. **This is the recommended design.**

1.  **Symbolic Execution with a Default Intrinsic:** The tool will use `RegisterDefaultIntrinsic` to intercept **all** function calls that occur during the symbolic execution starting from each `main` entry point.

2.  **Call Classification and Recording:** Inside the default intrinsic, each function call will be classified and recorded:
    *   **Direct Hits:** If the called function is the concrete target function, the current call stack (obtained via `interpreter.CallStack()`) is immediately recorded as a confirmed call path.
    *   **Potential Hits (Interface Calls):** If the called function is an interface method, the tool will record the call stack and the fully-qualified name of the interface method (e.g., `"io.Writer.Write"`). These are stored separately as "potential paths."

3.  **Post-Trace Path Resolution:** After the symbolic execution from all `main` functions is complete, a final resolution step connects the potential paths:
    *   **Build Interface Map:** The tool will build a map of all interfaces in the analysis scope to their concrete implementers. This logic can be adapted directly from `examples/find-orphans`.
    *   **Connect Paths:** The tool iterates through the recorded "potential paths." For each path that ends in a call to an interface method (e.g., `I.Do`), it checks if the target function's receiver type (e.g., `T`) is a known implementer of that interface `I`.
    *   If `T` implements `I`, the potential path is confirmed as a valid call path to the target function and is added to the final results.

This robust approach ensures that call paths are not lost when they go through an interface, providing a complete and accurate trace.

## 3. Technical Investigation & Feasibility

A technical investigation was conducted to assess the viability of this design.

*   **`symgo` Capabilities:** The `symgo` engine is a symbolic tracer that explores all possible code paths, making it suitable for this task. It correctly models interface calls as symbolic placeholders, which is the key behavior that enables Approach B.
*   **Call Stack Access:** The core of the tool relies on accessing the full call stack. The `symgo/evaluator.Evaluator` maintains an internal `callStack`, but it is not publicly exposed. A minor, non-breaking change to the `symgo` library is required to add a public `CallStack()` method to the `symgo.Interpreter`.

## 4. Open Questions and Future Considerations

1.  **Performance at Scale:** The recommended robust approach (Approach B) requires inspecting every function call via a default intrinsic. While correct, this is less performant than the flawed targeted approach. For very large codebases, the overhead of the default intrinsic could be significant. Performance profiling will be important.

2.  **Handling Dynamic Calls:** This remains a key consideration. The `symgo` engine operates on static source code.
    *   **Reflection:** Calls made via `reflect.ValueOf(fn).Call()` will likely not be detected. This should be documented as a known limitation.
    *   **Cgo:** Function calls that cross the Cgo boundary will not be traced.

3.  **Usability of Output:** What is the most effective way to present the results to the user? Options include a simple list of stack traces, a structured JSON format, or a graph visualization.

4.  **Target Function Specification:** The CLI needs a robust way to parse the target function signature, including methods on both value and pointer receivers.