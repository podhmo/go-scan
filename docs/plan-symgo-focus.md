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

A series of experiments were conducted by applying the proposed changes individually and running the test suite (`go test -timeout 30s ./symgo/...`) to observe the specific impact of each change.

### 6.1. Impact of Modifying `evaluator.getOrLoadPackage`

*   **Change:** Modified `getOrLoadPackage` to use the policy-enforcing `resolver.ResolvePackage`.
*   **Observation:** This change alone caused multiple test failures, such as `TestInterpreter_Eval_Simple`, with the error `identifier not found: fmt`.
*   **Analysis:** This is the expected and desired first-order effect. The tests attempt to evaluate code that uses the `fmt` package. Because the default test policy does not include `fmt`, `ResolvePackage` correctly denies access. The `getOrLoadPackage` function then correctly creates a placeholder `object.Package` with no scanned information. However, the existing `evalIdent` and `evalSelectorExpr` functions are not robust enough to handle this case and ultimately fail to resolve the `fmt` identifier, leading to the error.
*   **Conclusion:** This confirms that changing `getOrLoadPackage` is the correct first step, and it successfully exposes the downstream dependencies on policy-bypassing behavior.

### 6.2. Impact of Modifying `evalSelectorExpr` and `applyFunction`

*   **Change:** Modified `evalSelectorExpr` and `applyFunction` to use `resolver.ResolvePackage` when they encounter a package that has not yet been loaded.
*   **Observation:** This change caused tests like `TestFeature_SprintfIntrinsic` and `TestSymgo_WithExtraPackages` to fail. The common failure mode was expecting a concrete value (e.g., a string from `Sprintf`) but receiving a `SymbolicPlaceholder`.
*   **Analysis:** This is also the correct behavior. These functions were previously bypassing the policy by calling `resolvePackageWithoutPolicyCheck`. By switching to the policy-enforcing `ResolvePackage`, calls to out-of-policy functions like `fmt.Sprintf` are no longer executed via their intrinsic. Instead, they are correctly identified as calls to an unresolved function, and the result is a symbolic placeholder. The tests failed because they were written with the assumption that the policy would be bypassed.
*   **Resolution:** The failing tests need to be updated. Instead of asserting for a concrete return value from an out-of-policy intrinsic, they should assert that the result is an `object.SymbolicPlaceholder` or that the function call was to an `object.UnresolvedFunction`.

### 6.3. Impact of Modifying `resolver.ResolveFunction`

*   **Change:** Modified `ResolveFunction` to use the policy-enforcing `resolver.ResolveType` for method receivers.
*   **Observation:** This change caused **no test failures**.
*   **Analysis:** This indicates a gap in the current test suite. There are no tests that specifically exercise the scenario of symbolically analyzing a method call where the receiver's type is defined in an out-of-policy package. While the code change is correct and crucial for security and design alignment, its effect is not currently asserted by any test.
*   **Resolution:** A new test should be written to validate this behavior. The test should:
    1.  Define a package `a` with a struct `T` and a method `M`.
    2.  Define a main analysis package `b` that imports `a`.
    3.  Set a `ScanPolicy` that includes `b` but **excludes** `a`.
    4.  In `b`, call the method `t.M()` on a variable `t` of type `a.T`.
    5.  Assert that the `object.Function` resolved for `M` has a `Receiver` whose `TypeInfo` is an `UnresolvedTypeInfo` for `a.T`.
