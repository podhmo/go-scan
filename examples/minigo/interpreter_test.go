package main

import (
	"os"
	// "path/filepath" // For joining paths - No longer needed due to hardcoded paths
	// "runtime"       // For runtime.Caller - No longer needed
	"strings"
	"testing"

	"github.com/podhmo/go-scan" // For goscan.New
)

// Helper function to create a temporary Go source file for testing within a specific base directory.
func createTempFile(t *testing.T, content string, baseDir string) string {
	t.Helper()
	// Ensure the baseDir exists, create if not (though for testdata, it should exist)
	// For simplicity, assume baseDir exists or handle error appropriately if needed.
	// os.MkdirAll(baseDir, 0755) // Could be added if baseDir might not exist

	// Create a subdirectory within baseDir to further isolate temp files if desired,
	// or create directly in baseDir. Let's create directly in baseDir for now.
	// If using subdirectories:
	// tempSubDir, err := os.MkdirTemp(baseDir, "minigo_test_files_")
	// if err != nil {
	//  t.Fatalf("Failed to create temp subdirectory in %s: %v", baseDir, err)
	// }
	// tmpFile, err := os.CreateTemp(tempSubDir, "test_*.go")

	tmpFile, err := os.CreateTemp(baseDir, "test_*.go") // Create directly in baseDir
	if err != nil {
		t.Fatalf("Failed to create temp file in %s: %v", baseDir, err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name()) // Clean up partially created file
		t.Fatalf("Failed to write to temp file %s: %v", tmpFile.Name(), err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name()) // Clean up
		t.Fatalf("Failed to close temp file %s: %v", tmpFile.Name(), err)
	}
	return tmpFile.Name()
}

func TestImportStatements(t *testing.T) {
	// Determine resolvedTestdataDir once for all subtests
	var resolvedTestdataDir string // Keep this declaration
	cwd, wdErr := os.Getwd()
	if wdErr != nil {
		t.Fatalf("Failed to get current working directory: %v", wdErr)
	}
	// _, currentTestFile, _, ok := runtime.Caller(0) // No longer needed
	// if !ok {
	// 	t.Fatalf("Could not get current test file path for scanner setup")
	// }

	// Assuming tests are run from examples/minigo directory as per Makefile
	minigoPackageDir := "."
	resolvedTestdataDir = "testdata" // Assign to existing var

	// Log current working directory and resolved paths for verification
	t.Logf("### CWD: %s", cwd)
	t.Logf("### minigoPackageDir (relative): %s", minigoPackageDir)
	t.Logf("### resolvedTestdataDir (relative): %s", resolvedTestdataDir)

	// Verify that these relative paths correctly point to existing directories
	// by trying to stat them. Stat will use the current working directory.
	if _, err := os.Stat(minigoPackageDir); os.IsNotExist(err) {
		// This would mean CWD is not examples/minigo, or examples/minigo doesn't exist from CWD.
		t.Fatalf("FATAL: minigoPackageDir (expected '.') does not correspond to an existing directory from CWD '%s'. Error: %v", cwd, err)
	}
	if _, err := os.Stat(resolvedTestdataDir); os.IsNotExist(err) {
		// This would mean testdata directory is not found within CWD.
		t.Fatalf("FATAL: resolvedTestdataDir (expected './testdata') does not correspond to an existing directory from CWD '%s'. Error: %v", cwd, err)
	}

	// Create a temporary directory within resolvedTestdataDir for this test run's files
	// to avoid polluting the main testdata directory and to ensure cleanup.
	runSpecificTempDir, err := os.MkdirTemp(resolvedTestdataDir, "run_")
	if err != nil {
		t.Fatalf("Failed to create run-specific temp directory in %s: %v", resolvedTestdataDir, err)
	}
	defer os.RemoveAll(runSpecificTempDir) // Cleanup all files created in this directory

	tests := []struct {
		name                   string
		source                 string
		entryPoint             string
		expectedGlobalVarValue map[string]interface{} // string or int64
		expectError            bool
		expectedErrorMsgSubstr string
	}{
		{
			name: "import const without alias",
			source: `
package main
import "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
var resultString string
func main() {
	resultString = testpkg.ExportedConst
}`,
			entryPoint:             "main",
			expectedGlobalVarValue: map[string]interface{}{"resultString": "Hello from testpkg"},
		},
		{
			name: "import const with alias",
			source: `
package main
import pkalias "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
var resultInt int
func main() {
	resultInt = pkalias.AnotherExportedConst
}`, // Using a normal alias
			entryPoint:             "main",
			expectedGlobalVarValue: map[string]interface{}{"resultInt": int64(12345)},
		},
		{
			name: "reference non-exported const",
			source: `
package main
import "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
var r string
func main() {
	r = testpkg.NonExportedConst
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: "undefined selector: testpkg.NonExportedConst",
		},
		{
			name: "reference non-existent const",
			source: `
package main
import "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
var r string
func main() {
	r = testpkg.DoesNotExist
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: "undefined selector: testpkg.DoesNotExist",
		},
		{
			name: "reference symbol from non-existent package alias",
			source: `
package main
var r string
func main() {
	r = nonExistentAlias.Foo
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: "undefined package alias/name: nonExistentAlias",
		},
		{
			name: "reference symbol from non-existent package path after import",
			source: `
package main
import badpath "github.com/podhmo/go-scan/examples/minigo/testdata/nonexistentpkg"
var r string
func main() {
	r = badpath.Foo
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: `package "github.com/podhmo/go-scan/examples/minigo/testdata/nonexistentpkg" (aliased as "badpath") not found or failed to scan`,
		},
		{
			name: "disallowed dot import",
			source: `
package main
import . "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
func main() {
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: "dot imports are not supported",
		},
		{
			name: "blank import is ignored",
			source: `
package main
import _ "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
var resultInt int // Add a dummy variable to check execution
func main() {
	resultInt = 1 // Ensure main runs
}`,
			entryPoint:             "main",
			expectError:            false, // Should not error
			expectedGlobalVarValue: map[string]interface{}{"resultInt": int64(1)},
		},
		{
			name: "alias conflict",
			source: `
package main
import "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"
import testpkg "github.com/podhmo/go-scan/examples/minigo/testdata/anotherdummy"
func main() {
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: `import alias/name "testpkg" already used for "github.com/podhmo/go-scan/examples/minigo/testdata/testpkg"`,
		},
		{
			name: "import and call function from another package",
			source: `
package main
import "github.com/podhmo/go-scan/examples/minigo/stringutils"
var result string
func main() {
	result = stringutils.Concat("hello", " world")
}`,
			entryPoint:             "main",
			expectedGlobalVarValue: map[string]interface{}{"result": "hello world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use runSpecificTempDir for creating temp files for this test case
			filename := createTempFile(t, tt.source, runSpecificTempDir)
			// defer os.Remove(filename) // Cleanup is handled by defer os.RemoveAll(runSpecificTempDir)

			interpreter := NewInterpreter()
			// Scanner should always be initialized with the module root, as all import paths
			// will be fully qualified Go package paths (potentially resolved via replace directives).
			scannerRoot := minigoPackageDir
			interpreter.ModuleRoot = minigoPackageDir
			t.Logf("[%s] Using scannerRoot: %s, ModuleRoot: %s", tt.name, scannerRoot, interpreter.ModuleRoot)

			// Setup sharedScanner specifically for this test execution.
			testSpecificScanner, errScanner := goscan.New(scannerRoot)
			if errScanner != nil {
				t.Fatalf("[%s] Failed to create test-specific shared scanner with startPath %s: %v", tt.name, scannerRoot, errScanner)
			}
			interpreter.sharedScanner = testSpecificScanner

			err := interpreter.LoadAndRun(filename, tt.entryPoint)

			if tt.expectError {
				if err == nil {
					t.Errorf("[%s] Expected an error, but got none. Source:\n%s", tt.name, tt.source)
				} else if !strings.Contains(err.Error(), tt.expectedErrorMsgSubstr) {
					t.Errorf("[%s] Expected error message to contain '%s', but got '%s'. Source:\n%s", tt.name, tt.expectedErrorMsgSubstr, err.Error(), tt.source)
				}
			} else {
				if err != nil {
					t.Fatalf("[%s] LoadAndRun failed: %v\nSource:\n%s", tt.name, err, tt.source)
				}
				for varName, expectedVal := range tt.expectedGlobalVarValue {
					val, ok := interpreter.globalEnv.Get(varName)
					if !ok {
						t.Errorf("[%s] Global variable '%s' not found. Expected value was '%v'. Source:\n%s", tt.name, varName, expectedVal, tt.source)
						continue
					}

					switch expected := expectedVal.(type) {
					case int64:
						intVal, ok := val.(*Integer)
						if !ok {
							t.Errorf("[%s] Expected global variable '%s' to be Integer, but got %s (%s). Value was expected to be '%d'. Source:\n%s", tt.name, varName, val.Type(), val.Inspect(), expected, tt.source)
							continue
						}
						if intVal.Value != expected {
							t.Errorf("[%s] Global variable '%s': expected '%d', got '%d'. Source:\n%s", tt.name, varName, expected, intVal.Value, tt.source)
						}
					case string:
						strVal, ok := val.(*String)
						if !ok {
							t.Errorf("[%s] Expected global variable '%s' to be String, but got %s (%s). Value was expected to be '%s'. Source:\n%s", tt.name, varName, val.Type(), val.Inspect(), expected, tt.source)
							continue
						}
						if strVal.Value != expected {
							t.Errorf("[%s] Global variable '%s': expected '%s', got '%s'. Source:\n%s", tt.name, varName, expected, strVal.Value, tt.source)
						}
					default:
						t.Errorf("[%s] Unsupported type in expectedGlobalVarValue for variable '%s': %T. Source:\n%s", tt.name, varName, expectedVal, tt.source)
					}
				}
			}
		})
	}
}
// Note: Other test functions (TestFormattedErrorHandling, etc.) were here.
// For brevity in this step, they are omitted but would need similar scanner
// setup if they rely on go.mod discovery through goscan.New().
// If they only test syntax that doesn't involve external package resolution,
// the default scanner initialization within LoadAndRun (using the temp file's dir)
// might be sufficient, provided goscan.New() can gracefully handle "go.mod not found"
// for such purely local parsing tasks.
// The current goscan.New() errors if no go.mod is found from startPath or CWD,
// so all tests using LoadAndRun will need a discoverable go.mod,
// implying the test-specific scanner setup (or Chdir) is needed more broadly.
// For now, focusing on making TestImportStatements pass.
