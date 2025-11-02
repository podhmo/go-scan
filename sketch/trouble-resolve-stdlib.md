# Troubleshooting: Resolving Standard Library Types

## 1. Summary

A persistent issue in `go-scan` was its inability to scan standard library packages (e.g., `time`) when run from within a `go test` binary. This would manifest as a `mismatched package names` error. The initial workaround was to use `goscan.WithExternalTypeOverrides` to provide a "synthetic" definition for types like `time.Time`, but this was a cumbersome requirement.

This document details the investigation into this issue and the final, optimized solution that makes stdlib scanning robust and performant, removing the need for mandatory overrides.

## 2. The Core Problem: `mismatched package names` in Tests

When you run `go test`, the Go toolchain compiles a special test binary where the `main` package is a test runner synthesized by the tool. When `go-scan`, executing inside this binary, attempts to parse the source files of a standard library package (e.g., `/usr/local/go/src/time`), a conflict occurs. The scanner's parser would see that some files belong to `package time` while also being influenced by the test binary's `package main` context. This resulted in the scanner believing there were two different packages in the same directory, causing the `mismatched package names: time and main` error.

This is a known, tricky issue related to using Go analysis tools within a test environment.

## 3. The Solution: Optimistic Single-Pass Scanning

The issue was resolved by making the scanner's file parsing logic both resilient and performant. The `scanGoFiles` function in `scanner/scanner.go` was refactored to use an "optimistic single-pass" approach with a fallback heuristic.

1.  **Optimistic Single Pass**: The scanner starts by parsing files one by one, assuming the first package name it sees is the correct "dominant" name for the directory. This is fast and avoids the I/O overhead of reading every file twice.

2.  **Heuristic-Based Mismatch Resolution**: If a file is encountered with a different package name, a heuristic is applied. If the mismatch is between the dominant name and `"main"`, the scanner correctly assumes `"main"` is an artifact of the `go test` environment and safely ignores the file associated with it. This prevents the error without sacrificing performance for the common case.

3.  **Error for Genuine Mismatches**: If the mismatch is between two non-`main` packages (e.g., `package_a` and `package_b` in the same directory), the scanner correctly reports this as a fatal error, as this indicates a real problem with the code being scanned.

This enhancement makes the scanner robust enough to handle stdlib packages directly, even within a test, while maintaining optimal performance. As a result, **the `ExternalTypeOverride` workaround is no longer necessary for standard library types.**

## 4. Related Issue: Pointer Resolution

During the initial investigation, a related bug was found and fixed concerning pointers to overridden types (e.g., `*time.Time`).

-   **Problem**: Even when an override was provided for `time.Time`, resolving `*time.Time` would still fail. This was because the scanner's logic did not propagate the "resolved by config" status from the element type (`time.Time`) to the pointer type that wrapped it.
-   **Solution**: The scanner was fixed to ensure that if a type is resolved by an override, any pointer to that type is also marked as resolved by the override.

This fix remains in place and works in concert with the optimistic scanning to ensure all forms of type resolution are robust.
