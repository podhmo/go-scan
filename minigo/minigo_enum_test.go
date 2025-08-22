package minigo

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo/object"
)

func TestImportedEnumComparison(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		expected     object.Object
		wantErrorMsg string
	}{
		{
			name: "integer enum",
			script: `
package main
import "github.com/podhmo/go-scan/minigo/testdata/enum"
var result = enum.Active == 1
`,
			expected: object.TRUE,
		},
		{
			name: "integer enum (iota)",
			script: `
package main
import "github.com/podhmo/go-scan/minigo/testdata/enum"
var result = enum.UntypedActive == 1
`,
			expected: object.TRUE,
		},
		{
			name: "string enum",
			script: `
package main
import "github.com/podhmo/go-scan/minigo/testdata/enum"
var result = enum.StringStatusOK == "ok"
`,
			expected: object.TRUE,
		},
		{
			name: "undefined member",
			script: `
package main
import "github.com/podhmo/go-scan/minigo/testdata/enum"
var result = enum.Undefined
`,
			wantErrorMsg: "undefined: enum.Undefined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create interpreter with Go module resolution enabled
			interp := newTestInterpreter(t)
			var err error

			// The main test logic
			if err = interp.LoadFile("test.mgo", []byte(tt.script)); err != nil {
				t.Fatalf("LoadFile() failed: %v", err)
			}
			_, err = interp.Eval(context.Background())

			if tt.wantErrorMsg != "" {
				if err == nil {
					val, ok := interp.globalEnv.Get("result")
					if ok {
						t.Fatalf("expected error, but got none. result was: %v", val.Inspect())
					}
					t.Fatalf("expected error, but got none. result was not even set.")
				}
				if !strings.Contains(err.Error(), tt.wantErrorMsg) {
					t.Errorf("wrong error message.\n- want: %q\n- got:  %q", tt.wantErrorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("minigo.Eval() returned an error: %v", err)
			}

			val, ok := interp.globalEnv.Get("result")
			if !ok {
				t.Fatalf("variable 'result' not found in environment")
			}

			if val.Type() != tt.expected.Type() {
				t.Errorf("wrong result type. got=%s, want=%s", val.Type(), tt.expected.Type())
			}
			if val.Inspect() != tt.expected.Inspect() {
				t.Errorf("wrong result value. got=%s, want=%s", val.Inspect(), tt.expected.Inspect())
			}
		})
	}
}
