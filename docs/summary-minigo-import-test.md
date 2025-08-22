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

The `config.go` script needs to import the local helper package: `import "my-config/helper"`.

The problem arises because:
1. The `go test` command is run from `/path/to/my-tool`. Its primary module context is `github.com/me/my-tool`.
2. `my-tool` creates a `minigo` interpreter.
3. By default, this interpreter creates its own `go-scan` scanner instance. This scanner inherits the primary module context (`github.com/me/my-tool`).
4. When the interpreter evaluates `config.go` and sees `import "my-config/helper"`, its internal scanner tries to resolve this path. It fails because the scanner's context is `github.com/me/my-tool`, and it has no knowledge of the nested `my-config` module.

## The Solution: Sharing the Scanner

The key to solving this is to ensure the `minigo` interpreter uses a `go-scan.Scanner` that is correctly configured for the test's module context, rather than the host tool's context.

The `minigo.Interpreter` provides the `minigo.WithScanner()` option for this purpose. The host tool can create and configure a scanner and then pass it to the interpreter.

### Method 1: Using the `scantest` Helper (Recommended)

The `scantest` package is designed to simplify these scenarios. `scantest.Run` automatically creates a scanner that is correctly configured for the temporary test directory it manages.

**How it works:**
1. Use `scantest.WriteFiles` to create the test file layout.
2. `scantest.Run` takes an `action` function as an argument. This function receives the correctly configured `*goscan.Scanner` instance.
3. Pass this shared scanner to the code that creates the `minigo` interpreter (e.g., a config loader).

**Example (`docgen/integration_test.go`):**

```go
import (
	"context"
	"path/filepath"
	"testing"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestMyTool_WithScantest(t *testing.T) {
	files := map[string]string{
		"go.mod": "module my-test\n",
		"config.go": `
package config
import "my-test/helper"
// ... use helper package ...
`,
		"helper/go.mod": "module my-test/helper\n",
		"helper/api.go": "package helper\nfunc Help() {}",
	}

	// The action function receives the scanner `s` from scantest
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		// Assume LoadConfig now accepts a scanner
		configPath := filepath.Join(s.Locator().RootDir(), "config.go")
		_, err := LoadConfig(configPath, s)
		return err
	}

	// scantest.Run manages the temporary directory and scanner setup
	if _, err := scantest.Run(t, "", nil, action, scantest.WithFiles(files)); err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}
```

### Method 2: Manual Scanner Configuration (Without `scantest`)

If you are not using the `scantest` harness, you can achieve the same result by manually creating and configuring the scanner.

**How it works:**
1. Create your test directory and files manually (e.g., in `testdata`).
2. Before calling your tool's logic, create a `goscan.Scanner` instance.
3. Use `goscan.WithWorkDir()` to point the scanner to the root of your test module. This is the crucial step.
4. Pass this manually configured scanner to your tool's config loader.

**Example:**

```go
func TestMyTool_Manual(t *testing.T) {
	// The test module is located in a subdirectory
	moduleDir := "testdata/my-config"
	configPath := filepath.Join(moduleDir, "config.go")

	// 1. Create a scanner explicitly configured for the test module directory
	s, err := goscan.New(goscan.WithWorkDir(moduleDir))
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	// 2. Pass the configured scanner to the loader
	_, err = LoadConfig(configPath, s)
	if err != nil {
		t.Fatalf("LoadConfig failed: %+v", err)
	}
}
```

## Common Errors and Troubleshooting

- **Error:** `undefined: my-package.MyType (package scan failed: could not find package directory ...)`
  - **Cause:** The `minigo` interpreter's internal scanner does not have the correct module context. It cannot find the import path.
  - **Solution:** Ensure you are using one of the two methods above. Either let `scantest` provide the correctly configured scanner, or create one manually using `goscan.WithWorkDir()` pointed at your test module's root. Make sure this scanner is then passed to the `minigo` interpreter via `minigo.WithScanner()`.

- **Error:** `could not read config file: open my-config.go: no such file or directory`
  - **Cause:** The path to the script/config file is incorrect. This often happens when using `scantest`, as the test runs in a temporary directory.
  - **Solution:** Construct the path to your config file using the directory provided by the test harness (e.g., `scantest.WriteFiles`) or the `RootDir()` of the configured scanner. Do not rely on a hardcoded relative path.
```go
// Inside a scantest action:
configPath := filepath.Join(s.Locator().RootDir(), "config.go")

// Or if you created the temp dir manually:
configPath := filepath.Join(tempDir, "config.go")
```
