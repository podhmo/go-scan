# Design: Enforcing Strict Analysis Scope in `symgo`

## 1. Introduction & Goal

The `symgo` symbolic execution engine is designed to analyze a well-defined scope of code, treating external dependencies as symbolic placeholders. However, the current implementation of the `evaluator` is too "omniscient"—it bypasses its own `ScanPolicy` to load and inspect the full source Abstract Syntax Trees (ASTs) of packages that should be outside its analysis scope.

The goal of this plan is to enforce strict adherence to the `ScanPolicy`. The `evaluator` should not have access to information that the policy prohibits. Any attempt to resolve code outside the defined scope should result in the creation of a symbolic placeholder (e.g., an unresolved type, a symbolic function), not a fully parsed object. This change will make `symgo` faster, more predictable, and more aligned with its original design philosophy.

## 2. Problem Analysis: Policy Bypasses

The core of the problem lies in the `symgo/evaluator/resolver.go` and its usage within `symgo/evaluator/evaluator.go`. The resolver provides methods that explicitly bypass the `ScanPolicy`. These methods are typically named with a `WithoutPolicyCheck` suffix.

The primary locations where these unsafe methods are used are:

1.  **`evaluator.getOrLoadPackage`**: This function is responsible for loading the `object.Package` for a given import path. It currently calls `resolver.resolvePackageWithoutPolicyCheck`. This means that even for a package in `GOPATH` that is outside the analysis scope, the evaluator loads its complete ASTs. The evaluator then attempts to *manually* filter symbols based on the policy within `ensurePackageEnvPopulated`, but it has already incurred the cost of parsing and has access to information it shouldn't.

2.  **`evaluator.evalSelectorExpr` & `evaluator.applyFunction`**: These functions call `getOrLoadPackage` when they encounter a symbol from a package that isn't in the cache. Because `getOrLoadPackage` bypasses the policy, these functions receive a fully-parsed package object, leading to deeper-than-intended analysis.

3.  **`resolver.ResolveFunction`**: When creating an `object.Function` for a method, this function calls `resolver.resolveTypeWithoutPolicyCheck` to resolve the method's receiver type. This is done to provide detailed type information, but it violates the principle that out-of-policy types should remain unresolved. If a method is defined on a type from an external dependency, its receiver should be represented as an unresolved placeholder.

This behavior directly contradicts the user's goal of focusing analysis on a primary scope and treating dependencies as opaque.

## 3. Proposed Strategy: Ask, Don't Take

The guiding principle for the solution is "Ask, Don't Take." The `Evaluator` must always "ask" the `Resolver` for permission via policy-checking methods before accessing code. It should never "take" information by using unsafe `WithoutPolicyCheck` methods.

This will be achieved through the following concrete changes:

### Step 1: Enforce Policy in Package Loading

The `evaluator.getOrLoadPackage` function will be modified to use the safe, policy-respecting `resolver.ResolvePackage` method.

*   **Current (Unsafe):**
    ```go
    scannedPkg, err := e.resolver.resolvePackageWithoutPolicyCheck(ctx, path)
    ```

*   **Proposed (Safe):**
    ```go
    scannedPkg, err := e.resolver.ResolvePackage(ctx, path)
    ```

**Impact:**
The `resolver.ResolvePackage` function first checks the `ScanPolicy`. If the policy denies access, it immediately returns an error. The `getOrLoadPackage` function will handle this error not as a failure, but as a signal to create a placeholder `object.Package` whose `ScannedInfo` field is `nil`.

### Step 2: Handle Policy-Constrained Packages in Evaluator

With the change in Step 1, the evaluator will now encounter placeholder packages. `evalSelectorExpr` must be updated to handle this.

*   **Current Logic:** Assumes `pkg.ScannedInfo` is always populated and attempts to find symbols within it.
*   **Proposed Logic:** Before accessing `pkg.ScannedInfo`, `evalSelectorExpr` will check if it is `nil`.
    *   If `pkg.ScannedInfo` is `nil`, it means the package is out-of-policy.
    *   The evaluator will immediately create a symbolic placeholder for the requested symbol (e.g., `object.UnresolvedFunction` for `pkg.Symbol`). It will **not** attempt to iterate over function lists or type lists that don't exist.

### Step 3: Enforce Policy in Type Resolution for Method Receivers

The `resolver.ResolveFunction` method will be modified to use the safe `ResolveType` for resolving method receivers.

*   **Current (Unsafe):**
    ```go
    receiverVar.SetTypeInfo(r.resolveTypeWithoutPolicyCheck(context.Background(), funcInfo.Receiver.Type))
    ```
*   **Proposed (Safe):**
    ```go
    receiverVar.SetTypeInfo(r.ResolveType(context.Background(), funcInfo.Receiver.Type))
    ```

**Impact:**
If a method's receiver is from an out-of-policy package, `ResolveType` will return an `UnresolvedTypeInfo` placeholder. The resulting `object.Function` will correctly and accurately reflect that its receiver is of a type that is not being fully analyzed. This is the desired behavior.

## 4. Expected Outcomes

1.  **Strict Policy Adherence:** The `symgo` engine will no longer access source code that is forbidden by its `ScanPolicy`. Its behavior will be strictly aligned with its configuration.
2.  **Improved Performance:** By avoiding the unnecessary parsing of ASTs for out-of-scope dependencies, analysis will be significantly faster, especially in large projects with many dependencies.
3.  **Consistent Use of Placeholders:** The system will consistently use `SymbolicPlaceholder`, `UnresolvedTypeInfo`, and `UnresolvedFunction` objects to represent out-of-scope entities, as per the original design goal. This makes the engine's internal state more predictable and robust.
4.  **Test Adjustments:** Some existing tests may fail, as they might assert on fully-resolved types where the new, correct behavior is to provide an unresolved placeholder. These tests will need to be updated to reflect the stricter, more accurate analysis model. This is a necessary and positive consequence of the change.

## 5. Conclusion

This plan brings the `symgo` implementation back into alignment with its intended design as a focused, scope-aware static analysis engine. By removing all policy bypasses, we strengthen its core principles, improve performance, and create a more robust and predictable tool.

## 6. Impact Analysis and Resolution Strategy

After a series of experiments, a final coordinated fix was implemented to achieve the desired policy enforcement. This involved changes to `resolver.go` and `evaluator.go`.

### 6.1. The Coordinated Fix

1.  **`resolver.ResolveFunction`**: Modified to use the policy-enforcing `ResolveType` for method receivers. This ensures that methods on out-of-policy types are correctly associated with an unresolved receiver type.

2.  **`evaluator.getOrLoadPackage`**: Modified to use the policy-enforcing `resolver.ResolvePackage`. When the policy denies access, this function no longer returns an error but rather a placeholder `object.Package` with `ScannedInfo = nil`.

3.  **`evaluator.evalIdent`**: Modified to correctly handle placeholder packages. For an import without an alias (e.g., `import "fmt"`), if the corresponding package object is a placeholder, it now uses a heuristic—assuming the package name matches the last segment of the import path—to associate the identifier (`fmt`) with the placeholder package. This was the key change to resolve the `identifier not found` errors.

4.  **`evaluator.evalSelectorExpr`**: Modified to handle placeholder packages. When it encounters a selector on a package where `ScannedInfo` is `nil`, it immediately creates an `object.UnresolvedFunction` instead of attempting to re-scan the package. This ensures that the call can be hooked.

5.  **`evaluator.applyFunction`**: Modified to use the policy-enforcing `resolver.ResolvePackage` when attempting to get more information about an `UnresolvedFunction`.

### 6.2. Final Test Results & Analysis

With the coordinated fix in place, `go test -timeout 30s ./symgo/...` was executed.

*   **Build Status:** **SUCCESS**. The build now passes, indicating the heuristic used in `evalIdent` was sufficient to resolve the previous build errors.
*   **`identifier not found` Errors:** **RESOLVED**. Tests like `TestInterpreter_Eval_Simple` no longer fail with `identifier not found: fmt`. They now fail because they receive a placeholder object instead of a concrete one, which is the correct behavior.
*   **Hooking Verification:** The test failures confirm the hooking mechanism works. For example, in `TestInterpreter_RegisterIntrinsic`, the test fails because the result is a `SymbolicPlaceholder` instead of the string from the intrinsic. This shows that `evalCallExpr` correctly received the `UnresolvedFunction` for `fmt.Println`, passed it to the `defaultIntrinsic` hook, and then `applyFunction` correctly returned a placeholder as the result. The test fails only because its success criteria were based on the old, policy-bypassing behavior.

### 6.3. Resolution Strategy

The implementation is now correct and aligns with the design goals. The remaining test failures are not due to bugs in the implementation but rather to outdated assumptions in the tests themselves. The required resolution is to:

1.  **Update Failing Tests:** Modify the assertions in the failing tests. Instead of expecting concrete values from out-of-policy function calls, they should assert that the result is of type `*object.SymbolicPlaceholder` or that the function object is an `*object.UnresolvedFunction`.
2.  **Update Test Tracers:** Test helpers that act as a `defaultIntrinsic` (like the call tracer in `TestSymgo_WithExtraPackages`) must be updated to recognize and correctly handle `*object.UnresolvedFunction` objects passed to them.
3.  **Add Coverage:** Add a new test, as described in the initial analysis, to specifically validate that method receivers on out-of-policy types are correctly resolved to placeholders with `UnresolvedTypeInfo`.

## 7. References

*   [docs/plan-symbolic-execution-like.md](./plan-symbolic-execution-like.md)
*   [docs/analysis-symgo-implementation.md](./analysis-symgo-implementation.md)
*   [examples/find-orphans/spec.md](./../examples/find-orphans/spec.md)

## 8. Appendix: Debugging Log and Hypothesis (2025-09-12)

This section documents the trial-and-error process and evolving hypotheses during the effort to fix test regressions after the strict scan policy was implemented.

### 8.1. Initial State and Problem

- **Initial Change**: The core logic in `symgo/evaluator` was modified to strictly adhere to the `ScanPolicyFunc`. The goal was to stop the evaluator from accessing the ASTs of packages outside the defined analysis scope.
- **Test Regressions**: This change, while correct in principle, caused a significant number of test failures. The failures fell into two main categories:
    1.  **Correct Failures**: Tests that previously relied on the "omniscient" evaluator now correctly failed. They were asserting for fully resolved objects where the new, correct behavior was to return a placeholder (`SymbolicPlaceholder` or `UnresolvedFunction`). These tests needed to be updated.
    2.  **Incorrect Failures (Bugs)**: Tests that *should* have passed, even with the new policy, began to fail. These pointed to deeper issues.

### 8.2. First Hypothesis: Patching Tests is Sufficient

The initial plan was to update all failing tests to align with the new, stricter reality. This involved changing assertions from expecting concrete values (`*object.String`, etc.) to expecting placeholders.

- **Action**: Modified several tests (`TestFeature_SprintfIntrinsic`, `TestSymgo_ExtraModule_ConstantResolution`, `TestIntraModuleCall`, etc.) to expect placeholders.
- **Observation**: While this fixed some tests, it broke others and revealed inconsistencies. For example, after fixing the `applyFunction` logic to correctly handle intrinsics on `UnresolvedFunction`s, tests that were patched to expect placeholders (like `TestFeature_SprintfIntrinsic`) started failing again because they were now correctly receiving concrete values from the intrinsic.
- **Conclusion**: Simply patching the tests was not the right approach. It was hiding underlying bugs and making the test suite brittle. A full reset of the changes was performed to start with a clean slate.

### 8.3. Second Hypothesis: `WithPrimaryAnalysisScope` is Broken

After resetting, the focus shifted to why tests that explicitly defined a wide analysis scope were still failing.

- **Test Case**: `TestSymgo_WithExtraPackages` became the primary focus. Its `with_extra_package` subtest configures the interpreter with `WithPrimaryAnalysisScope("example.com/app/...", "example.com/helper/...")`. This *should* cause the `helper.Greet` function to be fully evaluated.
- **Observation**: The test failed, receiving a `SymbolicPlaceholder` instead of the expected string. This was a clear indication that the `helper` package was not being scanned, despite being included in the primary analysis scope.
- **Debugging Step**: Added logging to the `ScanPolicyFunc` created by `NewInterpreter`.
- **Key Finding**: The logs revealed that the scan policy was being checked for the `fmt` package (and correctly denying it), but it was **never being checked for `example.com/helper`**.

### 8.4. Current Hypothesis: `go.work` and Package Resolution Issue

The fact that the `ScanPolicyFunc` is not even being *called* for the `helper` package points to an issue earlier in the pipeline, before the resolver's policy check is reached.

- **Context**: `TestSymgo_WithExtraPackages` is unique in that it sets up a `go.work` workspace. The `app` and `helper` packages are in different modules within this workspace.
- **Hypothesis**: The problem likely lies in how the `go-scan` `Scanner` or the `symgo` `Resolver` handles package resolution in a multi-module workspace. The `evaluator`'s call to `getOrLoadPackage` for an import (`"example.com/helper"`) might not be correctly triggering the scanner to find and load the package from the workspace. It seems to be getting lost before the `ScanPolicy` is ever consulted.

This is the current line of investigation. The next step is to debug the package loading path within the evaluator, specifically how it interacts with the scanner and resolver when dealing with packages from different modules in a `go.work` environment.
