# Analysis of Scan Policy Bypasses in `symgo`

## Introduction

This document analyzes the locations in the `symgo` evaluator where scan policy checks are intentionally bypassed. The `symgo.evaluator.Resolver` is designed with a clear convention: exported methods perform policy checks, while unexported methods (or methods explicitly named with `WithoutPolicyCheck`) can bypass them.

This analysis details each call site that uses a policy-bypassing method, explains the rationale for doing so, and discusses potential future strategies that might allow for stricter policy enforcement.

---

## `resolvePackageWithoutPolicyCheck`

This method wraps `scanner.ScanPackageByImport` and does not check the scan policy. It is used for on-demand loading of package information that is essential for the evaluator to function, even for packages outside the primary analysis scope.

### 1. Location: `evaluator.go` -> `getOrLoadPackage`

-   **Context:** This function is the evaluator's primary package cache and loading mechanism. It's called from `evalIdent` when the evaluator encounters an identifier that it doesn't recognize, which could be a package name (e.g., `fmt` in `fmt.Println`).
-   **Reason for Bypassing Policy:** This is the core mechanism for on-demand, lazy loading of any package encountered during evaluation. An attempt was made to replace this call with the policy-checking `ResolvePackage`. This caused multiple test failures (e.g., `TestMismatchImportPackageName_OutOfPolicy`, `TestInterpreter_Eval_Simple`). These tests expect to be able to analyze or create placeholders for symbols from packages like `fmt` or other test-specific external libraries, even if they are not defined in the scan policy. Applying a strict policy check at the point of loading prevents the package from being loaded at all, leading to "identifier not found" errors and halting analysis prematurely.
-   **Potential Future Solutions:** The dependency on this behavior could be reduced if tests that need external packages explicitly declare them in their `ScanPolicyFunc` setup. More fundamentally, the evaluator could be enhanced to distinguish between "deep scanning" for in-policy packages and a "declarations-only scan" for all out-of-policy packages encountered on-demand. This would provide necessary type information for symbolic analysis without fully executing the bodies of external functions.

### 2. Location: `evaluator.go` -> `evalSelectorExpr` (case `*object.Package`)

-   **Context:** This is called when the evaluator encounters a selector expression on a package object (e.g., `pkg.MyFunc`) and finds that the package's `ScannedInfo` has not yet been loaded.
-   **Reason for Bypassing Policy:** This is another entry point for the on-demand loading described above. The reasons for bypassing the policy are identical to those for `getOrLoadPackage`. Tests failed when this was changed to the policy-checking `ResolvePackage`.
-   **Potential Future Solutions:** The solutions are the same as for `getOrLoadPackage`.

### 3. Location: `evaluator.go` -> `evalSelectorExpr` (case `*object.Variable`)

-   **Context:** This is called when resolving method calls on interface types. If a variable has an interface type defined in an external, not-yet-scanned package (e.g., `io.Reader`), the evaluator needs to load that package (`io`) to get the interface's method set.
-   **Reason for Bypassing Policy:** This is fundamental for performing method resolution on interfaces from external dependencies. If this call were blocked by a policy check, the evaluator would be unable to determine the methods of any interface from an out-of-policy package, rendering much of the symbolic execution useless.
-   **Potential Future Solutions:** This is a core requirement for the analysis engine. A possible future enhancement could be to use the `WithDeclarationsOnlyPackages` option for any package loaded transitively this way. This would provide the necessary type and method information without the overhead and potential complexity of a full symbolic execution of the external package's implementation.

---

## `resolveTypeWithoutPolicyCheck`

This method resolves a `scanner.FieldType` to a `scanner.TypeInfo` without checking the scan policy.

### 1. Location: `evaluator.go` -> `assignIdentifier`

-   **Context:** This call is used when an assignment occurs (e.g., `x := ...` or `x = ...`). The code checks if the static type of the variable `x` is an interface.
-   **Reason for Bypassing Policy:** The purpose is to determine the "true" kind of a type, even if it comes from an out-of-policy package. The policy-checking `ResolveType` would return a `TypeInfo` with `Unresolved: true`, which would not have its `Kind` field properly set to `scanner.InterfaceKind`. By bypassing the policy, the evaluator can correctly identify that a variable is of an interface type and enable the logic for tracking its possible concrete implementations. An attempt to replace this with `ResolveType` passed the existing tests, but this is likely due to a lack of test coverage for this specific scenario involving out-of-policy interfaces. The bypass is considered correct for robust type analysis.
-   **Potential Future Solutions:** This could be addressed by a more sophisticated `TypeInfo` struct that can distinguish between an unresolved type that is known to be an interface and one that is not.

### 2. Location: `evaluator.go` -> `applyFunction`

-   **Context:** This is used when creating symbolic placeholders for the return values of an external (out-of-policy) function call.
-   **Reason for Bypassing Policy:** The surrounding code block *already performs a manual policy check*. The call to `resolveTypeWithoutPolicyCheck` is in the `else` branch of an `if !r.ScanPolicy(...)` check. Therefore, the policy has already been verified before this method is called. Using the policy-bypassing method here is correct and avoids a redundant check.
-   **Potential Future Solutions:** No change is needed; this implementation is correct. The code could potentially be refactored to use `ResolveType`, but it would be functionally identical.
