# Trouble: `symgo` evaluator hangs due to infinite recursion

This document outlines the investigation and resolution of a bug where the `symgo` symbolic execution engine hangs indefinitely.

## 1. Problem

When the `symgo` engine is used to analyze certain Go codebases, it enters an infinite loop and never terminates. This was observed when analyzing code containing functions that perform lookups in their outer scope.

### Symptoms

The primary symptom is that the analysis process hangs. Log files show a large number of repeated warning messages, indicating a persistent, looping error state within the evaluator.

The following log entry is an example of the repeated message:

```
time=2025-09-10T20:39:38.529+09:00 level=WARN msg="expected multi-return value on RHS of assignment" in_func=Get in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/minigo/object/object.go:1135:10 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2189 got_type=SYMBOLIC_PLACEHOLDER
```

### Log Analysis

Based on user feedback and further analysis, the log should be interpreted as follows:

-   `exec_pos`: This points to the location in the `symgo` evaluator that is currently running. In this case, it's inside `symgo/evaluator/evaluator.go`.
-   `in_func_pos`: This points to the location in the **code being analyzed** by `symgo`. In this case, `symgo` was analyzing a file located at `minigo/object/object.go`.

This means the bug is not in the `minigo` code itself. Rather, the `symgo` evaluator, while attempting to symbolically execute the `Get` function from the `minigo` package, gets stuck in an internal infinite loop. The problem lies within `symgo`'s own environment and scope-handling logic.

## 2. Cause

The investigation into `symgo/evaluator/evaluator.go` and `symgo/object/object.go` revealed the following:

1.  **Recursive `Get`:** The `symgo` environment lookup method, `symgo/object.Environment.Get()`, is a simple recursive function. It looks for a variable in the current scope and, if not found, calls itself on the `outer` environment.
2.  **No Cycle Detection:** This `Get` method has no mechanism to detect cycles. If the chain of `outer` pointers ever forms a loop (e.g., `EnvA.outer -> EnvB.outer -> EnvA`), the `Get` method will recurse infinitely.
3.  **Complex Environment Chains:** The `symgo` evaluator creates complex environment chains to correctly model Go's lexical scoping, especially for closures. A function object holds a reference to the environment where it was defined (`Function.Env`). This function object is then stored *in* that same environment, creating a necessary data cycle (`PackageEnv -> FunctionObject -> PackageEnv`).

The root cause is the combination of a non-robust `Get` method and the complex, necessarily cyclic data structures required to model Go's scope. While the data cycle itself is not a bug, it increases the risk that another subtle bug in the evaluator could accidentally create a cycle in the `outer` pointer chain.

The fix is to make `Environment.Get()` robust by adding cycle detection, preventing it from hanging even if the environment chain is corrupted.
