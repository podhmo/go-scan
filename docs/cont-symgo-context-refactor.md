### **Continuation Document: Refactor `symgo` to Propagate `context.Context`**

**1. Initial Prompt**

> TODO.mdから1つ読み実装してください。必要ならサブのTODOタスクに分解してください。分解した後はTODO.mdに書いても良いです。
>
> その後作業を進めてください。テストが成功するまでコードを修正し続けてください。作業終了後は必ず最後にTODO.mdを更新してください。
>
> 選ぶタスクはdocgenのタスクにしてください。具体的にはexamples/docgen/analyzer.goでstructにctxをフィールドとして持つのは辞めてctxを引数として取るように修正してください。テストコードは利用してもかまいません。ビルドエラーも修正してください。
>
> 完遂できなかった場合にもTODO.mdに追記してください。

*(Translation: Please read and implement one task from TODO.md. If necessary, break it down into sub-tasks. You can write the decomposed tasks in TODO.md. Then, please proceed with the work. Keep fixing the code until the tests pass. After finishing the work, please be sure to update TODO.md at the end. The task to choose should be a docgen task. Specifically, in examples/docgen/analyzer.go, please stop having ctx as a field in the struct and instead take ctx as an argument. You may use the test code. Please also fix any build errors. If you cannot complete it, please also add it to TODO.md.)*

**2. Goal**

The primary goal is to refactor `examples/docgen/analyzer.go` to remove the `context.Context` field from the `Analyzer` struct. Instead, the context should be passed as an argument to the methods that require it.

**3. Initial Implementation Attempt**

My first step was to analyze `analyzer.go`. I identified that the `ctx` field was used by several methods, most of which were registered as `IntrinsicFunc` handlers with the `symgo` interpreter. My initial thought was to simply add a `context.Context` argument to these methods and pass it down.

**4. Roadblocks & Key Discoveries**

I quickly discovered that this was not a simple change. The `symgo.IntrinsicFunc` signature did not include a `context.Context`. The context was passed down the main `Eval` call chain in the evaluator, but it was not available to the intrinsic handlers called during evaluation.

This led to the key discovery: to properly solve the issue in `docgen`, I needed to perform a more fundamental refactoring within the `symgo` engine itself. The core task became changing the signature of `IntrinsicFunc` throughout the system to accept a `context.Context`.

**5. Major Refactoring Effort**

Based on this discovery, I undertook a significant refactoring with these key changes:

*   **`symgo/object/object.go`**: Modified the `Intrinsic` struct's `Fn` field to `func(ctx context.Context, args ...Object) Object`.
*   **`symgo/intrinsics/intrinsics.go`**: Updated the `IntrinsicFunc` type definition to `func(ctx context.Context, args ...object.Object) object.Object`.
*   **`symgo/symgo.go`**: Updated the user-facing `IntrinsicFunc` type definition and all wrapper functions (e.g., `RegisterIntrinsic`) to correctly handle the new signature and pass the context.
*   **`symgo/evaluator/evaluator.go`**: Modified all call sites for intrinsic functions (e.g., in `evalCallExpr`, `evalSelectorExpr`, `Finalize`) to pass the `context.Context` to the intrinsic handlers.
*   **`examples/docgen/analyzer.go`**: With the core `symgo` changes in place, I successfully refactored `analyzer.go` as per the original request, removing the `ctx` field and passing it as an argument.

**6. Current Status**

The core refactoring is complete. However, this was a breaking change that has impacted numerous test files across the `symgo` package. I am currently in the process of fixing the resulting build errors in these test files.

I have already fixed a significant number of them (in `symgo_interface_resolution_test.go`, `evaluator_call_test.go`, etc.). The current build failure is in `symgo/evaluator` and `symgo` test files, where test-specific intrinsics still use the old function signature.

**7. References**

A future agent should consult the following files to understand the core changes:
*   `symgo/object/object.go` (specifically the `Intrinsic` struct)
*   `symgo/intrinsics/intrinsics.go`
*   `symgo/evaluator/evaluator.go` (specifically `evalCallExpr` and `applyFunction`)
*   `symgo/symgo.go` (specifically `RegisterIntrinsic`)

**8. TODO / Next Steps**

1.  Continue fixing the test files that are currently failing to build due to the `IntrinsicFunc` signature change.
2.  Once all `go test ./...` build errors are resolved, run the full test suite to ensure all tests pass.
3.  Debug and fix any runtime test failures that may have been introduced by the refactoring.
4.  Once all tests pass, the task will be complete. I will then update `TODO.md` as requested in the initial prompt.
