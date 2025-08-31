# Analysis of Scan Policy Bypasses in `symgo`

## Introduction

This document analyzes the locations in the `symgo` evaluator where scan policy checks are intentionally bypassed. The `symgo.evaluator.Resolver` is designed with a clear convention: exported methods perform policy checks, while unexported methods (or methods explicitly named with `WithoutPolicyCheck`) can bypass them.

A key design principle, illustrated by tools like `find-orphans` (see `examples/find-orphans/spec.md`), is the distinction between code within the **analysis scope** and code outside of it (e.g., standard libraries, third-party dependencies). The `ScanPolicyFunc` defines this boundary.

Code outside the analysis scope should not be deeply executed. Instead, the `symgo` engine should be able to create **symbolic placeholders** for calls to out-of-scope functions. To do this, the engine must still be able to load basic package information (like function signatures and type definitions) from these external packages.

Bypassing the policy at certain key points is therefore not a flaw, but a **core feature** that enables this shallow, symbolic analysis of external code.

---

## `resolvePackageWithoutPolicyCheck`

This method wraps `scanner.ScanPackageByImport` and does not check the scan policy. It is the fundamental mechanism for on-demand loading of package information required for symbolic analysis to proceed across package boundaries.

### 1. Locations:
- `evaluator.go` -> `getOrLoadPackage`
- `evaluator.go` -> `evalSelectorExpr` (case `*object.Package` and case `*object.Variable`)

-   **Context:** These functions represent the various entry points for on-demand, lazy loading of a package when the evaluator encounters a symbol it has not seen before. This happens when resolving a package name (e.g., `fmt`), accessing a symbol in a package for the first time (e.g., `pkg.MyFunc`), or resolving a type from a transitive dependency (e.g., an interface method).

-   **Reason for Bypassing Policy:** To create a symbolic placeholder for a call to an out-of-scope function (e.g., `fmt.Println`), the evaluator first needs to load the `fmt` package to confirm that `Println` exists and to get its signature. If the policy check were strictly enforced at this loading stage, the `fmt` package would be rejected entirely, and the evaluator would fail with an "identifier not found" error instead of continuing the analysis by creating a placeholder. Bypassing the policy here allows the package to be loaded. The `ScanPolicy` is then used later to determine the *depth* of analysis: in-policy packages are executed deeply, while out-of-policy packages are shallowly analyzed (i.e., calls to them result in placeholders).

-   **Potential Future Solutions:** The current design is correct and necessary for shallow scanning. A future enhancement could involve making this behavior more explicit. For example, instead of just bypassing the policy, the resolver could have a method like `ResolvePackageForSymbolicAnalysis`, which always loads the package but returns metadata indicating whether it's in-policy or out-of-policy. This would make the evaluator's subsequent decision to create placeholders even more explicit.

---

## `resolveTypeWithoutPolicyCheck`

This method resolves a `scanner.FieldType` to a `scanner.TypeInfo` without checking the scan policy.

### 1. Location: `evaluator.go` -> `assignIdentifier`

-   **Context:** This is called during a variable assignment to check if the variable's static type is an interface.
-   **Reason for Bypassing Policy:** Similar to package loading, the evaluator needs to know the true "kind" of a type (is it a struct, an interface, etc.) even if it's from an out-of-policy package. The policy-checking `ResolveType` would return a generic `Unresolved` placeholder, which loses this crucial information. By bypassing the policy, the evaluator can get the detailed `TypeInfo`, see that its `Kind` is `scanner.InterfaceKind`, and correctly enable the logic for tracking concrete type implementations for that interface variable. This is essential for correct symbolic execution of interface-based logic.
-   **Potential Future Solutions:** The current implementation is correct. A more advanced type system could perhaps embed the `Kind` information within the `Unresolved` type placeholder itself, but the current approach of bypassing the policy to get the full `TypeInfo` is a valid and functional design.

### 2. Location: `evaluator.go` -> `applyFunction`

-   **Context:** This is used when creating symbolic placeholders for the return values of an external (out-of-policy) function call.
-   **Reason for Bypassing Policy:** The surrounding code block **already performs a manual policy check** before this call is made (`if !e.resolver.ScanPolicy(...)`). This call exists in the `else` branch, meaning the policy has already been checked and has passed. Using the policy-bypassing method here is simply a correct optimization to avoid a redundant check.
-   **Potential Future Solutions:** No change is needed; this implementation is correct and efficient.
