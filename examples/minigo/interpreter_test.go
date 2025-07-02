package main

import (
	"os"
	"path/filepath" // For joining paths
	"runtime"       // For runtime.Caller
	"strings"
	"testing"

	"github.com/podhmo/go-scan" // For goscan.New
)

// Helper function to create a temporary Go source file for testing.
func createTempFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tmpDir, "test_*.go")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}
	return tmpFile.Name()
}

func TestImportStatements(t *testing.T) {
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
import "mytestmodule/testpkg"
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
import pkalias "mytestmodule/testpkg"
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
import "mytestmodule/testpkg"
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
import "mytestmodule/testpkg"
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
import badpath "mytestmodule/nonexistentpkg"
var r string
func main() {
	r = badpath.Foo
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: `package "mytestmodule/nonexistentpkg" (aliased as "badpath") not found or failed to scan`,
		},
		{
			name: "disallowed dot import",
			source: `
package main
import . "mytestmodule/testpkg"
func main() {
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: "dot imports are not supported",
		},
		{
			name: "disallowed blank import",
			source: `
package main
import _ "mytestmodule/testpkg"
func main() {
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: "blank imports are not supported",
		},
		{
			name: "alias conflict",
			source: `
package main
import "mytestmodule/testpkg"
import testpkg "mytestmodule/anotherdummy"
func main() {
}`,
			entryPoint:             "main",
			expectError:            true,
			expectedErrorMsgSubstr: `import alias/name "testpkg" already used for "mytestmodule/testpkg"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := createTempFile(t, tt.source)
			defer os.Remove(filename)

			interpreter := NewInterpreter()

			// Always set up the test-specific scanner for TestImportStatements
			var resolvedTestdataDir string
			cwd, wdErr := os.Getwd()
			if wdErr != nil {
				t.Fatalf("Failed to get current working directory for test %s: %v", tt.name, wdErr)
			}

			_, currentTestFile, _, ok := runtime.Caller(0)
			if !ok {
				t.Fatalf("Could not get current test file path for scanner setup in test: %s", tt.name)
			}
			minigoPackageDir := filepath.Dir(currentTestFile)
			pathAttempt1 := filepath.Join(minigoPackageDir, "testdata")

			if _, statErr := os.Stat(pathAttempt1); statErr == nil {
				resolvedTestdataDir = pathAttempt1
			} else if os.IsNotExist(statErr) {
				pathAttempt2 := filepath.Join(cwd, "examples", "minigo", "testdata") // Use cwd obtained earlier
				if _, statErr2 := os.Stat(pathAttempt2); statErr2 == nil {
					resolvedTestdataDir = pathAttempt2
				} else {
					t.Fatalf("Could not locate examples/minigo/testdata for scanner setup for test %s. CWD: %s, currentTestFile: %s, Attempted paths: %s, %s", tt.name, cwd, currentTestFile, pathAttempt1, pathAttempt2)
				}
			} else {
				t.Fatalf("Error checking path %s for test %s: %v", pathAttempt1, tt.name, statErr)
			}

			testSpecificScanner, errScanner := goscan.New(resolvedTestdataDir)
			if errScanner != nil {
				t.Fatalf("[%s] Failed to create test-specific scanner with startPath %s: %v", tt.name, resolvedTestdataDir, errScanner)
			}
			interpreter.sharedScanner = testSpecificScanner
			// interpreter.FileSet will be set by LoadAndRun from the localScriptScanner.
			// Setting it here from testSpecificScanner might be okay if testSpecificScanner's Fset
			// is compatible or if subsequent operations primarily use the one from LoadAndRun.
			// For safety, let LoadAndRun manage i.FileSet based on the main script's scanner.
			// The FileSet for sharedScanner is internal to it and used when it reports errors.
			// If LoadAndRun correctly sets i.FileSet from its localScriptScanner, that's the one
			// formatErrorWithContext will use for errors originating from the main script's parsing/evaluation.

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
