# Problem: `symgo` Fails on Imports Where Package Name Mismatches Path

**Status: Partially Resolved**

## Summary

The `symgo` symbolic execution engine had a flawed mechanism for resolving the package name of an import. It naively assumed the package name is the last segment of the import path. This failed for packages like `gopkg.in/yaml.v2`, where the import path is `gopkg.in/yaml.v2` but the declared package name is `yaml`.

Additionally, when `symgo` operated on code outside its scan policy, it would error on undefined identifiers, halting analysis. The desired behavior was to create a symbolic placeholder to allow analysis to continue.

## Resolution (Partial)

The issue was partially resolved through a series of fixes to the `symgo` evaluator:

1.  **Lazy, Correct Import Resolution**: The old, incorrect import handling logic was removed. The `evalIdent` function in the evaluator was enhanced to handle package resolution lazily by scanning packages on-demand to determine their *actual* package name.
2.  **Resilience for Out-of-Policy Code**: The `evalIdent` function was modified to return a `SymbolicPlaceholder` instead of an error when an undefined identifier is found in an out-of-policy package. `evalSelectorExpr` was also updated to handle method calls on these new typeless placeholders.

These changes make the `symgo` engine more robust. However, testing revealed follow-on issues that remain unresolved.

---

## Unresolved Issues and Next Steps

1.  **Fix Built-in Type Resolution**:
    *   **Problem**: The tests revealed that after fixing the `yaml` import, the evaluation fails on a built-in type conversion: `string(b)`. The evaluator reports `identifier not found: string`.
    *   **Task**: Enhance `evalIdent` to correctly resolve built-in types (`string`, `int`, `bool`, etc.), likely by checking against a universe of built-in type names.

2.  **Finalize Test Assertions**: The tests in `symgo_mismatch_import_test.go` should be cleaned up and their assertions finalized once the built-in type resolution bug is fixed. The dependency on `gopkg.in/yaml.v2` was removed from the main `go.mod` to avoid leaving unused test dependencies, but the test file itself still contains the logic that requires it. This test should be finalized.
