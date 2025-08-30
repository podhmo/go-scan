# Problem: `symgo` Fails on Imports Where Package Name Mismatches Path

**Status: Resolved**

## Summary

The `symgo` symbolic execution engine had two related issues:

1.  **Mismatched Import Path/Package Name**: The engine's import resolution mechanism naively assumed a package's name was the last segment of its import path. This failed for packages like `gopkg.in/yaml.v2`, where the import path is `gopkg.in/yaml.v2` but the declared package name is `yaml`.
2.  **Undefined Built-in Types**: A follow-on bug was discovered during testing where the evaluator would fail on type conversions to built-in types (e.g., `string(b)`), reporting `identifier not found: string`.

Additionally, when `symgo` operated on code outside its defined scan policy, it would error on undefined identifiers, halting analysis. The desired behavior was to create a symbolic placeholder to allow analysis to continue.

## Resolution

These issues have been fully resolved through a series of fixes to the `symgo` evaluator:

1.  **Lazy, Correct Import Resolution**: The old, incorrect import handling logic was removed. The `evalIdent` function in the evaluator was enhanced to handle package resolution lazily by scanning packages on-demand to determine their *actual* package name. This now correctly handles cases like `gopkg.in/yaml.v2`.

2.  **Centralized Built-in Type Handling**: To fix the type conversion bug, the handling of all built-in types (`string`, `int`, `bool`, `error`, etc.) was centralized. A `types` map was added to the `universe` scope, which is pre-populated with symbolic placeholders for each built-in type. The `evalIdent` function was refactored to query this map, removing the fragile `switch` statement and ensuring built-in types are always resolved correctly.

3.  **Resilience for Out-of-Policy Code**: The `evalIdent` function was modified to return a `SymbolicPlaceholder` instead of an error when an undefined identifier is found in an out-of-policy package. This allows symbolic analysis to continue without crashing on external code.

4.  **Dependency-Free Testing**: The test suite for this feature (`symgo_mismatch_import_test.go`) was refactored to remove its dependency on `gopkg.in/yaml.v2`. It now uses a self-contained, locally generated package that simulates the mismatched name scenario, making the tests more robust and self-sufficient.

These changes make the `symgo` engine significantly more robust and reliable when analyzing real-world code with external dependencies and common Go patterns.
