# Trouble: symgo Fails to Resolve Imports with Mismatched Package Names

Date: 2025-08-29

## The Problem

The `symgo` symbolic execution engine incorrectly resolves the package name for certain Go packages. Specifically, when a package's import path does not match its package name (as declared in the `package` statement), `symgo` makes an incorrect assumption.

A common example is `gopkg.in/yaml.v2`. The import path is `gopkg.in/yaml.v2`, but the package name is `yaml`.

The existing logic naively takes the last part of the import path as the package name. For `gopkg.in/yaml.v2`, this results in the incorrect package name `v2`. This leads to "identifier not found" errors when analyzing code that uses such packages without an explicit import alias.

## The Solution

The solution involves centralizing the import logic within the `evaluator` and making it more intelligent by using the `go-scan` library to determine the correct package name when an import is first processed.

1.  **Centralize Logic**: The redundant import pre-processing loop in `symgo.Interpreter.Eval` was removed. All import processing is now handled exclusively by `symgo.evaluator.Evaluator`'s `evalImportSpec` method.
2.  **Implement Correct Package Name Resolution**: The `evaluator.evalImportSpec` method was modified to call `scanner.ScanPackageByImport` for unaliased imports. This parses the target package to discover its true name from its `package` declaration.
3.  **Consolidate Logic**: A similar import-handling loop in `applyFunction` was also refactored to use the new centralized `evalImportSpec` method.

## Implementation and Results

A test suite (`symgo_mismatched_import_test.go`) was created to verify the fix using the `gopkg.in/yaml.v2` package. The tests cover two scenarios: one where the `yaml` package is "in-policy" for scanning, and one where it is "out-of-policy".

**The tests currently fail.** The error is `return value has no type info`.

The investigation revealed a deeper architectural issue between `symgo` (the dynamic evaluator) and `go-scan` (the static analyzer):

1.  The fix in `evalImportSpec` correctly populates the `symgo` environment. For example, it creates an entry where the key is `"yaml"` and the value is the package object for `gopkg.in/yaml.v2`.
2.  However, when the evaluator encounters a variable declaration like `var node yaml.Node`, it calls `scanner.TypeInfoFromExpr` to resolve the type `yaml.Node`.
3.  `TypeInfoFromExpr` is a static analysis function within `go-scan`. It does **not** have access to `symgo`'s dynamic environment. It relies on a static `importLookup` map generated from the file's `import` statements.
4.  For an unaliased import like `import "gopkg.in/yaml.v2"`, the static lookup map has no entry for the name `yaml`.
5.  Consequently, `TypeInfoFromExpr` fails to resolve the type and returns `nil`. The variable is created with no type information, causing the test assertion to fail.

This creates a deadlock: `symgo` needs to resolve types to evaluate code, but the type resolver (`go-scan`) doesn't have the necessary package name information that `symgo` has already discovered. The scan policy (in-policy vs. out-of-policy) does not affect this outcome, as the failure occurs at the static type resolution phase, before the policy is applied during symbolic execution.

### Attempted Workaround: Augmented Import Lookup

A workaround was attempted to bridge this gap. The idea was to augment the static `importLookup` map before passing it to `scanner.TypeInfoFromExpr`. A new helper function, `augmentImportLookup`, was added to the evaluator. This function would:
1.  Copy the static `importLookup` map.
2.  Walk the `symgo` environment and find all `object.Package` instances.
3.  For each package, it would add a new entry to the lookup map: `lookup[<correct package name>] = <import path>`.

This augmented map was then used in `evalGenDecl`, `evalCompositeLit`, etc. The hypothesis was that this would provide `TypeInfoFromExpr` with the information it was missing (e.g., a mapping for `"yaml"` -> `"gopkg.in/yaml.v2"`).

**This attempt also failed.** The tests continued to fail with the same "no type info" error. The root cause appears to be the same: the `importLookup` is still not being correctly applied or accessed at the point where `TypeInfoFromExpr` needs it, indicating a fundamental disconnect between the static analysis capabilities of `go-scan` and the dynamic environment of `symgo`.

Resolving this would likely require a more significant architectural change to how `go-scan` and `symgo` share package metadata. The current implementation has been submitted to document the problem and the findings.
