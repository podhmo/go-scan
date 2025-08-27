# Troubleshooting: Introducing a Scan Policy to `symgo`

This document outlines the process, challenges, and decisions made while refactoring `symgo` to use a configurable scanning policy. It's intended as a guide for future agents undertaking similar tasks.

## 1. The Goal

The primary objective was to prevent the `symgo` symbolic execution engine from eagerly scanning and evaluating Go packages outside of a user-defined scope. The original behavior caused `symgo` to analyze the source code of any encountered package (including the entire standard library and third-party dependencies) to resolve types. This was inefficient and would not scale on projects with large dependency trees.

The core requirements were:
- `symgo` should, by default, only perform deep analysis on packages within the user's immediate workspace.
- The mechanism for controlling this behavior should be flexible and dynamic.
- Tools like `find-orphans` (which operate on a well-defined workspace) should continue to function correctly.
- The `symgo` engine should still be able to interact with symbols from external packages, but by treating them as opaque placeholders rather than by analyzing their source code.

## 2. Initial Plan & A Critical Pivot

My initial plan was to replace the existing `WithExtraPackages([]string)` option with a new `WithWorkspacePackages(map[string]bool)` option. This would act as a static whitelist, where the interpreter would only scan packages whose import paths were present in the provided map.

**Pivot:** The user immediately provided feedback that a static whitelist was insufficient and that they required a more dynamic way to control the policy. This was a crucial course correction. I revised the plan to use a callback function, `ScanPolicyFunc`, defined as `func(importPath string) bool`. This approach was approved and formed the basis of the final implementation. It allows the user to implement any logic they need to decide which packages to scan at runtime.

## 3. Implementation Journey & Obstacles

The implementation process revealed several obstacles and required fixes beyond the initial scope.

### Obstacle 1: Go Import Cycle

**Problem:** My first implementation defined `ScanPolicyFunc` in the `symgo` package. The `symgo.Interpreter` then passed this function to the `symgo/evaluator.Evaluator`. However, the `evaluator` now needed to import the `symgo` package to use the `ScanPolicyFunc` type, while `symgo` already imported `evaluator`. This created a classic Go import cycle.

**Solution:** Shared types between two packages that depend on each other must be moved to a third, lower-level package that both can import without creating a cycle. I moved `ScanPolicyFunc` to the `symgo/object` package, which already contained shared object definitions that both `symgo` and `evaluator` depended on. This resolved the cycle.

**Lesson:** When refactoring Go packages, always be mindful of dependency direction. Introducing a new type that needs to be shared between a package and its subpackage is a common cause of import cycles. The solution is to move the shared type to a common, lower-level dependency.

### Obstacle 2: Incorrect API Assumption & Default Policy

**Problem:** The user requested that the default policy should be to scan packages within the same module. To implement this, I assumed the `goscan.Scanner` object (which `symgo.Interpreter` holds) had a method to list all modules in the current workspace (e.g., `scanner.Modules()`). This led to a compile error: `undefined method: Modules`.

**Solution:** I had to investigate the `go-scan` codebase.
1. I discovered that the top-level `goscan.Scanner` is the workspace-aware object, holding a list of `*locator.Locator`s (one for each module), but it did *not* expose this list publicly.
2. The lower-level `scanner.Scanner` is for a single module.
3. To correctly implement the default policy, I had to add a new public method, `Modules() []*scanner.ModuleInfo`, to the `goscan.Scanner`. This required first defining a new `scanner.ModuleInfo` struct in `scanner/models.go` and then implementing the `Modules()` method in `goscan.go` to iterate over its internal locators and return their module information.

**Lesson:** Do not assume an API exists. When a compile error indicates a missing method, verify the public API of the type in question by reading its source code. If the required data is available internally but not exposed, a necessary part of the task may be to add a new public method to expose it.

### Obstacle 3: Evaluator Brittleness and Test Regressions

**Problem:** The initial implementation of the `ScanPolicyFunc` was correctly preventing `symgo` from scanning out-of-scope packages at the top level. However, this introduced a new class of failures in downstream tools and tests. Specifically, when the evaluator encountered a variable whose type was from a non-scanned package (e.g., the result of a function call into a dependency), it would later fail with a `cannot access field or method on variable with no type info` error when a method was called on that variable. This was a critical issue because the goal was to *ignore* out-of-scope code, not to crash because of it.

**Analysis:** This revealed a flaw in the evaluator's design. It was not robust enough to handle the consequences of its own scanning policy. It assumed that if it had a variable, it could always resolve its type information, which was no longer true. A long series of attempts to fix this by adding more policy checks around type resolution calls (`.Resolve()`) proved to be a dead end, as it caused cascading failures in other tests (`docgen`) which had different scanning requirements.

**Solution:** After a user hint, the correct, two-part solution was identified:
1.  **Make the Evaluator Robust:** The primary fix was to change the behavior of `evalSelectorExpr` in `symgo/evaluator/evaluator.go`. When it encounters a method call on a variable with no `TypeInfo`, it no longer returns an error. Instead, it logs a debug message and returns a `SymbolicPlaceholder`. This allows the analysis to continue gracefully, correctly treating the method call as an opaque, un-analyzable operation, which is the desired behavior for tools like `find-orphans`.
2.  **Fix Client Configuration:** The secondary failures in tools like `docgen` were not due to a bug in the evaluator, but a bug in the *test configuration for the tool*. The `docgen` tool, by its nature, *needs* to analyze the `net/http` package. The tests were failing because they were not explicitly telling `docgen`'s scan policy to allow this. The fix was to update the `docgen` tests to pass `[]string{"net/http"}` as an `extraPkgs` parameter, and to update the main `docgen` command to include this by default.

**Lesson:** When a core change (like enforcing a policy) causes regressions, the problem may not be in the core change itself, but in the assumptions made by downstream clients. Instead of making the core logic overly complex to satisfy all clients, it's often better to make the core logic robust to expected failure conditions (like missing type info) and then update the clients to provide the correct configuration for their specific needs.
