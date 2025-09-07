# Continuing the Symgo Interface Resolution Task

This document outlines the progress, challenges, and next steps for the task of refactoring the `symgo` evaluator to support dynamic interface resolution. The goal is to provide enough context for a developer to continue this work efficiently.

## 0. Initial Prompt

The task was initiated with the following request:

> symgoのevaluatorのinterfaceの解決の仕組みを動的にしたいです。
> 現状の実装では実行時にmethod callが解決できないといけないので、ifaceの利用箇所が先にあった場合に解決できません。
> これを解決するために、method callの解決を遅延させる仕組みを追加してください。
>
> まずは`symgo/evaluator/evaluator_test.go`の`TestInterfaceResolution_Order_Iface_Impl_User`のskipを外してテストを動かすところから始めてください。

**Translation:**
> I want to make the interface resolution mechanism in symgo's evaluator dynamic.
> With the current implementation, method calls must be resolvable at runtime, so it cannot resolve cases where the interface is used before it is implemented.
> To solve this, please add a mechanism to delay the resolution of method calls.
>
> First, please start by removing the skip from `TestInterfaceResolution_Order_Iface_Impl_User` in `symgo/evaluator/evaluator_test.go` and running the test.

## 1. Goal

The primary objective is to make the `symgo` evaluator capable of resolving method calls on interfaces where the concrete implementation is assigned to the interface variable *after* the method call is symbolically evaluated. This requires deferring the resolution of the interface method call until a concrete type is assigned.

The key test case to enable and make pass is `TestInterfaceResolution_Order_Iface_Impl_User` in `symgo/evaluator/evaluator_test.go`.

## 2. Initial Implementation Attempt

My first approach involved the following modifications to `symgo/evaluator/evaluator.go`:

1.  **Introduce New State**: Added two new fields to the `Evaluator` struct:
    *   `pendingInterfaceCalls map[string][]*pendingCall`: To store method calls on interfaces that couldn't be resolved immediately. The key is the fully-qualified interface type name.
    *   `interfaceImplementations map[string][]*scanner.TypeInfo`: To track which concrete types have been assigned to which interfaces.

2.  **Modify `applyFunction`**: When a method call is made on a symbolic placeholder representing an interface, instead of erroring, the call details (receiver, method info, arguments) are stored in `pendingInterfaceCalls`.

3.  **Modify `assignIdentifier`**: When a value is assigned to a variable, check if that variable is an interface type.
    *   If it is, and the assigned value's type implements that interface, store this relationship in `interfaceImplementations`.
    *   Then, check if there are any pending calls for that interface in `pendingInterfaceCalls`.
    *   If so, re-evaluate those pending calls using the newly discovered concrete implementation.

4.  **Add Helper Functions**: Created helper functions `implements()`, `getAllMethodsForType()`, and `compareSignatures()` to perform the interface satisfaction check.

## 3. Roadblock & Key Discovery: The Two Scanners

After the initial implementation, the build failed with confusing type errors, primarily `scanner.TypeInfo is not a type`.

**Initial (Incorrect) Hypothesis**: My first thought was that there was an inconsistency in import aliases for `"github.com/podhmo/go-scan/scanner"` across different files in the `symgo/evaluator` package. I spent some time trying to standardize the aliases (`goscan`, `scan`, etc.), but this did not resolve the issue.

**The Realization**: A deeper investigation revealed a critical structural aspect of the `go-scan` library. There are two distinct `Scanner` types:
1.  `*goscan.Scanner` from the root package `github.com/podhmo/go-scan`.
2.  `*scanner.Scanner` from the sub-package `github.com/podhmo/go-scan/scanner`.

The `symgo/evaluator` was originally written using `*goscan.Scanner`. However, all the rich type information required for the interface resolution logic (e.g., `scanner.TypeInfo`, `scanner.MethodInfo`, `scanner.FieldInfo`) belongs to the `github.com/podhmo/go-scan/scanner` package.

The core problem was an incompatibility between the scanner object being used and the data structures it was expected to produce and consume.

## 4. The Major Refactoring Effort

To resolve this fundamental type mismatch, a significant refactoring of the `symgo/evaluator` package was necessary.

1.  **Standardize on `scanner.Scanner`**: The `Evaluator` and `Resolver` structs, along with their constructor functions (`New`, `NewResolver`), were modified to use `*scanner.Scanner` exclusively. The import for `goscan "github.com/podhmo/go-scan"` was removed from these files.

2.  **Adapt to API Differences**: The `*scanner.Scanner` API is different from the old one. Specifically, accessing the `FileSet` required changing calls from `e.scanner.Fset` to `e.scanner.FileSet()`. All such instances in `evaluator.go` were updated.

3.  **Refactor Tests**: The manual setup of the scanner in many test files (e.g., `TestEvalIntegerLiteral`) was no longer valid due to the more complex constructor of `scanner.Scanner`. All such tests in `evaluator_test.go` were refactored to use the `scantest.Run` helper, which correctly handles the setup and injection of a valid `*scanner.Scanner` instance.

## 5. Current Status & Next Steps

The major refactoring to standardize on the `github.com/podhmo/go-scan/scanner` package is complete. However, the build is still failing with `undefined: scanner` errors. This indicates that despite the extensive search-and-replace effort, some usages of the old `scanner` package identifier (without the `scannerv2` alias that was used as a debugging step) remain.

**TODO:**

1.  **Resolve Final Build Errors**: Meticulously comb through every file in `symgo/evaluator/` one last time. The goal is to find and fix the remaining `undefined: scanner` errors. This will likely involve correcting a few missed spots where the `scannerv2` alias was not applied.

2.  **Run Test Suite**: Once the build is fixed, run the full test suite with `go test ./...`.

3.  **Debug the Feature Logic**: With a clean build, focus on making the `TestInterfaceResolution_Order_Iface_Impl_User` test pass. This involves debugging the core logic in `applyFunction` and `assignIdentifier` to ensure pending calls are correctly stored and executed.

4.  **Expand Test Coverage**: Add more tests to cover various orderings of interface declaration, implementation assignment, and method usage to ensure the solution is robust.

5.  **Submit**: Once all tests pass and the feature is verified, submit the work.
