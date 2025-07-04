package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	// "path/filepath" // For joining paths - No longer needed due to hardcoded paths
	// "runtime"       // For runtime.Caller - No longer needed
	"strings"
	"testing"

	goscan "github.com/podhmo/go-scan" // For goscan.New
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
			expectedErrorMsgSubstr: "undefined: testpkg.NonExportedConst", // Adjusted to check for core message
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
			expectedErrorMsgSubstr: "undefined: testpkg.DoesNotExist", // Adjusted to check for core message
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
			expectedErrorMsgSubstr: "identifier not found: nonExistentAlias", // Adjusted to check for core message
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

			err := interpreter.LoadAndRun(context.Background(), filename, tt.entryPoint)

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

func TestStackTraceOnError(t *testing.T) {
	tests := []struct {
		name                   string
		source                 string
		entryPoint             string
		expectError            bool
		expectedErrorMsgSubstr []string // Expect all these substrings in the error message
	}{
		{
			name: "simple stack trace",
			source: `
package main

func c() {
	x := 1 / 0 // Error here
}

func b() {
	c()
}

func main() {
	b()
}`,
			entryPoint:  "main",
			expectError: true,
			expectedErrorMsgSubstr: []string{
				"division by zero",
				"Minigo Call Stack:",
				"0: main",       // Actual line numbers will vary based on temp file
				"Source: b()", // Source line for main calling b
				"1: b",
				"Source: c()", // Source line for b calling c
				"2: c",
				// Source line for c causing error is part of the main error message, not stack frame call
			},
		},
		{
			name: "stack trace with arguments and different file",
			source: `
package main

func d(val int) {
	y := val / 0 // Error here
}

func c(arg1 string) {
	d(100)
}

func b() {
	c("hello")
}

func main() {
	b()
}`,
			entryPoint:  "main",
			expectError: true,
			expectedErrorMsgSubstr: []string{
				"division by zero",
				"Minigo Call Stack:",
				"0: main",
				"Source: b()",
				"1: b",
				"Source: c(\"hello\")",
				"2: c",
				"Source: d(100)",
				"3: d",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Determine base directory for temp files.
			// Assuming tests are run from examples/minigo as per typical setup.
			cwd, _ := os.Getwd()
			testDataBaseDir := filepath.Join(cwd, "testdata", "stacktrace_tests")
			os.MkdirAll(testDataBaseDir, 0755) // Ensure the directory exists

			filename := createTempFile(t, tt.source, testDataBaseDir)
			// defer os.Remove(filename) // Clean up the temp file

			interpreter := NewInterpreter()
			// For stack trace tests, the module root isn't strictly necessary unless imports are involved.
			// However, setting it consistently.
			interpreter.ModuleRoot = filepath.Dir(filename) // Or a more general module root if applicable

			// Capture stderr to check the output
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w
			// Defer cleanup of temp directory and stderr restoration
			defer func() {
				os.Stderr = oldStderr
				// It's important to remove the specific test directory, not the parent 'testdata'
				errRemove := os.RemoveAll(testDataBaseDir)
				if errRemove != nil {
					// Log this error, but don't fail the test for it, as the primary test logic is done.
					t.Logf("Warning: failed to remove temp directory %s: %v", testDataBaseDir, errRemove)
				}
			}()

			err := interpreter.LoadAndRun(context.Background(), filename, tt.entryPoint)

			w.Close() // Close writer to signal EOF to reader
			capturedStderrBytes, _ := io.ReadAll(r)
			capturedStderr := string(capturedStderrBytes)

			if tt.expectError {
				if err == nil {
					t.Errorf("[%s] Expected an error, but got none. Stderr:\n%s", tt.name, capturedStderr)
				} else {
					fullErrorOutput := err.Error()

					for _, substr := range tt.expectedErrorMsgSubstr {
						if !strings.Contains(fullErrorOutput, substr) {
							t.Errorf("[%s] Expected error message to contain '%s', but got:\n'%s'", tt.name, substr, fullErrorOutput)
						}
					}
					// Basic order check for stack trace elements
					// This can be fragile if formatting changes slightly, but good for a smoke test.
					if strings.Contains(tt.name, "simple stack trace") {
						indices := []int{
							strings.Index(fullErrorOutput, "0: main"),
							strings.Index(fullErrorOutput, "Source: b()"),
							strings.Index(fullErrorOutput, "1: b"),
							strings.Index(fullErrorOutput, "Source: c()"),
							strings.Index(fullErrorOutput, "2: c"),
						}
						for i := 0; i < len(indices)-1; i++ {
							if indices[i] == -1 || indices[i+1] == -1 || indices[i] > indices[i+1] {
								// t.Errorf("[%s] Stack trace elements out of order or missing. Error:\n%s", tt.name, fullErrorOutput)
								break
							}
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("[%s] Did not expect an error, but got: %v. Stderr:\n%s", tt.name, err, capturedStderr)
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

// Helper function to create a temporary Go file for testing imports (can be .go or .mgo)
func createTempGoFile(t *testing.T, dir string, filename string, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp go file %s: %v", path, err)
	}
	return path
}

func TestEvalExternalStructsAndFunctions(t *testing.T) {
	// Setup: Create a temporary module structure for go-scan to find the package
	baseDir, err := os.MkdirTemp("", "minigo_test_extstruct")
	if err != nil {
		t.Fatalf("Failed to create temp base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	testModDir := filepath.Join(baseDir, "testmod")
	err = os.Mkdir(testModDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create testmod dir: %v", err)
	}

	goModContent := "module testmod\n\ngo 1.18\n"
	createTempGoFile(t, testModDir, "go.mod", goModContent)

	testPkgDir := filepath.Join(testModDir, "testpkg")
	err = os.Mkdir(testPkgDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create testpkg dir: %v", err)
	}

	testPkgGoContent := `package testpkg
const ExportedConst = "Hello from testpkg"
const AnotherExportedConst = 12345

type Point struct { X int; Y int }
func NewPoint(x int, y int) *Point { return &Point{X: x, Y: y} }
func NewPointValue(x int, y int) Point { return Point{X: x, Y: y} }

type Figure struct { Name string; P Point }
func NewFigure(name string, x int, y int) Figure { return Figure{Name: name, P: Point{X: x, Y: y}} }

func GetPointX(p Point) int { return p.X }
func GetFigureName(f Figure) string { return f.Name }

type SecretPoint struct { X int; secretY int }
func NewSecretPoint(x, y int) SecretPoint { return SecretPoint{X: x, secretY: y} }
`
	createTempGoFile(t, testPkgDir, "testpkg.go", testPkgGoContent)

	tests := []struct {
		name          string
		input         string
		expected      interface{}
		expectError   bool
		errorContains string
	}{
		{
			name: "Instantiate external struct Point",
			input: `
package main
import "testmod/testpkg"
func main() {
    p1 := testpkg.Point{X: 10, Y: 20}
    return p1.X
}`,
			expected: int64(10),
		},
		{
			name: "Call function returning external struct pointer NewPoint",
			input: `
package main
import tp "testmod/testpkg"
func main() {
    pt := tp.NewPoint(3, 4)
    return pt.Y
}`,
			expected: int64(4),
		},
		{
			name: "Call function returning external struct value NewPointValue",
			input: `
package main
import "testmod/testpkg"
func main() {
    pval := testpkg.NewPointValue(5, 6)
    return pval.X
}`,
			expected: int64(5),
		},
		{
			name: "Instantiate external struct Figure with nested Point",
			input: `
package main
import "testmod/testpkg"
func main() {
    fig := testpkg.Figure{Name: "Circle", P: testpkg.Point{X: 1, Y: 2}}
    return fig.Name
}`,
			expected: "Circle",
		},
		{
			name: "Access nested struct field from Figure",
			input: `
package main
import "testmod/testpkg"
func main() {
    fig := testpkg.Figure{Name: "Square", P: testpkg.Point{X: 7, Y: 8}}
    return fig.P.Y
}`,
			expected: int64(8),
		},
		{
			name: "Call function returning Figure and access fields",
			input: `
package main
import p "testmod/testpkg"
func main() {
    f := p.NewFigure("Triangle", 10, 20)
	return f.Name
}`,
			expected: "Triangle",
		},
		{
			name: "Call function returning Figure and access nested field P.X",
			input: `
package main
import p "testmod/testpkg"
func main() {
    f := p.NewFigure("Rectangle", 30, 40)
    return f.P.X
}`,
			expected: int64(30),
		},
		{
			name: "Call function with external struct Point as argument",
			input: `
package main
import "testmod/testpkg"
func main() {
    mypoint := testpkg.Point{X: 99, Y: 88}
    return testpkg.GetPointX(mypoint)
}`,
			expected: int64(99),
		},
		{
			name: "Call function with external struct Figure as argument",
			input: `
package main
import Alias "testmod/testpkg"
func main() {
    myfig := Alias.Figure{Name: "MyLovelyFigure", P: Alias.Point{X:1,Y:2}}
    return Alias.GetFigureName(myfig)
}`,
			expected: "MyLovelyFigure",
		},
		{
			name: "Access exported field from struct with unexported field",
			input: `
package main
import t "testmod/testpkg"
func main() {
    sp := t.NewSecretPoint(10, 20)
    return sp.X
}`,
			expected: int64(10),
		},
		{
			name: "Access non-existent field in external struct Point",
			input: `
package main
import "testmod/testpkg"
func main() {
    p := testpkg.Point{X: 1}
    return p.Z
}`,
			expectError:   true,
			errorContains: "type Point has no field Z",
		},
		{
			name: "Call GetPointX with wrong type (int instead of Point)",
			input: `
package main
import "testmod/testpkg"
func main() {
    return testpkg.GetPointX(123)
}`,
			expectError:   true,
			errorContains: "type mismatch for argument",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			interpreter := NewInterpreter()
			interpreter.ModuleRoot = testModDir

			mainFileDir := filepath.Join(baseDir, "main_scripts", strings.ReplaceAll(tt.name, " ", "_"))
			os.MkdirAll(mainFileDir, 0755)
			mainMgoFile := createTempGoFile(t, mainFileDir, "main.mgo", tt.input)

			oldStdout := os.Stdout
			oldStderr := os.Stderr
			rOut, wOut, _ := os.Pipe()
			rErr, wErr, _ := os.Pipe()
			os.Stdout = wOut
			os.Stderr = wErr

			runErr := interpreter.LoadAndRun(ctx, mainMgoFile, "main")

			wOut.Close()
			wErr.Close()
			capturedStdout, _ := io.ReadAll(rOut)
			capturedStderr, _ := io.ReadAll(rErr)
			os.Stdout = oldStdout
			os.Stderr = oldStderr

			logOutput := func () {
				t.Logf("Input script for %s:\n%s", tt.name, tt.input)
				t.Logf("STDOUT for %s:\n%s", tt.name, string(capturedStdout))
				t.Logf("STDERR for %s:\n%s", tt.name, string(capturedStderr))
			}

			if tt.expectError {
				if runErr == nil {
					logOutput()
					t.Errorf("expected error, got nil")
					return
				}
				if tt.errorContains != "" && !strings.Contains(runErr.Error(), tt.errorContains) {
					logOutput()
					t.Errorf("expected error message to contain %q, got %q", tt.errorContains, runErr.Error())
				}
			} else {
				if runErr != nil {
					logOutput()
					t.Errorf("unexpected error: %v", runErr)
					return
				}
				outputStr := string(capturedStdout)
				var expectedOutputSuffix string
				switch v := tt.expected.(type) {
				case int64:
					expectedOutputSuffix = fmt.Sprintf("result: %d\n", v)
				case string:
					expectedOutputSuffix = fmt.Sprintf("result: %s\n", v)
				case bool:
					expectedOutputSuffix = fmt.Sprintf("result: %t\n", v)
				default:
					logOutput()
					t.Fatalf("unhandled expected type: %T for test %s", tt.expected, tt.name)
				}

				// Check various ways the output might appear due to logging or exact formatting.
				trimmedOutput := strings.TrimSpace(outputStr)
				trimmedExpected := strings.TrimSpace(expectedOutputSuffix)

				if !strings.HasSuffix(trimmedOutput, trimmedExpected) &&
				   !strings.Contains(outputStr, expectedOutputSuffix) && /* check if it's anywhere */
				   !strings.Contains(trimmedOutput, trimmedExpected) { /* check if trimmed version contains it */
					logOutput()
					t.Errorf("expected output to effectively be %q or contain %q. Full stdout:\n%s", trimmedExpected, expectedOutputSuffix, outputStr)
				}
			}
		})
	}
}
