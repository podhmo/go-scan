# Troubleshooting: Resolving Standard Library Types

## 1. Summary

A persistent issue in `go-scan` was its inability to scan standard library packages (e.g., `time`) when run from within a `go test` binary. This would manifest as a `mismatched package names` error. The standard workaround was to use `goscan.WithExternalTypeOverrides` to provide a "synthetic" definition for types like `time.Time`, but this was a cumbersome requirement.

This document details the investigation into this issue and the two-part fix that makes stdlib scanning robust and removes the need for mandatory overrides.

## 2. The Core Problem: `mismatched package names` in Tests

When you run `go test`, the Go toolchain compiles a special test binary where the `main` package is a test runner synthesized by the tool. When `go-scan`, executing inside this binary, attempts to parse the source files of a standard library package (e.g., `/usr/local/go/src/time`), a conflict occurs. The scanner's parser would see that some files belong to `package time` while also being influenced by the test binary's `package main` context. This resulted in the scanner believing there were two different packages in the same directory, causing the `mismatched package names: time and main` error.

This is a known, tricky issue related to using Go analysis tools within a test environment.

## 3. The Solution: Two-Pass Resilient Scanning

The issue was resolved by making the scanner's file parsing logic more resilient to this environmental quirk. The `scanGoFiles` function in `scanner/scanner.go` was refactored from a single-pass to a two-pass approach.

1.  **First Pass (Package Clause Scan)**: The scanner first makes a quick pass over all `.go` files in a directory, parsing *only* the `package <name>` clause. It counts the occurrences of each package name found.

2.  **Dominant Package Detection**: It then determines the "dominant" package name. The logic is specifically designed to handle the test-binary issue: if it finds both `"main"` and another name (e.g., `"time"`), it correctly chooses the non-`main` name as dominant.

3.  **Second Pass (Full AST Parse)**: The scanner makes a second, full pass over the files. It now *only* parses the full AST for files that belong to the dominant package name identified in the previous step. Any files that would have caused a name mismatch are safely ignored.

This enhancement makes the scanner robust enough to handle stdlib packages directly, even within a test. As a result, **the `ExternalTypeOverride` workaround is no longer necessary for standard library types.**

## 4. Related Issue: Pointer Resolution

During the initial investigation, a related bug was found and fixed concerning pointers to overridden types (e.g., `*time.Time`).

-   **Problem**: Even when an override was provided for `time.Time`, resolving `*time.Time` would still fail. This was because the scanner's logic did not propagate the "resolved by config" status from the element type (`time.Time`) to the pointer type that wrapped it.
-   **Solution**: The scanner was fixed to ensure that if a type is resolved by an override, any pointer to that type is also marked as resolved by the override.

While this fix was effective, it is now largely superseded by the two-pass scanning solution, which removes the need for the override in the first place. Both fixes working together create a more robust and user-friendly scanning engine.
