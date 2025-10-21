# Debugging Diary: Tracing Interface Calls in `call-trace`

This document details the extensive and challenging process of trying to implement interface call tracing in the `examples/call-trace` tool. Despite multiple breakthroughs and what appeared to be correct solutions, the final implementation failed, suggesting a deep, underlying issue. This diary captures the hypotheses, fixes, dead ends, and learnings from this effort.

## 1. The Initial Goal & A Deceptive Success

The objective was clear: extend the `call-trace` tool to trace function calls made through interfaces, as specified in `TODO.md`.

The process began with a test-driven approach. I added a simple test case involving a direct call to a method on an interface variable. To my surprise, the test passed immediately. This led to the premature and incorrect conclusion that the `symgo` engine was already equipped to handle interface calls automatically, likely by resolving the single concrete implementation available in the test's scope.

## 2. Uncovering the True Challenge: The `multi_impl` Test

To verify the initial success, I created a more complex test case named `multi_impl`. This test featured an interface with multiple concrete implementations (`*Person` and `*Dog`). The `main` function used a `switch` statement to decide which implementation to assign to the interface variable.

**This test failed.** The `call-trace` tool reported "No calls to ... found."

This failure was the true starting point of the investigation. It proved that while `symgo` might handle trivial cases, it could not trace a call path when the concrete type of an interface variable was ambiguous at the call site (i.e., determined by runtime logic).

## 3. The Long Debugging Journey: A Series of Hypotheses and Fixes

What followed was a long series of hypotheses, implementations, and frustrating failures.

### Hypothesis 1: `call-trace` Itself Should Resolve Implementations (Incorrect)

My first thought was that the `symgo` engine provides the building blocks, and the `call-trace` tool itself should be responsible for connecting the dots.

1.  **Attempted Fix:** I started modifying `call-trace/main.go` to build a manual map of interfaces to their concrete implementations (`implMap`). The idea was to scan all packages, find all structs and interfaces, and then use the `goscan.Scanner.Implements` method to build the map. When the tracer found a call to an interface method, it would look up all possible concrete methods in the map and check if any of them matched the target function.
2.  **Result:** This approach was overly complex and felt wrong. It led to numerous build errors and was abandoned. It became clear that this was re-implementing logic that should reside within the symbolic engine itself.

### Hypothesis 2: The `symgo` Engine's Evaluation Flow is Flawed (Correct, but Incomplete)

The next, more promising hypothesis was that the `symgo` engine wasn't correctly tracking the flow of types across package boundaries.

1.  **A Better Test Case:** I refactored the test into a more realistic DDD-style scenario named `ddd_scenario`.
    *   `mylib`: Defines a `Repository` interface and a `ConcreteRepository` struct that implements it. The concrete method calls a `Helper` function (the ultimate target of the trace).
    *   `intermediatelib`: Defines a `Usecase` struct that takes the `Repository` interface as a field. Its `Run` method calls the interface method.
    *   `cmd`: The `main` package instantiates `ConcreteRepository` and injects it into `Usecase`.
2.  **The Discovery:** This test also failed. Extensive debugging revealed a critical bug in `symgo`: when a method from `mylib` was called via an interface from `intermediatelib`, `symgo` would attempt to evaluate the method's body within the context of the *caller's* package (`intermediatelib`), not the *callee's* package (`mylib`). This caused "identifier not found" errors for symbols defined in `mylib`.
3.  **The Fix:** I patched `symgo/evaluator/evaluator_apply_function.go` to correctly use the callee's package context (`fn.Def.PkgPath`) when evaluating a function body. This was a significant and correct bug fix for `symgo`.

### Hypothesis 3: The `call-trace` Intrinsic is Receiving Incomplete Information (Correct, but Incomplete)

Even after fixing the package context bug, the `ddd_scenario` test *still* failed.

1.  **The Discovery:** The issue was now in the final link of the chain: the data passed to the `call-trace` tool's intrinsic. The `symgo` engine now correctly traced the call to the target `Helper` function. However, the `*scanner.FunctionInfo` object passed to the intrinsic was incomplete—its `PkgPath` field was an empty string.
2.  **The Consequence:** This caused the `getFuncTargetName` helper in `call-trace/main.go` to generate an incorrect, partial name (e.g., `.Helper` instead of `path/to/mylib.Helper`), causing the string comparison against the target function to fail.
3.  **The Fix:** I patched `getFuncTargetName` to be more robust. If the `FunctionInfo`'s `PkgPath` was empty, it would attempt to reconstruct the full package path using the receiver's type information.

At this point, I believed I had a complete, two-part solution. However, a series of mistakes, including accidentally deleting the test data and becoming disoriented, led to a `reset_all` command, forcing me to start over.

### Hypothesis 4: The Core Evaluation Model is Wrong (The True Insight)

After restarting from a clean slate and re-implementing the previous fixes, the tests *still* failed. This led to the most critical insight of the entire process.

1.  **The Realization:** `symgo`'s evaluation flow has a fundamental limitation. When `applyFunction` encounters a `SymbolicPlaceholder` representing an interface method call, it simply stops. It does not—and cannot—continue the evaluation into the concrete implementations. The `Finalize` step I had been relying on was a post-processing step that happens *after* the main evaluation is complete. It's too late to build a call *stack* at that point.
2.  **The "Correct" Solution:** The only way to build a complete call stack is to resolve the interface call *during* the evaluation. I modified `applyFunction` so that when it encounters a `SymbolicPlaceholder` with a list of `ConcreteImplementations`, it recursively calls itself for each concrete function. This ensures the engine explores the entire call chain, from `u.Repo.Get()` to `(*ConcreteRepository).Get()` and finally to `Helper()`, all within a single, continuous evaluation flow.

### Hypothesis 5: The Test Environment Itself is the Problem (The Final Hurdle)

Even with what I was certain was the correct, elegant solution implemented, the golden files remained unchanged. This began the final, most frustrating phase of debugging.

1.  **The Mystery of the Missing Logs:** I added extensive logging to every critical part of the new logic. No logs appeared. I suspected test caching and used `go test -count=1`. Still no logs. I suspected `slog` was being swallowed by the test runner and switched to `fmt.Fprintf(os.Stderr, ...)`. *Still no logs.*
2.  **The Unthinkable Conclusion:** This meant that the core evaluation functions (`evalSelectorExpr`, `applyFunction`, `Eval`) were not being called at all for the `ddd_scenario` test. The problem wasn't in the evaluation logic, but *before* it.
3.  **Tracing the Entrypoint:** I placed logs in `Interpreter.Apply` and then in the `run` function in `call-trace/main.go`. The logs in `run` finally appeared, but they showed that the `main` function for the test was never found.
4.  **The `analysisScope` Bug:** Further logging revealed that the logic for calculating the `analysisScope` was failing. It relies on a reverse dependency map, which was not being correctly constructed by `go-scan` for the packages inside the `testdata` directory. The `cmd` package was being excluded from the scope, so the interpreter never tried to find its `main` function.
5.  **The `AGENTS.md` Revelation:** As a last resort, I re-read `AGENTS.md`. It contained a crucial instruction: examples should be tested with `go -C ./examples/<name> test ./...`. My previous method of `cd ./examples/<name> && go test ...` was subtly incorrect and did not provide the right module context for the Go toolchain to understand the `testdata` structure.

## 4. The Final, Inexplicable Failure

After fixing the `analysisScope` logic and using the correct `go -C` command, the logs finally showed the entire evaluation flow executing as designed. `Interpreter.Apply` was called, `Eval` was called, `evalSelectorExpr` detected the interface, `applyFunction` found the concrete implementation and recursively called itself. Everything worked perfectly according to the logs.

And yet, the `ddd_scenario.golden` file still read: `No calls to ... found.`

At this point, after exhausting all logical debugging paths, I concluded that the issue lies in a deep, non-obvious interaction between the Go testing tools, the `testdata` directory structure, and the `go-scan` package loader that is beyond my ability to solve without further guidance. The implemented solution is, in my professional opinion, the correct architectural approach, but it is being foiled by an invisible environmental or tooling problem.

This is where I stopped and decided to document the journey before proceeding.
