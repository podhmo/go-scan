# Plan for Inspect and Dry-Run Features

This document outlines the plan to introduce `inspect` and `dry-run` modes to the `go-scan` library and its example applications.

## 1. Goals

The primary goals of this initiative are:

1.  **Improve Debuggability**: Provide a mechanism to inspect the marker/annotation detection process. This will help developers understand why a certain type is or is not being processed by a code generator.
2.  **Enable Safe Testing**: Introduce a "dry-run" mode that allows running the entire scanning and generation pipeline without writing any files to disk. This is useful for testing and validating generator output.
3.  **Maintain Usability**: Integrate these features in a way that is intuitive and consistent across the different example applications.

## 2. Technical Approach

The implementation is divided into two main features: Inspect Mode and Dry-Run Mode.

### 2.1. Inspect Mode

The inspect mode provides detailed logging about the annotation detection process.

1.  **Configuration via Options**:
    *   A new `WithInspect()` option should be added to the top-level `goscan.Scanner`.
    *   A `WithLogger(logger *slog.Logger)` option should also be added to allow the scanner to use the application's logger.

2.  **Configuration Propagation**:
    *   The `inspect` flag and the `logger` instance should be propagated from the main `goscan.Scanner` down to the internal `scanner.Scanner`.
    *   To avoid breaking changes to method signatures, each `scanner.TypeInfo` struct created by the parser could be populated with the `inspect` flag and a reference to the logger. This is a pragmatic choice to avoid a breaking change, though it slightly couples the data model (`TypeInfo`) with behavior (logging). This design decision should be carefully considered.

3.  **Logging Implementation**:
    *   The core logic should be implemented in the `scanner.TypeInfo.Annotation()` method.
    *   When `inspect` mode is active, this method would log its activity:
        *   **`DEBUG` Level**: A log should be generated for *every* annotation check, indicating the type being checked, the annotation name, and whether it was a "hit" or "miss".
        *   **`INFO` Level**: A log should be generated only for successful "hits", detailing the type, annotation, and the value found.

4.  **CLI Flag**:
    *   An `--inspect` boolean flag should be added to the example CLIs (`derivingjson`, `derivingbind`, `convert`).
    *   When this flag is used, the application would initialize the scanner with the `WithInspect()` and `WithLogger()` options.

### 2.2. Dry-Run Mode

The dry-run mode prevents the application from writing generated files.

1.  **Configuration via Option**:
    *   A new `WithDryRun()` option should be added to the `goscan.Scanner`. This would set a `DryRun bool` field on the scanner instance.

2.  **Application-Level Logic**:
    *   The responsibility for checking the `DryRun` flag lies within the individual applications, not the core `go-scan` library, as file writing is an application-specific concern.
    *   A `--dry-run` boolean flag should be added to the example CLIs.
    *   Before writing a file, the application would check the `gscn.DryRun` flag. If `true`, it would log a message indicating that the file write is being skipped and proceed without error.

## 3. Usage Example

To use the new features, developers could run the example applications with the new flags.

**Example: Inspecting `derivingjson`**

To see which `@deriving:json` annotations are being detected in a package, a user could run:

```sh
go run ./examples/derivingjson --inspect --log-level=debug ./examples/derivingjson/testdata/simple/
```

**Expected Log Output:**

```
DEBUG checking for annotation type=User annotation=@deriving:json result=hit
INFO found annotation type=User annotation=@deriving:json value=""
DEBUG checking for annotation type=UserProfile annotation=@deriving:json result=miss
...
```

**Example: Dry-running `convert`**

To see the generated code for the `convert` tool without writing the `generated.go` file:

```sh
go run ./examples/convert -pkg="<your_pkg>" --dry-run
```

**Expected Log Output:**

```
INFO Dry run: skipping file write path=generated.go
DEBUG Generated code would be:
---
package main
// ... generated code ...
---
```

This implementation would provide powerful debugging tools while maintaining a clean separation of concerns between the core library and the applications that use it.
