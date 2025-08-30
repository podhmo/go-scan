# Problem: `symgo` Fails on Imports Where Package Name Mismatches Path

**Status: Resolved**

## Summary

The `symgo` symbolic execution engine had a flawed mechanism for resolving the package name of an import. It naively assumed the package name is the last segment of the import path. This failed for packages like `gopkg.in/yaml.v2`, where the import path is `gopkg.in/yaml.v2` but the declared package name is `yaml`.

Additionally, when `symgo` operated on code outside its scan policy, it would error on undefined identifiers, halting analysis. The desired behavior was to create a symbolic placeholder to allow analysis to continue.

## Resolution

The issue was resolved through a series of fixes to the `symgo` evaluator:

1.  **Lazy, Correct Import Resolution**: The old, incorrect import handling logic was removed from `symgo/symgo.go`. The `evalIdent` function in the evaluator was enhanced to handle package resolution lazily. When it encounters an unresolved identifier, it now checks the file's imports. For imports without an alias, it performs a trial scan of the package (`go-scan` caches the result) to determine its *actual* package name. It then creates the package object with the correct name, allowing subsequent selections (e.g., `yaml.Marshal`) to succeed.

2.  **Resilience for Out-of-Policy Code**: The `evalIdent` function was modified. If an identifier is not found, it now checks if the containing package is within the `ScanPolicy`. If the package is out-of-policy, `evalIdent` returns a `*object.SymbolicPlaceholder` instead of an error.

3.  **Resilience for Typeless Placeholders**: A follow-on fix was made to `evalSelectorExpr`. If a method is called on a symbolic placeholder that has no type information (which is the case for placeholders created for undefined identifiers), it no longer errors. Instead, it returns another placeholder representing the result of the symbolic call.

4.  **Built-in Type Resolution**: A bug discovered during testing, where built-in type identifiers like `string` were not found, was fixed. `evalIdent` was updated to recognize a list of Go's built-in types and return a placeholder, allowing type conversions like `string(b)` to be evaluated symbolically.

These changes together make the `symgo` engine more robust and accurate in handling complex import schemes and analyzing code with incomplete dependency information. The fixes were verified with a new test suite in `symgo_mismatch_import_test.go`.
