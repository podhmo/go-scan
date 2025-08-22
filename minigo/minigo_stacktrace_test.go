package minigo_test

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestStackTrace(t *testing.T) {
	tests := []struct {
		name                 string
		script               string
		filename             string // Add filename to struct
		expectedToContain    []string
		expectedToNotContain []string
	}{
		{
			name:     "anonymous function call",
			filename: "test.mgo",
			script: `
package main

func caller() {
	func() {
		var x = 1 + "a" // runtime error
	}()
}

var _ = caller()
`,
			expectedToContain: []string{
				"runtime error: type mismatch: INTEGER + STRING",
				"var x = 1 + \"a\" // runtime error",
				"in <anonymous>",
				"in caller",
			},
		},
		{
			name:     "function assigned to variable",
			filename: "test.mgo",
			script: `
package main

func caller() {
	var f = func() {
		var x = 1 + "a" // runtime error
	}
	f()
}

var _ = caller()
`,
			expectedToContain: []string{
				"runtime error: type mismatch: INTEGER + STRING",
				"var x = 1 + \"a\" // runtime error",
				"in f",
				"in caller",
			},
		},
		{
			name:     "named function call",
			filename: "test.mgo",
			script: `
package main

func erroringFunc() {
	var x = 1 + "a" // runtime error
}

func namedCaller() {
	erroringFunc()
}

var _ = namedCaller()
`,
			expectedToContain: []string{
				"runtime error: type mismatch: INTEGER + STRING",
				"var x = 1 + \"a\" // runtime error",
				"in erroringFunc",
				"in namedCaller",
			},
		},
		{
			name:     "non-existent file",
			filename: "non-existent.mgo",
			script: `
package main

func level1() {
	level2()
}

func level2() {
	var x = 1 + "a"
}

var _ = level1()
`,
			expectedToContain: []string{
				"runtime error: type mismatch: INTEGER + STRING",
				"non-existent.mgo:9:10:",
				"in level2",
				"non-existent.mgo:5:2:",
				"in level1",
			},
			expectedToNotContain: []string{
				"[Error opening source file",
				"var x = 1 + \"a\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp := newTestInterpreter(t)
			var err error

			// Create a dummy file for tests that need to read source
			if tt.filename == "test.mgo" {
				if err := os.WriteFile("test.mgo", []byte(tt.script), 0644); err != nil {
					t.Fatalf("Failed to create dummy file: %v", err)
				}
				defer os.Remove("test.mgo")
			}

			err = interp.LoadFile(tt.filename, []byte(tt.script))
			if err != nil {
				t.Fatalf("LoadFile() unexpected error = %v", err)
			}

			_, err = interp.Eval(context.Background())
			if err == nil {
				t.Fatalf("Eval() expected an error, but got nil")
			}

			// The 'error' returned by Eval contains the formatted stack trace from
			// object.Error's Inspect() method.
			errMsg := err.Error()
			for _, expected := range tt.expectedToContain {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("error message should contain %q, but it was:\n---\n%s\n---", expected, errMsg)
				}
			}
			for _, unexpected := range tt.expectedToNotContain {
				if strings.Contains(errMsg, unexpected) {
					t.Errorf("error message should NOT contain %q, but it was:\n---\n%s\n---", unexpected, errMsg)
				}
			}
		})
	}
}
