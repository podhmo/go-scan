# Design and Technical Investigation for `examples/call-trace`

This document outlines the design and technical feasibility of a new command-line tool, `examples/call-trace`. The tool's primary objective is to identify the command-line entry points (i.e., `main` functions) that result in a call to a specified target function.

This investigation leverages existing components within the `go-scan` repository, particularly `examples/deps-walk` and the `symgo` symbolic execution engine.

## 1. Objective

The goal of `call-trace` is to answer the question: "Which commands in my codebase call function `F`?"

This is valuable for understanding the impact of changing a shared function, or for discovering all the binaries that interact with a specific resource (e.g., a web API client method or an SQS queue).

### Example Use Cases:

*   **API Client Usage:** Given a function `client.Users.Create()`, find all `main` packages that call this method to understand which services create users.
*   **SQS Integration:** Given a function `sqs.ReceiveMessage("my-queue")`, find all commands that consume from that queue. Similarly, given `sqs.SendMessage("my-queue", ...)` find all commands that produce messages to it.

## 2. High-Level Design

The tool will operate in two main phases, inspired by the user's suggestion and existing examples.

### Phase 1: Scope Discovery (Dependency Analysis)

Before performing an expensive analysis, the tool must first identify a minimal set of packages that could possibly call the target function.

1.  **Input:** The user provides a target function (e.g., `example.com/mylib.MyFunction`).
2.  **Action:** The tool will use the reverse dependency walking logic, similar to that in `examples/deps-walk --direction=reverse`. It will start from the package containing the target function (`example.com/mylib`) and find all packages that import it, directly or transitively.
3.  **Output:** This phase produces a set of packages. This set defines the **Primary Analysis Scope**. Only code within this scope will be symbolically executed. This prevents the tool from analyzing unrelated parts of the codebase.

### Phase 2: Call Path Tracing (Symbolic Execution)

With a well-defined scope, the tool will use the `symgo` engine to trace execution paths.

1.  **Entry Points:** The tool will identify all `package main` functions within the Primary Analysis Scope. These are the potential command-line entry points.
2.  **Symbolic Execution:** For each `main` function, the `symgo.Interpreter` will be used to symbolically execute the code.
3.  **Call Interception (Efficient Approach):** Instead of intercepting every function call with a default intrinsic, the tool will adopt a more targeted strategy inspired by `examples/docgen`. It will use `RegisterIntrinsic` to register a specific intrinsic handler **only for the target function**.
4.  **Trace Analysis:** When the registered intrinsic is triggered, it signifies a direct call to the target function. The logic inside the intrinsic is straightforward:
    a. Capture the **current call stack** using the proposed `interpreter.CallStack()` method.
    b. Store the captured call stack, which traces back to the `main` entry point, as a valid call path.

This approach is significantly more performant as it avoids the overhead of checking every single function call during the analysis, focusing only on the calls that matter.
5.  **Output:** The tool will report all unique call paths found, grouping them by their `main` function entry point.

## 3. Technical Investigation & Feasibility

A technical investigation was conducted to assess the viability of this design using the current `symgo` implementation.

### 3.1. Existing Components

*   **`examples/deps-walk`:** Provides a robust implementation for Phase 1. Its logic for reverse dependency walking can be reused directly.
*   **`examples/find-orphans`:** Serves as a perfect template for Phase 2. It demonstrates how to configure the `symgo.Interpreter`, set an analysis scope, identify `main` functions, and use a default intrinsic.
*   **`docs/analysis-symgo-implementation.md`:** Confirms that `symgo` is a symbolic tracer that explores all possible code paths, which is exactly what is needed to find all potential call chains, even those behind complex conditional logic.

### 3.2. Key Technical Question: Call Stack Tracking

The core of this tool relies on the ability to retrieve the full call stack when the target function is hit.

*   **Finding:** The `symgo/evaluator.Evaluator` struct contains an internal field: `callStack []*object.CallFrame`. This stack is correctly maintained during execution, with frames being pushed and popped in the `applyFunction` method.
*   **Barrier:** This `callStack` field is **not publicly accessible** via the `symgo.Interpreter` API. The default intrinsic receives the `*symgo.Interpreter` instance, but there is no method on it to get the call stack.
*   **Solution:** A minor, non-breaking change to the `symgo` library is required. A new public method should be added:
    ```go
    // In symgo/evaluator/evaluator.go
    func (e *Evaluator) CallStack() []*object.CallFrame {
        return e.callStack
    }

    // In symgo/symgo.go
    func (i *Interpreter) CallStack() []*object.CallFrame {
        return i.eval.CallStack()
    }
    ```
    With this change, the intrinsic function can call `interpreter.CallStack()` to get the full execution path at the moment of the call, resolving the primary technical barrier.

### 3.3. Technical Limitations & Considerations

*   **Function Pointers/Interfaces:** `symgo` is designed to handle these. When a call is made on an interface, it tracks all possible concrete types. The `call-trace` tool will need to correctly handle this by checking if the target function is part of an interface implementation. The logic in `find-orphans` for building an `interfaceMap` is a good reference.
*   **Memoization:** The user asked if memoization is possible. The `symgo` engine supports this via `symgo.WithMemoization(true)`. The `applyFunction` implementation confirms that memoization lookups occur before the call stack is modified, making it safe and effective to use for this tool to improve performance.
*   **State:** The current `symgo` implementation is stateless regarding function call history within a single trace. The proposed `CallStack()` method provides the necessary state for this tool's purpose.

## 4. Implementation Plan Outline

1.  **CLI:** Create a new `main.go` in `examples/call-trace`.
    *   It will accept one positional argument: the fully-qualified name of the target function (e.g., `example.com/mylib.MyFunction`).
    *   Flags will be similar to `find-orphans`: `--workspace-root`, `--include-tests`, etc.
2.  **Phase 1 Implementation:**
    *   Adapt the reverse dependency logic from `deps-walk` to find the set of packages that depend on the target's package. These packages become the `primary-analysis-scope`.
3.  **Phase 2 Implementation:**
    *   **Modify `symgo`:** Add the `CallStack()` method to `Interpreter` and `Evaluator` as described in section 3.2.
    *   **Setup `symgo.Interpreter`:**
        *   Use the scope from Phase 1 to configure `symgo.WithPrimaryAnalysisScope()`.
        *   Enable memoization: `symgo.WithMemoization(true)`.
    *   **Find Entry Points:** Scan the analysis scope for all `main` packages and their `main` functions.
    *   **Implement the Intrinsic:**
        *   Register a default intrinsic.
        *   Inside the intrinsic, check if the called function object matches the target function signature.
        *   If it matches, call `interpreter.CallStack()` to get the stack trace.
        *   Store the stack trace in a results map, keyed by the entry point function name.
    *   **Execute:** Loop through each `main` entry point and run `interpreter.Apply()` on it.
4.  **Reporting:**
    *   After all entry points have been analyzed, iterate through the collected results.
    *   For each entry point that led to a call, print the call stack in a readable format.

This design is technically sound and leverages the existing strengths of `go-scan` and `symgo`, requiring only a minimal and safe addition to the core library.

## 5. Open Questions and Future Considerations

While the proposed design is sound, there are several open questions and areas for future improvement that should be considered during implementation.

1.  **Performance at Scale:** How will the tool perform on very large monorepos with thousands of packages? While the two-phase approach helps limit the analysis scope, symbolically executing dozens or hundreds of `main` functions could still be time-consuming. Further profiling and optimization may be necessary.

2.  **Handling Dynamic Calls:** The `symgo` engine operates on static source code. How will the tool handle highly dynamic call patterns?
    *   **Reflection:** Calls made via `reflect.ValueOf(fn).Call()` will likely not be detected, as the function being called is not statically known. This should be documented as a known limitation.
    *   **Cgo:** Function calls that cross the Cgo boundary will not be traced.

3.  **Usability of Output:** What is the most effective way to present the results to the user?
    *   Should the output be a simple list of stack traces?
    *   Would a structured format like JSON be more useful for integration with other tools?
    *   Could a graph visualization (e.g., DOT format) be generated to show the call tree?

4.  **Target Function Specification:** The CLI needs a robust way to parse the target function signature. The design assumes a simple `package.Function` format. It must also handle:
    *   Methods on value receivers: `(example.com/mylib.MyType).MyMethod`
    *   Methods on pointer receivers: `(*example.com/mylib.MyType).MyMethod`
    The parsing logic needs to be flexible enough to accommodate these patterns.