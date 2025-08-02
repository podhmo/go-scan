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

## 4. Structured Logging Fields

To maximize the utility of the `inspect` mode, especially when logs are consumed by automated systems, the following structured fields should be included in the `slog` output.

### 4.1. `DEBUG` Level Log (Annotation Check)

This log is generated for every annotation check.

-   **`level`**: `DEBUG`
-   **`msg`**: `"checking for annotation"`
-   **`component`**: `"go-scan"` (To identify the log source)
-   **`type_name`**: `"User"` (The name of the struct/type being inspected)
-   **`type_pkg_path`**: `"example.com/m/models"` (The full package path of the type)
-   **`type_file_path`**: `"/path/to/project/models/user.go:10:1"` (The file path and position of the type definition)
-   **`annotation_name`**: `"@deriving:json"` (The full annotation string being searched for)
-   **`result`**: `"hit"` or `"miss"`
-   **`resolution_path`**: `"SrcStruct.FieldA -> NestedStruct.FieldB"` (The chain of fields that led to this type resolution, if applicable)

**Example Log Record (JSON format):**
```json
{
  "time": "2023-10-27T10:00:00.000Z",
  "level": "DEBUG",
  "msg": "checking for annotation",
  "component": "go-scan",
  "type_name": "User",
  "type_pkg_path": "example.com/m/models",
  "type_file_path": "/path/to/project/models/user.go:10:1",
  "annotation_name": "@deriving:json",
  "result": "hit"
}
```

### 4.2. `INFO` Level Log (Annotation Found)

This log is generated only when an annotation is successfully found.

-   **`level`**: `INFO`
-   **`msg`**: `"found annotation"`
-   **`component`**: `"go-scan"`
-   **`type_name`**: `"User"`
-   **`type_pkg_path`**: `"example.com/m/models"`
-   **`type_file_path`**: `"/path/to/project/models/user.go:10:1"`
-   **`annotation_name`**: `"@deriving:json"`
-   **`annotation_value`**: `"omitempty"` (The extracted value of the annotation, if any)
-   **`resolution_path`**: `"SrcStruct.FieldA -> NestedStruct.FieldB"` (The chain of fields that led to this type resolution, if applicable)

**Example Log Record (JSON format):**
```json
{
  "time": "2023-10-27T10:00:00.000Z",
  "level": "INFO",
  "msg": "found annotation",
  "component": "go-scan",
  "type_name": "User",
  "type_pkg_path": "example.com/m/models",
  "type_file_path": "/path/to/project/models/user.go:10:1",
  "annotation_name": "@deriving:json",
  "annotation_value": "omitempty"
}
```

By including these fields, logs become much more powerful for filtering and querying. For example, a user could easily find all failed annotation checks for a specific type or all successful hits for a particular annotation across the entire project. The `type_file_path` is especially useful for direct navigation from logs to the source code.

#### Implementation Note for `resolution_path`

To capture the `resolution_path`, the implementation would need to pass a `context.Context` through the type resolution process (e.g., `FieldType.Resolve(ctx, ...)`). As the resolution process traverses through struct fields, the path can be appended to a value within the context. The logging function would then extract this path from the context to add it to the log record. This approach ensures that the path information is available at the point of logging without requiring significant changes to method signatures throughout the call stack.
