> [!NOTE]
> This feature has been implemented.

# Plan for Inspect and Dry-Run Features

This document outlines the plan to introduce `inspect` and `dry-run` modes to the `go-scan` library and its example applications.

## 1. Goals

- [x] **Improve Debuggability**: Provide a mechanism to inspect the marker/annotation detection process. This will help developers understand why a certain type is or is not being processed by a code generator.
- [x] **Enable Safe Testing**: Introduce a "dry-run" mode that allows running the entire scanning and generation pipeline without writing any files to disk. This is useful for testing and validating generator output.
- [x] **Maintain Usability**: Integrate these features in a way that is intuitive and consistent across the different example applications.

## 2. Technical Approach

The implementation is divided into two main features: Inspect Mode and Dry-Run Mode.

### 2.1. Inspect Mode

- [x] **Configuration via Options**:
    - [x] A new `WithInspect()` option should be added to the top-level `goscan.Scanner`.
    - [x] A `WithLogger(logger *slog.Logger)` option should also be added to allow the scanner to use the application's logger.
- [x] **Configuration Propagation**:
    - [x] The `inspect` flag and the `logger` instance should be propagated from the main `goscan.Scanner` down to the internal `scanner.Scanner`.
    - [x] To avoid breaking changes to method signatures, each `scanner.TypeInfo` struct created by the parser could be populated with the `inspect` flag and a reference to the logger. This is a pragmatic choice to avoid a breaking change, though it slightly couples the data model (`TypeInfo`) with behavior (logging). This design decision should be carefully considered.
- [x] **Logging Implementation**:
    - [x] The core logic should be implemented in the `scanner.TypeInfo.Annotation()` method.
    - [x] When `inspect` mode is active, this method would log its activity:
        - [x] **`DEBUG` Level**: A log should be generated for *every* annotation check, indicating the type being checked, the annotation name, and whether it was a "hit" or "miss".
        - [x] **`INFO` Level**: A log should be generated only for successful "hits", detailing the type, annotation, and the value found.
- [x] **CLI Flag**:
    - [x] An `--inspect` boolean flag should be added to the example CLIs (`derivingjson`, `derivingbind`, `convert`).
    - [x] When this flag is used, the application would initialize the scanner with the `WithInspect()` and `WithLogger()` options.

### 2.2. Dry-Run Mode

- [x] **Configuration via Option**:
    - [x] A new `WithDryRun()` option should be added to the `goscan.Scanner`. This would set a `DryRun bool` field on the scanner instance.
- [x] **Application-Level Logic**:
    - [x] The responsibility for checking the `DryRun` flag lies within the individual applications, not the core `go-scan` library, as file writing is an application-specific concern.
    - [x] A `--dry-run` boolean flag should be added to the example CLIs.
    - [x] Before writing a file, the application would check the `gscn.DryRun` flag. If `true`, it would log a message indicating that the file write is being skipped and proceed without error.

## 3. Usage Example

- [x] To use the new features, developers could run the example applications with the new flags.

## 4. Structured Logging Fields

- [x] **`DEBUG` Level Log (Annotation Check)**
- [x] **`INFO` Level Log (Annotation Found)**
- [x] **`DEBUG` and `INFO` Level Log (Type Resolution)**: Added `resolution_path` to logs to trace dependency resolution.
