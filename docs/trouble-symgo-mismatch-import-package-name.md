# Trouble: symgo Fails to Resolve Imports with Mismatched Package Names

Date: 2025-08-29

## The Problem

The `symgo` symbolic execution engine incorrectly resolves the package name for certain Go packages. Specifically, when a package's import path does not match its package name (as declared in the `package` statement), `symgo` makes an incorrect assumption.

A common example is `gopkg.in/yaml.v2`. The import path is `gopkg.in/yaml.v2`, but the package name is `yaml`.

The existing logic in both `symgo/symgo.go` and `symgo/evaluator/evaluator.go` naively takes the last part of the import path as the package name. For `gopkg.in/yaml.v2`, this results in the incorrect package name `v2`.

This leads to "identifier not found" errors when analyzing code that uses such packages without an explicit import alias, for example:

```go
import "gopkg.in/yaml.v2"

// ...

var v yaml.Node // symgo would fail here, looking for "v2.Node"
```

Furthermore, the import handling logic was duplicated. The `symgo.Interpreter` pre-processed imports before handing off to the `evaluator`, which then processed them again. This redundancy made the code harder to maintain and was the source of the initial incorrect processing.

## The Solution

The solution involves centralizing the import logic within the `evaluator` and making it more intelligent by using the `go-scan` library to determine the correct package name.

1.  **Centralize Logic in the Evaluator**: The redundant import pre-processing loop in `symgo.Interpreter.Eval` will be removed. All import processing will now be handled exclusively by `symgo.evaluator.Evaluator` as part of its `evalFile` and `evalImportSpec` methods. This follows the principle of single responsibility and makes the system easier to reason about.

2.  **Implement Correct Package Name Resolution**: The `evaluator.evalImportSpec` method will be modified. When it encounters an import statement without an explicit alias (e.g., `import "gopkg.in/yaml.v2"`), it will perform the following steps:
    a.  Invoke the `go-scan` scanner (`scanner.ScanPackageByImport`) on the import path.
    b.  The scanner will parse the package's source files and return a `scanner.PackageInfo` struct, which contains the correct package name from its `package` declaration (e.g., `yaml`).
    c.  This correct name will be used to create the `object.Package` and register it in the environment.
    d.  To maintain robustness, if `ScanPackageByImport` fails (e.g., for a package that cannot be found), the evaluator will log a warning and fall back to the previous behavior of using the base of the import path.

3.  **Pre-cache Scanned Information**: As a performance optimization, when `evalImportSpec` successfully scans a package to determine its name, it will store the resulting `scanner.PackageInfo` in the `object.Package`. This avoids the need for the `evaluator` to re-scan the same package later when one of its symbols is accessed.

This change ensures that `symgo` can correctly analyze a wider range of Go code, including projects with common dependencies like `go-yaml`, regardless of whether the package is inside or outside the user-defined scan policy. The name resolution will be correct, and the policy will only determine whether the analysis can trace calls *into* the package's functions.
