package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper function to run a MiniGo script and check its return value or error.
// Assumes the script's main function returns the value to be checked.
func runMiniGoScriptForTest(t *testing.T, scriptContent string, expectedValue interface{}, expectError bool, expectedErrorMsgSubstring string) {
	t.Helper()

	// Create a temporary directory for this specific test run to isolate files.
	// This helps in managing go.mod if it were needed and keeping testdata clean.
	baseDir, err := os.MkdirTemp("", "minigo_jointest_run_")
	if err != nil {
		t.Fatalf("Failed to create temp base dir for test: %v", err)
	}
	defer os.RemoveAll(baseDir) // Clean up the entire directory afterward.

	// Create the main.mgo file within this temporary directory.
	mainMgoFile := filepath.Join(baseDir, "main.mgo")
	err = os.WriteFile(mainMgoFile, []byte(scriptContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp script file %s: %v", mainMgoFile, err)
	}

	interpreter := NewInterpreter()
	// Set ModuleRoot if your tests rely on go.mod discovery for imports,
	// though for these self-contained strings.Join tests, it might not be strictly necessary
	// if no external packages are imported by the test scripts themselves.
	// For consistency with other tests, and if NewInterpreter setup benefits, let's set it.
	// If tests are simple and don't involve go-scan for imports, this can be simpler.
	// Assuming these tests don't need complex module resolution beyond what NewInterpreter provides by default.
	// interpreter.ModuleRoot = baseDir // Or a more appropriate root if shared packages were involved.

	ctx := context.Background()
	runErr := interpreter.LoadAndRun(ctx, mainMgoFile, "main")

	// Capture stdout/stderr from LoadAndRun if needed for debugging, though not directly used for these value/error checks.
	// (This helper is simplified; TestEvalExternalStructsAndFunctions has more complex stdout capture).

	if expectError {
		if runErr == nil {
			t.Errorf("Expected an error, but got none. Script:\n%s", scriptContent)
			return
		}
		if expectedErrorMsgSubstring != "" && !strings.Contains(runErr.Error(), expectedErrorMsgSubstring) {
			t.Errorf("Expected error message to contain %q, got %q. Script:\n%s", expectedErrorMsgSubstring, runErr.Error(), scriptContent)
		}
	} else {
		if runErr != nil {
			// If an unexpected error occurred, try to get the result from global "result" var for debugging.
			var resultValStr string
			if resObj, ok := interpreter.globalEnv.Get("result"); ok {
				resultValStr = resObj.Inspect()
			} else {
				resultValStr = "<result variable not found>"
			}
			t.Errorf("Unexpected error: %v. Script:\n%s\nGlobal 'result' was: %s", runErr, scriptContent, resultValStr)
			return
		}

		// Assuming the result of the operation is returned by main and captured by LoadAndRun's logging.
		// For direct value checking, it's better if main stores result in a global var.
		// Let's modify tests to store result in a global var "globalResult" and check that.
		globalResultObj, found := interpreter.globalEnv.Get("globalResult")
		if !found {
			t.Errorf("Global variable 'globalResult' not found. Script should assign result to it. Script:\n%s", scriptContent)
			return
		}

		switch expected := expectedValue.(type) {
		case string:
			strResult, ok := globalResultObj.(*String)
			if !ok {
				t.Errorf("Expected globalResult to be STRING, got %s. Script:\n%s", globalResultObj.Type(), scriptContent)
				return
			}
			if strResult.Value != expected {
				t.Errorf("Expected result '%s', got '%s'. Script:\n%s", expected, strResult.Value, scriptContent)
			}
		case int64: // For tests that might return int (e.g. length, or error code if designed that way)
			intResult, ok := globalResultObj.(*Integer)
			if !ok {
				t.Errorf("Expected globalResult to be INTEGER, got %s. Script:\n%s", globalResultObj.Type(), scriptContent)
				return
			}
			if intResult.Value != expected {
				t.Errorf("Expected result '%d', got '%d'. Script:\n%s", expected, intResult.Value, scriptContent)
			}
		default:
			t.Errorf("Unsupported expected type %T in test helper. Script:\n%s", expectedValue, scriptContent)
		}
	}
}

func TestStringsJoinRefactored(t *testing.T) {
	tests := []struct {
		name                      string
		script                    string // MiniGo script content
		expectedValue             interface{}
		expectError               bool
		expectedErrorMsgSubstring string
	}{
		{
			name: "Join multiple strings",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"a", "b", "c"}, ",")
}`,
			expectedValue: "a,b,c",
		},
		{
			name: "Join with different separator",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"foo", "bar", "baz"}, "---")
}`,
			expectedValue: "foo---bar---baz",
		},
		{
			name: "Join empty slice",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{}, ",")
}`,
			expectedValue: "",
		},
		{
			name: "Join slice with one element",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"hello"}, ",")
}`,
			expectedValue: "hello",
		},
		{
			name: "Join slice with empty strings",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"", "", ""}, "-")
}`,
			expectedValue: "--", // Correct: "" + "-" + "" + "-" + ""
		},
		{
			name: "Join with empty separator",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"x", "y", "z"}, "")
}`,
			expectedValue: "xyz",
		},
		{
			name: "Error: First argument not a slice",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join("not a slice", ",")
}`,
			expectError:               true,
			expectedErrorMsgSubstring: "first argument to strings.Join must be a SLICE, got STRING",
		},
		{
			name: "Error: Element in slice is not a string",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"a", 123, "c"}, ",")
}`, // Note: MiniGo currently allows []string{123} if 123 evaluates to an object.
			// The type checking is within strings.Join for elements.
			// The literal []string{...} itself doesn't type check elements strictly at parse time in current MiniGo.
			// However, the `evalCompositeLit` for `[]string` would try to eval `123` to an object.
			// Let's assume `123` becomes an `Integer` object.
			expectError:               true,
			expectedErrorMsgSubstring: "element 1 in the slice for strings.Join must be a STRING, got INTEGER",
		},
		{
			name: "Error: Second argument (separator) not a string",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"a", "b"}, 123)
}`,
			expectError:               true,
			expectedErrorMsgSubstring: "second argument to strings.Join (separator) must be a STRING, got INTEGER",
		},
		{
			name: "Error: Too few arguments (none)",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join()
}`,
			expectError:               true,
			expectedErrorMsgSubstring: "strings.Join expects exactly two arguments (a slice of strings and a separator string), got 0",
		},
		{
			name: "Error: Too few arguments (one)",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"a"})
}`,
			expectError:               true,
			expectedErrorMsgSubstring: "strings.Join expects exactly two arguments (a slice of strings and a separator string), got 1",
		},
		{
			name: "Error: Too many arguments",
			script: `
package main
var globalResult string
func main() {
    globalResult = strings.Join([]string{"a"}, ",", "extra")
}`,
			expectError:               true,
			expectedErrorMsgSubstring: "strings.Join expects exactly two arguments (a slice of strings and a separator string), got 3",
		},
		{
			name: "Using array instead of slice (should fail type check for first arg)",
			// This depends on how evalCompositeLit for [N]Type returns an Array object
			// and if its Type() is ARRAY_OBJ distinct from SLICE_OBJ. It is.
			script: `
package main
var globalResult string
func main() {
    arr := [2]string{"one", "two"}
    globalResult = strings.Join(arr, "-")
}`,
			expectError:               true,
			expectedErrorMsgSubstring: "first argument to strings.Join must be a SLICE, got ARRAY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each tt.script is expected to be a complete runnable MiniGo program,
			// including "package main" and definition of "globalResult" variable.
			runMiniGoScriptForTest(t, tt.script, tt.expectedValue, tt.expectError, tt.expectedErrorMsgSubstring)
		})
	}
}
