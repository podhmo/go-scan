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

### Obstacle 3: Regressions in Downstream Tools (`docgen`)

**Problem:** After implementing the core feature and fixing the initial tests, I discovered that tests for the `docgen` example were now failing with panics and strange errors (`undefined method: Addr on symbolic type net/http.Server`).

**Analysis:** My initial reaction was that the panic was a bug in the evaluator. I even added a defensive `nil` check. However, the root cause was more subtle. The `docgen` tool needs to analyze code that uses `net/http`. To "help" it, my first attempt at a `ScanPolicy` for its tests was too broad: I told it to scan *any* standard library package. This was a mistake.

This overly permissive policy caused `symgo` to abandon its use of intrinsics (which are designed to handle stdlib functions without deep evaluation) and instead try to symbolically execute the source code of `net/http`, which it is not equipped to do. This led to the `s.Addr` error (trying to access a field on a symbolic server) and the panic (evaluating unexpected AST nodes).

**Solution:** The correct fix was to make the `docgen` scan policy *stricter*. It should only scan the user's own code. The standard library functions (`http.ListenAndServe`, `http.HandleFunc`, etc.) should **not** be scanned, forcing `symgo` to rely on the registered intrinsics, which is the intended design for `docgen`. I also had to add a new intrinsic for `http.ListenAndServe` to prevent the evaluator from trying to step into it at all.

**Lesson:** When dealing with a symbolic execution engine, "more scanning" is not always better. The engine's stability can rely on a careful balance between deep scanning of user code and using intrinsics or placeholders for external dependencies (especially the standard library). A regression might be caused by a policy that is too permissive, not too restrictive. Always consider the intended design of the tool using the engine.

### Obstacle 4: Inconsistent Policy Enforcement & Evaluator Halting

**Problem:** After the initial `ScanPolicyFunc` was implemented, tools like `find-orphans` were still exhibiting overly aggressive scanning behavior, and in some cases, the analysis would halt with an error. The policy was being correctly applied for resolving top-level functions from external packages, but it was not being respected in all code paths.

**Analysis:** A deep dive into the `symgo/evaluator` revealed that the root cause was not a single bug, but a combination of two related issues:
1.  **Inconsistent Policy Checks:** While the policy was checked when resolving a package-level function (e.g., `pkg.MyFunc()`), it was *not* checked during method resolution (e.g., `myVar.MyMethod()`). Helper functions like `findMethodOnType` would unconditionally try to scan the package of the receiver's type, bypassing the policy.
2.  **Evaluator Brittleness:** When the policy *did* work correctly (preventing a scan), the evaluator didn't handle the consequences gracefully. For example, if `pkg.NewThing()` returned a type from a non-scanned package, the resulting symbolic variable would have no `TypeInfo`. A subsequent call like `myThing.DoSomething()` would cause the evaluator to panic with a "cannot access field or method on variable with no type info" error, halting the entire analysis. A robust tool should treat this as an unknown and simply continue.

**Solution:** The problem was solved with a two-part fix in `symgo/evaluator/evaluator.go`:
1.  **Fixing Brittleness (Primary Fix):** The evaluator was modified to no longer error when a method is called on a variable with no `TypeInfo`. Instead of halting, it now logs a debug message and returns a generic symbolic placeholder. This allows the analysis to continue gracefully, treating the unknown method call as an opaque operation, which is the correct behavior for out-of-scope code.
2.  **Adding Missing Policy Checks (Defense-in-Depth):** Although the primary fix resolved the halting issue, the inconsistent policy checks were also fixed for correctness and efficiency. The policy check was added to `findDirectMethodOnType` and other internal functions to prevent them from attempting to scan packages that the policy disallows.

This layered approach ensures that the evaluator is both efficient (by not attempting disallowed scans) and robust (by not halting when it encounters the inevitable consequences of adhering to the policy).

**Lesson:** A security or policy mechanism is only as strong as its most permissive code path. It's crucial to ensure that policies are checked at *all* relevant boundaries. Furthermore, when a policy correctly blocks an action, the calling code must be robust enough to handle the resulting lack of information, rather than crashing.
