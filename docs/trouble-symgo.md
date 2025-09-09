# Trouble-Shooting: `symgo` Interface State Merging Across Control Flow

## 1. The Problem

The `symgo` symbolic execution engine fails to correctly aggregate the state of interface variables across different control-flow branches. When an interface variable is assigned different concrete types within `if` and `else` blocks, the evaluator only retains the state from the last branch it evaluates. This leads to an incomplete understanding of the possible concrete types an interface may hold, causing failures in downstream analysis tools that rely on this information.

## 2. Evidence: `TestEval_InterfaceMethodCall_AcrossControlFlow`

This behavior is clearly demonstrated by the failing test case `TestEval_InterfaceMethodCall_AcrossControlFlow` in `symgo/evaluator/evaluator_interface_method_test.go`.

The test sets up the following scenario:
```go
var s Speaker // Interface
if someCondition {
    s = &Dog{}
} else {
    s = &Cat{}
}
s.Speak()
```

The test asserts that the `object.Variable` corresponding to `s` should have two entries in its `PossibleTypes` map: `*Dog` and `*Cat`. However, the test fails because the map only contains one of these types, indicating that the state from one branch is overwriting or ignoring the state from the other.

## 3. Root Cause Analysis

The root cause lies in the implementation of `evalIfStmt` in `symgo/evaluator/evaluator.go`. The function correctly follows a symbolic execution pattern by evaluating both the `then` and `else` blocks. It creates separate, enclosed environments (`thenEnv` and `elseEnv`) for each branch to ensure lexical scoping is respected.

However, after the evaluation of these branches completes, there is no logic to merge the resulting state changes from `thenEnv` and `elseEnv` back into the parent environment (`ifStmtEnv`). The `assignIdentifier` function correctly adds a new concrete type to a variable's `PossibleTypes` set, but it does so on a `Variable` object within the temporary branch environment. This environment and all the state changes within it are discarded once the branch evaluation is complete.

As a result, the `PossibleTypes` of any variable modified within the `if` or `else` block are not persisted in the parent scope, leading to the observed bug.

## 4. Contradiction with Existing Documentation

The document `docs/analysis-symgo-implementation.md` contains a section that incorrectly describes the engine's behavior in this exact scenario. Section 2.1 states:

> Furthermore, the `assignIdentifier` function contains special logic for variables with an `interface` type. When a value is assigned to an interface variable, the evaluator **adds** the concrete type of the value to a set of `PossibleTypes` on the variable object. This "additive update" mechanism is how the evaluator correctly merges the outcomes of both `if` and `else` branches...

This analysis is flawed. While the "additive update" logic in `assignIdentifier` exists, it is rendered ineffective by the lack of a state-merging step in `evalIfStmt`. The documentation describes the intended design, but not the actual, buggy implementation.

## 5. Proposed Solution

The fix is to enhance `evalIfStmt`. After evaluating the `then` and `else` branches, new logic will be added to:
1.  Iterate through the variables in the parent environment (`ifStmtEnv`).
2.  For each variable, look up its counterpart by name in `thenEnv` and `elseEnv`.
3.  If a counterpart exists and has a populated `PossibleTypes` set, merge those types into the `PossibleTypes` set of the original variable in `ifStmtEnv`.

This will ensure that the effects of assignments within all control-flow paths are correctly accumulated.
