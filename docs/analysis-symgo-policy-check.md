# Critical Analysis of Scan Policy Bypasses in `symgo`

## Introduction

This document provides a critical analysis of locations in the `symgo` evaluator where scan policy checks are intentionally bypassed. The goal is to move towards a design where policy checks are enforced consistently, and any exceptions are handled through explicit, robust architectural patterns rather than through policy bypasses.

The current design convention is that unexported methods or those named with a `...WithoutPolicyCheck` suffix can bypass the `ScanPolicyFunc`. While this enables key features for tools like `find-orphans`, it can be considered a "code smell" because it makes the policy's behavior less predictable and the evaluator's logic harder to reason about.

This analysis details each bypass, critiques the current implementation, and proposes more robust future solutions.

---

## `resolvePackageWithoutPolicyCheck`

This method bypasses the `ScanPolicyFunc` to load package information on-demand.

-   **Locations:**
    -   `evaluator.go` -> `getOrLoadPackage`
    -   `evaluator.go` -> `evalSelectorExpr`

-   **Critique of Current Design:** The current on-demand loading mechanism is an implicit or "magic" feature. A user might define a strict `ScanPolicyFunc` expecting *only* the specified packages to be parsed, but the evaluator will still attempt to scan any other package it encounters (e.g., `fmt`). This violates the principle of least privilege and weakens the guarantee provided by the policy. It conflates the concepts of "packages to be deeply analyzed" and "packages required for transitive type resolution."

-   **Proposed Future Solution:** A more robust architecture would eliminate on-demand loading in favor of an explicit declaration of all required analysis scopes. The `goscan.Scanner` could be configured with two distinct scopes:
    1.  **`PrimaryAnalysisScope`**: A set of packages to be deeply, symbolically executed.
    2.  **`SymbolicDependencyScope`**: A set of packages for which only declarations should be parsed to enable symbolic placeholder creation. This scope could be automatically populated by analyzing the imports of the primary scope.

    With this design, any attempt to load a package not in either scope would result in a hard error, making the analysis hermetic and predictable. The `ScanPolicyFunc` would evolve into a function that determines which scope a package belongs to. This would completely remove the need for `resolvePackageWithoutPolicyCheck`.

---

## `resolveTypeWithoutPolicyCheck`

This method resolves a `scanner.FieldType` to a `scanner.TypeInfo` without checking the scan policy.

### 1. Location: `evaluator.go` -> `assignIdentifier`

-   **Critique of Current Design:** This bypass is used to determine if a variable's type is an interface, even if the type is from an out-of-policy package. The policy-checking `ResolveType` would return a generic `Unresolved` placeholder, obscuring the type's true kind. This is brittle; it forces the caller to violate the policy abstraction to get a single piece of information. If more information were needed in the future, it would encourage adding more policy bypasses.

-   **Proposed Future Solution:** The `scanner.TypeInfo` struct for unresolved types should be made more expressive. Instead of being an opaque placeholder, it should retain critical information that can be determined without a deep scan.
    ```go
    // A potential future enhancement
    type TypeInfo struct {
        // ...
        Unresolved    bool
        UnresolvedKind scanner.Kind // e.g., InterfaceKind, StructKind
        // ...
    }
    ```
    This would allow the policy-checking `ResolveType` to return a placeholder that still contains the necessary kind information, making the `resolveTypeWithoutPolicyCheck` bypass unnecessary for this use case.

### 2. Location: `evaluator.go` -> `applyFunction`

-   **Critique of Current Design:** The code manually performs a policy check and then calls the policy-bypassing `resolveTypeWithoutPolicyCheck` in the `else` block. While functionally correct, this pattern is confusing and violates the convention that policy logic should be centralized within the `Resolver`. A developer reading the code has to analyze the surrounding `if/else` block to understand that the policy is, in fact, being honored.

-   **Proposed Future Solution:** For clarity, consistency, and strict adherence to the design convention, this code should be refactored to simply call the policy-checking `ResolveType` method.
    ```go
    // OLD
    if !e.resolver.ScanPolicy(...) {
        resolvedType = scanner.NewUnresolvedTypeInfo(...)
    } else {
        resolvedType = e.resolver.resolveTypeWithoutPolicyCheck(...)
    }

    // PROPOSED
    resolvedType = e.resolver.ResolveType(ctx, fieldType)
    ```
    The minimal performance cost of a potentially redundant check inside `ResolveType` is a small price to pay for the significant improvement in code readability and architectural consistency.
