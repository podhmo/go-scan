# Plan: Selectable Entry Points for `find-orphans` in App Mode

This document outlines the plan to enhance the `find-orphans` tool by allowing users to specify which `main` packages should be used as entry points for analysis in application mode.

## 1. Current Situation

- The `find-orphans` tool can run in two primary modes: `lib` (library) and `app` (application).
- In `app` mode, the tool identifies all functions named `main` in packages named `main` (i.e., all `main.main` functions) within the analysis scope.
- It then treats all of these discovered `main.main` functions as the entry points for the call-graph analysis.
- There is currently no mechanism to restrict the analysis to a specific subset of these `main.main` entry points. In a repository with multiple binaries (multiple `main` packages), the tool analyzes them all simultaneously, which may not be the desired behavior.

## 2. Desired Outcome

The `find-orphans` tool should be updated to allow a user to specify one or more `main` packages to be used as entry points when running in `app` mode.

- A new command-line flag, `--entrypoint-pkg`, will be introduced.
- This flag will accept a comma-separated list of package import paths (e.g., `example.com/cmd/server1,example.com/cmd/server2`).
- If this flag is provided while in `app` mode, only the `main.main` functions from the specified packages will be used as the starting points for the orphan analysis.
- If the flag is not provided, the tool's behavior should remain unchanged (i.e., it will use all found `main.main` functions as entry points).
- The flag should have no effect in `lib` mode.

## 3. Implementation Plan

The implementation will be focused on the `tools/find-orphans/main.go` file.

1.  **Add New Command-Line Flag**:
    - In the `main` function, define a new `stringSliceFlag` named `entrypointPkgs`.
    - The flag will be exposed as `--entrypoint-pkg`.
    - The help text will describe its purpose: "comma-separated list of main packages to use as entry points in app mode".

2.  **Propagate Flag Value**:
    - Pass the `entrypointPkgs` slice from `main` into the `run` function.
    - Add an `entrypointPkgs` field to the `analyzer` struct.
    - Store the passed value in the new field when the `analyzer` is instantiated.

3.  **Filter Entry Points**:
    - In the `analyzer.analyze` method, locate the section where `mainEntryPoints` are collected.
    - After collecting all `main.main` functions but *before* the `switch a.mode` statement, add a new filtering step.
    - This step will check if `len(a.entrypointPkgs) > 0`.
    - If it is, create a new slice `filteredMainEntryPoints`. Iterate through the original `mainEntryPoints` and add a function to the new slice only if its package path (`fn.Package.ImportPath`) is present in the `a.entrypointPkgs` list.
    - Replace the `mainEntryPoints` slice with `filteredMainEntryPoints`.

4.  **Add Validation**:
    - If the `--entrypoint-pkg` flag is used, but none of the specified packages contain a `main.main` function, the program should exit with a clear error message.

5.  **Create Tests**:
    - In `main_test.go`, add a new test case to a suitable test function (or create a new one).
    - This test will simulate a project with at least two `main` packages.
    - The test will run `find-orphans` in `app` mode with the `--entrypoint-pkg` flag set to one of the main packages.
    - It will assert that only the orphans relative to the *specified* entry point are reported, and that functions used only by the *other* main package are correctly identified as orphans.

6.  **Update Documentation**:
    - Briefly update `tools/find-orphans/spec.md` to mention the new `--entrypoint-pkg` flag and its effect on `app` mode analysis.

## 4. Risks and Considerations

- **User Experience**: The error message for non-existent or invalid entry point packages must be clear and helpful.
- **Backwards Compatibility**: The changes must not alter the default behavior. If the `--entrypoint-pkg` flag is not used, the tool should function exactly as it does now. This will be ensured by only triggering the filtering logic when the flag is explicitly provided.