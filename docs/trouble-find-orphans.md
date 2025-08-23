# Trouble Shooting: `find-orphans` and `symgo` Evaluation

This document details the debugging process for a persistent issue encountered while developing the `find-orphans` tool. The core problem is that the symbolic execution engine, `symgo`, is not correctly tracking function and method calls, resulting in an empty usage map and a report where nearly all functions are incorrectly flagged as orphans.

## Symptoms

1.  The `find-orphans` test (`examples/find-orphans/main_test.go`) consistently fails.
2.  The non-verbose output lists almost every function, including those that are clearly used (like `main`, `greeter.New`, `greeter.SayHello`), as orphans.
3.  The verbose output is missing the "-- Used Functions --" section entirely, confirming the `usageMap` is not being populated.
4.  A corresponding test in `symgo` itself (`TestEvalCallExpr_VariousPatterns/method_chaining`) also fails, indicating the bug is in the evaluator, not just the `find-orphans` tool's usage of it.

## Investigation and Fixes Attempted

The debugging process involved several iterations, fixing progressively deeper bugs in the `symgo` evaluator.

1.  **Test Setup Issues**: The initial test setup used an in-memory file overlay, but the `go-scan` `Locator` component expects a real file system. This was fixed by refactoring the tests to write to a temporary directory.

2.  **Isolated `Eval` Calls**: The analysis loop was initially calling `Interpreter.Eval()` for every function declaration. This was incorrect as it doesn't trace the call graph. The fix was to only `Apply()` the `main` function as the entry points.

3.  **`ReturnValue` Unwrapping**: It was discovered that the evaluator was not unwrapping `*object.ReturnValue` objects after a function call, causing type information to be lost on assignment. A fix was added to `evalIdentAssignment`.

4.  **Missing Method Call Implementation**: The evaluator had no logic to handle method calls on concrete types (e.g., `myStruct.DoSomething()`). The `evalSelectorExpr` logic for `*object.Instance` and `*object.Variable` only checked for intrinsics. A new `evalMethodCall` helper function was created to handle this, but it is still not working correctly.

## Current Status and Root Cause Analysis

Despite all the above fixes, the tests still fail with the same symptoms: the `usageMap` is empty. The `defaultIntrinsic` is never called.

The fundamental issue remains: `symgo`'s `evalCallExpr` successfully calls `applyFunction`, but the chain of evaluation within `applyFunction` (which recursively calls `Eval`) does not seem to trigger the `defaultIntrinsic` for nested calls.

**Hypothesis:** The `Interpreter`'s environment (`i.globalEnv`) is correctly populated with `*object.Function` definitions when all files are evaluated. However, when `Apply` is called on the `main` function, the subsequent calls within its body (e.g., `greeter.New()`) are resolved to `*object.Function` objects, but the `evalCallExpr` that should be triggered for them is somehow being bypassed or the `defaultIntrinsic` is not firing.

The problem is extremely subtle and lies deep within the evaluator's logic for how it handles environments and function application. The `TestDefaultIntrinsic` passes, but it tests a very simple case. The more complex scenario in `find-orphans` with cross-package calls and method calls is failing. Further debugging will require a step-by-step trace of the `Eval` and `Apply` calls for the `find-orphans` test case.

## Required `symgo` Enhancements

To make `find-orphans` and other complex tools viable, the `symgo` evaluator needs the following features/fixes:

-   **Reliable Method Dispatch**: The logic in `evalMethodCall` must be able to correctly resolve and execute a method call on a variable or instance of a concrete struct type, including handling pointer vs. non-pointer receivers correctly.
-   **Correct Type Propagation**: Type information must be correctly propagated through variable assignments (`:=`, `=`) and function returns. The `ReturnValue` unwrapping was one part of this, but other leaks may exist.
-   **Robust Environment Management**: The call stack and lexical scoping must be handled correctly so that when a function `A` calls function `B`, the execution of `B` occurs in the correct environment and the `defaultIntrinsic` is triggered for the call.
-   **Tracing and Debuggability**: The `--inspect` flag or a similar mechanism should be extended to provide a detailed trace of the symbolic execution flow, including which functions are called, what their arguments are, and what values are returned. This would have made debugging this issue significantly easier.

Until these issues are addressed, building tools that rely on deep call graph analysis (like `find-orphans`) with `symgo` will be unreliable.

## Post-Mortem: Lessons Learned from Testing

After fixing the core `symgo` evaluation bugs, the `find-orphans` test still failed, but for entirely different reasons related to the test setup itself. This section documents those issues and their solutions.

### Problem 1: Filesystem Paths vs. Import Paths

The most persistent issue was a failure to correctly resolve packages within the temporary test module created by `scantest.WriteFiles`. The test consistently failed with an error like:

`could not find directory for import path ./...`

Or when an import path was used:

`could not find directory for import path example.com/find-orphans-test/...`

**Root Cause:** A fundamental misunderstanding of how the `go-scan` APIs work.
-   The `go-scan` **`ModuleWalker`** operates on **Go import paths** (e.g., `"example.com/me/foo"`). It uses its internal `locator` to resolve these to filesystem directories.
-   The `go test` command and filesystem functions operate on **filesystem paths** (e.g., `"."`, `"./..."`, `"/tmp/..."`).

My mistake was passing filesystem patterns like `.` or `./...` to the `find-orphans` logic, which expected import paths.

**Solution:**
1.  **Configure the Scanner Correctly**: When testing a tool that needs to resolve a temporary module, the `goscan.Scanner` must be initialized with two key options:
    -   `goscan.WithWorkDir(tempDir)`: This tells the scanner's internal `locator` that `tempDir` is the root of a module, allowing it to find the `go.mod` file.
    -   `goscan.WithGoModuleResolver()`: This enables the locator to understand `go.mod` files and resolve dependencies.
2.  **Use the Right Path Type**: The test must pass the correct **import path pattern** to the logic being tested.

**Correct Test Implementation (`examples/find-orphans/main_test.go`):**
```go
func TestFindOrphans(t *testing.T) {
	// 1. Create a temporary module on the filesystem.
	dir, cleanup := scantest.WriteFiles(t, files) // files contains go.mod with "module example.com/find-orphans-test"
	defer cleanup()

	// 2. Define the starting pattern using the Go import path.
	startPatterns := []string{"example.com/find-orphans-test/..."}

	// 3. Call the main logic, passing the temp directory as the workspace
	//    and the import path pattern as the starting point.
	err := run(
		context.Background(),
		true,    // all
		false,   // includeTests
		dir,     // workspace
		false,   // verbose
		startPatterns,
	)
	if err != nil {
		t.Fatalf("run() failed: %v", err)
	}
    // ... assertions ...
}
```

**Correct `run` function setup (`examples/find-orphans/main.go`):**
```go
func run(ctx context.Context, ..., workspace string, ..., startPatterns []string) error {
    var scannerOpts []goscan.ScannerOption
    // ...
    if workspace != "" {
        // This is the crucial link.
        scannerOpts = append(scannerOpts, goscan.WithWorkDir(workspace))
    }
    // This is also crucial for module resolution.
    scannerOpts = append(scannerOpts, goscan.WithGoModuleResolver())

    s, err := goscan.New(scannerOpts...)
    // ...
    // The walker will now correctly resolve the import paths in startPatterns
    // relative to the workspace's go.mod.
    return a.analyze(ctx, startPatterns)
}
```

### Problem 2: `...` Wildcard Support

The `go` command-line tool has built-in support for the `...` wildcard to specify all packages within a path. The `go-scan` library does **not** have this built-in support. The `ModuleWalker` expects a list of concrete import paths to walk.

**Solution:** The `find-orphans` tool was enhanced to expand `...` patterns manually. It performs a preliminary walk of the module to collect all package paths and then analyzes the collected list. This makes the tool's command-line interface behave as users would expect.
