# Testing `minigo` Scripts with Local Imports

When writing tests for tools that use the `minigo` interpreter, a common challenge arises when the `minigo` script itself needs to import packages from the test's local context. This is especially true when the test sets up its own Go module (e.g., in a temporary directory or a `testdata` directory) that is nested within the main project's module.

This document outlines the problem and provides robust solutions for handling it.

## The Problem: Mismatched Module Contexts

Consider a tool `my-tool` that uses `minigo` to load a configuration script. The test for `my-tool` lives in `my-tool/main_test.go`. To test the configuration loading, the test creates a temporary, self-contained module structure:

```
/path/to/my-tool/
├── go.mod        (module: github.com/me/my-tool)
├── main_test.go
└── testdata/
    └── my-config/
        ├── go.mod      (module: my-config)
        ├── config.go   (minigo script)
        └── helper/
            └── helper.go (package helper)
```

The `config.go` script needs to import a local helper package (`import "my-config/helper"`) or a package from the main project (`import "github.com/me/my-tool/other-pkg"`).

The problem arises because:
1. The `go test` command is run from `/path/to/my-tool`. Its primary module context is `github.com/me/my-tool`.
2. `my-tool` creates a `minigo` interpreter.
3. By default, this interpreter creates its own `go-scan` scanner instance. This scanner inherits the primary module context (`github.com/me/my-tool`), and its `WorkDir` will be the CWD of the test runner.
4. When the interpreter evaluates `config.go` and sees an import, its internal scanner tries to resolve it. It will likely fail because its module context is incorrect for the temporary file structure. It knows nothing about the nested `my-config` module or its `replace` directives.

## The Solution: Passing a Configured Scanner

The key to solving this is to ensure the `minigo` interpreter uses a `go-scan.Scanner` that is correctly configured for the test's module context, rather than the host tool's context.

The `minigo.NewInterpreter` function now **requires** a `*goscan.Scanner` as its first argument. This makes the dependency explicit and forces the caller to provide a correctly configured scanner. The old `minigo.WithScanner()` option has been removed.

### Method 1: Using the `scantest` Helper (Recommended)

The `scantest` package is designed to simplify these scenarios. `scantest.Run` automatically creates a scanner that is correctly configured for the temporary test directory it manages. It also correctly handles `replace` directives with relative paths by creating an in-memory file overlay with absolute paths.

**How it works:**
1. Use `scantest.WriteFiles` to create the test file layout. The `go.mod` for the test module should use a `replace` directive with a relative path to depend on the main project if needed.
2. `scantest.Run` takes an `action` function as an argument. This function receives the correctly configured `*goscan.Scanner` instance.
3. Pass this shared scanner to the code that creates the `minigo` interpreter (e.g., a config loader).

**Example (`docgen/integration_test.go`):**

```go
func TestDocgen_integrationWithSharedScanner(t *testing.T) {
	files := map[string]string{
		// Test module `go.mod`. The relative replace is crucial.
		"go.mod": "module my-test\n\ngo 1.24\n\nreplace github.com/podhmo/go-scan => ../../\n",

		// The minigo script. It imports a package from the main project.
		"patterns.go": `
package patterns
import "github.com/podhmo/go-scan/examples/docgen/patterns"
// ... use patterns package ...
var Patterns = []patterns.PatternConfig{
	{Key: "dummy", Type: patterns.RequestBody},
}
`,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// `scantest.Run` does NOT change the CWD. We must construct an absolute path.
		patternsFilePath := filepath.Join(dir, "patterns.go")
		_, err := LoadPatternsFromConfig(patternsFilePath, slog.Default(), s)
		return err
	}

	if _, err := scantest.Run(t, dir, nil, action); err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}
```

### Method 2: Manual Scanner Configuration (Without `scantest`)

If you are not using the `scantest` harness (e.g., for a test that uses a fixed `testdata` directory), you can achieve the same result by manually creating and configuring the scanner.

**How it works:**
1. Your test files are in a fixed location, e.g., `testdata/my-module`.
2. The `go.mod` in that directory should use a relative `replace` path.
3. In your test, create a `goscan.Scanner` instance.
4. Use `goscan.WithWorkDir()` to point the scanner to the root of your test module (`testdata/my-module`). This is the crucial step. `go-scan`'s locator will correctly handle the relative `replace` path from this context.
5. Pass this manually configured scanner to your tool's config loader.

**Example (`docgen/main_test.go`):**

```go
func TestDocgen_withCustomPatterns(t *testing.T) {
	moduleDir := "testdata/custom-patterns"

	// Setup: Change directory so file paths are simple.
	wd, _ := os.Getwd()
	os.Chdir(moduleDir)
	defer os.Chdir(wd)

	// 1. Create a scanner explicitly configured for the test module directory.
	// Note: WithWorkDir is not strictly needed here because we used os.Chdir,
	// but it's good practice for clarity.
	s, err := goscan.New(
		goscan.WithWorkDir("."),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// 2. Pass the configured scanner to the loader.
	_, err = LoadPatternsFromConfig("patterns.go", slog.Default(), s)
	if err != nil {
		t.Fatalf("failed to load custom patterns: %v", err)
	}
    // ...
}
```

## Common Errors and Troubleshooting

- **Error:** `undefined: my-package.MyType (package scan failed: could not find package directory ...)`
  - **Cause:** The `minigo` interpreter's internal scanner does not have the correct module context. It cannot find the import path.
  - **Solution:** Ensure you are using one of the two methods above. Either let `scantest` provide the correctly configured scanner, or create one manually using `goscan.WithWorkDir()` pointed at your test module's root. Make sure this scanner is then passed directly to `minigo.NewInterpreter()`. If your test module needs to access packages from your main project, ensure you have a `replace` directive with a relative path in your test module's `go.mod`.

- **Error:** `could not read config file: open my-config.go: no such file or directory`
  - **Cause:** The path to the script/config file is incorrect. This often happens when `os.Chdir` is not used or when running in a temporary directory.
  - **Solution:** Always construct a full, absolute path to the file you want to read, for example by joining it with the directory returned by `scantest.WriteFiles` or a known `testdata` path.
```go
// For scantest
patternsFilePath := filepath.Join(dir, "patterns.go")
LoadPatternsFromConfig(patternsFilePath, ...)

// For a fixed testdata directory
patternsFilePath := filepath.Join("testdata", "my-module", "patterns.go")
LoadPatternsFromConfig(patternsFilePath, ...)
```
