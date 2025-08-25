# Post-mortem: Refining the `find-orphans` Tool

This document details the thought process, obstacles, and strategic pivots during the task of refining the `find-orphans` tool. It serves as a guide for future work on this or similar components.

## 1. Initial Goal

The initial request was to investigate and fix an issue in the `find-orphans` tool where it would report unused functions (orphans) from packages other than those specified by the user. For example, when analyzing package `A` which depends on package `B`, orphans from `B` were being reported, which was not the desired behavior. The user wanted the tool's output to be strictly scoped to the packages matching the input patterns.

## 2. Initial Plan & Estimation

My initial analysis concluded that this was not a bug, but a design choice. The tool performed a whole-program analysis on the entire dependency graph of the input packages.

The plan was straightforward:
1.  Resolve the user-provided patterns (e.g., `./...`) into a definitive set of "target" package import paths.
2.  Store this set of target paths.
3.  Perform the existing dependency traversal and symbolic execution as before.
4.  In the final reporting stage, filter the list of orphans, only showing those whose package path was in the initial target set.

I estimated this to be a relatively simple change, primarily contained within `examples/find-orphans/main.go`, with a minor change to `modulewalker.go` to expose a pattern resolution function.

## 3. Implementation Journey & Unforeseen Obstacles

The seemingly simple task spiraled into a multi-day effort involving a deep refactoring of the core library.

### Follow-up Requests
Shortly after the initial plan was made, two follow-up requests were added:
1.  **Exclude `testdata` directories:** Similar to `vendor`, these should be ignored during scans.
2.  **Support relative paths:** Patterns like `../../` should be correctly resolved from the current working directory.

These requests were incorporated into the plan.

### Obstacle 1: The Widespread Test Failures & The Flawed Assumption

After implementing the primary logic (filtering orphans by target package), I ran the test suite. Nearly all tests for `find-orphans` failed.

*   **Initial Thought:** My new filtering logic must be buggy.
*   **The Pivot:** After careful analysis of the test failures, I realized the opposite was true. My logic was *correct* according to the new requirements. The tests, however, were written with the expectation of the *old* behavior. For example, a test would specify `example.com/a` as a pattern and expect orphans from its dependency `example.com/b` to be reported. My new code correctly filtered these out, causing the test to fail.
*   **What I Should Have Known:** A change in a tool's fundamental behavior, even if it's a "fix", will invalidate tests that depend on that behavior. I should have anticipated the need to update the tests to align with the new, stricter scoping. The fix was to update the tests to use wildcard patterns (e.g., `example.com/a/...`) to explicitly state that they intended to analyze the full dependency tree.

### Obstacle 2: The Deeper Issue - Path Resolution in Workspaces

Even after fixing the tests, a new, more complex category of failures emerged, specifically in multi-module workspace tests. The logs showed errors like `path "..." does not belong to any module in the workspace`.

*   **Initial Thought:** My path-joining logic for relative paths (`../../`) must be wrong. I tried several iterations of fixing this in `modulewalker.go`, but the problem persisted.
*   **The Pivot:** The true root cause was much deeper. The `locator.PathToImport` method, which converts a file path to an import path, was inherently non-workspace-aware. It was tied to a single module's context (`go.mod` file). When scanning a workspace with multiple modules, the walker would find a file (e.g., `/tmp/workspace/moduleB/lib/lib.go`) but the primary locator (for `moduleA`) wouldn't know how to convert it to an import path.
*   **What I Should Have Known:** The distinction between the `goscan.Scanner` (which should be workspace-aware) and the `locator.Locator` (which is module-specific) is critical. Core functionalities like resolving a file path within a workspace must be handled at the `Scanner` level, which can iterate over all its configured `locators` to find the correct one. My initial refactoring attempts were all at the `ModuleWalker` level, which was the wrong place to fix a workspace-level problem.

## 4. Final Refactoring Strategy

The discovery of the workspace resolution issue prompted a major pivot from a small fix to a larger refactoring:

1.  **Promote `PathToImport`:** I moved the responsibility for path-to-import-path resolution from the module-specific `locator.Locator` to the workspace-aware `goscan.Scanner`. The new `Scanner.PathToImport` iterates through all modules in the workspace to find the correct one for a given file path.
2.  **Decouple `ModuleWalker`:** I made the `ModuleWalker` more of a pure, configurable component.
    *   It now holds a reference back to its parent `goscan.Scanner` (`goscanner` field) to access workspace-aware methods like the new `PathToImport`.
    *   Directory exclusion became a configurable `ExcludeDirs` slice, passed in during construction, rather than being hardcoded. This resolved the conflict between `find-orphans` needing to exclude `testdata` and the core library tests needing to include it.
3.  **Update the Client (`find-orphans`):** The `find-orphans` tool was updated to use the new `WithWalkerExcludeDirs` scanner option, making the exclusion behavior explicit and configurable via a command-line flag.

## 5. Current Status & Next Steps

As of writing this document, the major refactoring is complete. However, the `make test` command is still failing with build errors due to minor mistakes made during the refactoring (e.g., using the wrong field name when initializing a struct).

My immediate next steps are:
1.  Fix the remaining build errors, which appear to be simple typos or naming mistakes.
2.  Run `make test` again. I am now confident that with the corrected architecture, the tests should pass.
3.  Once all tests pass, the original task will be complete.
4.  The final step will be to update `TODO.md` and submit the changes.

---

## 6. Remaining Challenges & High-Level Plan

Based on the analysis of the test failures, the path to completion requires solving three core challenges.

1.  **Challenge: Inconsistent Directory Exclusion**
    *   **Problem:** The logic for skipping directories like `testdata` is applied inconsistently. It's present in some directory walks (e.g., `discoverModules`) but absent in the main pattern resolver (`resolvePatternsToImportPaths`). This leads to unexpected behavior where `testdata` is sometimes included in the analysis.
    *   **Goal:** Centralize and make the exclusion logic configurable and consistent across all directory-walking operations in the library. This will likely involve adding a configuration option to `ModuleWalker` that can be set by client code like `find-orphans`.

2.  **Challenge: Non-Workspace-Aware Path Resolution**
    *   **Problem:** The current mechanism for converting a file path to a Go import path (`locator.PathToImport`) only works for a single module. In a multi-module workspace, it fails to resolve paths for any module other than the "primary" one. This is the root cause of the multi-module test failures.
    *   **Goal:** Implement a workspace-aware path resolution mechanism. This function should live at the `Scanner` level, which has access to all modules in the workspace, and it should iterate through them to find the correct module for any given file path.

3.  **Challenge: Ambiguous File Path Pattern Handling**
    *   **Problem:** The logic for handling file path patterns without a wildcard (e.g., `.` or `..`) is brittle. It incorrectly assumes such a path points to a single package and tries to convert it directly to an import path. This fails when the path points to a directory containing multiple packages or modules (like a workspace root).
    *   **Goal:** Refactor the pattern resolver (`resolvePatternsToImportPaths`) to robustly handle all file path patterns. When a file path pattern is encountered, it should be walked to discover all constituent Go packages within that directory, similar to how wildcard patterns (`...`) are handled.
