# Retrospective: Implementing a Deferred Evaluation Policy in `symgo`

## 1. The Premise: The Initial Goal

The original task was to prevent the `symgo` symbolic execution engine from performing deep, eager scans of external packages (like the Go standard library). The desired behavior was for `symgo` to treat types from these packages as opaque placeholders by default. A full scan and evaluation should only occur if a package was explicitly added to a "scan list" (the `extraPackages` option). This would improve performance and avoid errors when encountering complex or platform-specific code in external dependencies.

## 2. The Journey: Outlook, Implementation, and Pivots

### Initial Implementation & First Pivot

-   **Initial Outlook:** A simple boolean check (`shouldScanPackage`) before resolving types seemed sufficient. If a package wasn't on the "scan list," we would not resolve its types, leading to placeholder objects.
-   **Implementation:** I wrapped `Resolve()` calls in `shouldScanPackage` checks.
-   **Result & Pivot:** This immediately broke a key test, `TestInterfaceBinding`, which relied on resolving the `io.Writer` interface. My first pivot was a naive fix: I made `shouldScanPackage` always return `true` for standard library packages. This fixed `TestInterfaceBinding` but was a conceptual step backward. It led to a cascade of new failures in the `examples/docgen` tests, as the evaluator was now trying to *deeply evaluate* the entire `net/http` package and failing on unexported test hooks (`testHookServerServe`). This was the first major obstacle.

### The "Placeholder Explosion" Idea & Second Pivot

-   **User Guidance:** A crucial piece of user feedback guided the next step: "The fundamental problem is that multi-values can't be interpreted, right? ...isn't it simply a matter of changing the placeholder for multi-value use at the time the multi-value is needed?" The user also explicitly stated that my "just-in-time scan" idea (which I had attempted) was a mistake.
-   **New Outlook:** The problem wasn't necessarily resolving the method signature at the call site, but ensuring the assignment worked.
-   **Implementation:** I reverted the JIT scan logic in `applyFunction`. I then modified `evalAssignStmt` to check if a single `SymbolicPlaceholder` was being assigned to multiple variables (e.g., `n, err := ...`). If so, it would "explode" the single placeholder into a `MultiReturn` object containing the correct number of new placeholders.
-   **Result & Pivot:** This was a major step forward. It fixed the `multi-return value` warning and got the core tests passing. However, the `docgen` example was still broken, as it lost all parameter and response information. The "explosion" was a patch for one symptom, but it didn't solve the root cause: the evaluator still had no type information for chained calls like `r.URL.Query().Get("id")`. The information was being lost too early.

### The Final Approach: "Shallow Scans"

-   **Final Outlook:** I realized I needed to distinguish between two concepts:
    1.  **Parsing a package:** Getting access to all its type definitions and function signatures.
    2.  **Evaluating a package:** Symbolically executing the *bodies* of its functions.
    The `extraPackages` option conflated these two. The `testHookServerServe` error happened because of deep evaluation, but the `docgen` information loss happened because of a lack of parsed signatures. The solution was to have a policy for "parse, but don't evaluate."
-   **Implementation:**
    1.  I introduced a new `WithShallowScanPackages` option.
    2.  I refactored the internal policy check from a boolean (`shouldScanPackage`) to an enum (`getScanPolicy` returning `ScanDeep`, `ScanShallow`, `ScanNone`).
    3.  I modified `evalSelectorExpr` to use this policy. For `ScanDeep` packages, it creates a full `*object.Function` with a body to be evaluated. For `ScanShallow` packages, it creates a `*object.SymbolicPlaceholder` enriched with the function's signature (`UnderlyingFunc`), which prevents the body from being evaluated but gives `applyFunction` the signature information it needs.
-   **Result:** This solution worked. It provided the `docgen` example with the necessary signatures from `net/http`, `net/url`, etc., allowing it to generate a complete OpenAPI spec, while preventing the evaluator from going too deep and hitting errors. All tests passed.

## 3. Foreseeable Obstacles: What I Should Have Known

1.  **Conflating Parsing and Evaluation:** The core obstacle was treating "scanning a package" as a single, monolithic operation. I should have anticipated earlier that we might need the *declarations* from a package without wanting its *implementation*. Separating these two concerns was the key to the final solution.
2.  **The Nature of `go-scan`:** I initially assumed the underlying scanner used `go/types`, which would have provided full method sets for all imported packages automatically. I spent time on a solution based on this wrong assumption. A preliminary check of the `locator` and `scanner` implementation would have revealed it was a pure AST-based parser, which prevented some avenues for resolution.
3.  **Interpreting Test Failures:** The `testHookServerServe` error was a critical clue that deep evaluation of the standard library was dangerous. I correctly identified this but pivoted too hard towards the "placeholder explosion" idea instead of trying to refine the "scan for signatures" idea. I should have trusted my initial instinct that signature information was necessary and focused on how to get it *safely*.
