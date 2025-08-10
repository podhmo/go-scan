package minigo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
)

func TestStackTrace(t *testing.T) {
	tests := []struct {
		name              string
		script            string
		expectedToContain []string
	}{
		{
			name: "anonymous function call",
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
				"in <anonymous>",
				"in caller",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interp, err := minigo.NewInterpreter()
			if err != nil {
				t.Fatalf("NewInterpreter() error = %v", err)
			}

			err = interp.LoadFile("test.mgo", []byte(tt.script))
			if err != nil {
				t.Fatalf("LoadFile() unexpected error = %v", err)
			}

			_, err = interp.Eval(context.Background())
			if err == nil {
				t.Fatalf("Eval() expected an error, but got nil")
			}

			errMsg := err.Error()
			for _, expected := range tt.expectedToContain {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("error message should contain %q, but it was:\n---\n%s\n---", expected, errMsg)
				}
			}
		})
	}
}
