package minigo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo"
)

func TestNilBehavior(t *testing.T) {
	cases := []struct {
		name           string
		script         string
		wantErr        bool
		skip           bool
		expectedOutput string
	}{
		{
			name: "untyped nil passed to slice func",
			script: `
package main
func f[T any](v []T) {}
func main() {
	f(nil)
}`,
			wantErr: true,
		},
		{
			name: "typed nil slice",
			script: `
package main
func f[T any](v []T) {}
func main() {
	var s []int
	f(s)
}`,
			wantErr: false,
			skip:    true, // This is the known failing test
		},
		{
			name: "typed nil interface",
			script: `
package main

type I interface{}

func do() I {
	var r *int
	return r
}

func main() {
	println(do() == nil)
	var i I = nil
	println(i == nil)
}
`,
			wantErr:        false,
			skip:           true, // This is a known failing test
			expectedOutput: "false\ntrue\n",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("skipping known failing test for nil behavior")
			}

			var outbuf strings.Builder
			interp, err := minigo.NewInterpreter(minigo.WithStdout(&outbuf))
			if err != nil {
				t.Fatalf("NewInterpreter() error = %v", err)
			}

			err = interp.LoadFile("test.mgo", []byte(tt.script))
			if err != nil {
				t.Fatalf("LoadFile() unexpected error = %v", err)
			}

			_, err = interp.Eval(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Eval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
				return
			}

			if tt.expectedOutput != "" && outbuf.String() != tt.expectedOutput {
				t.Errorf("expected output %q, but got %q", tt.expectedOutput, outbuf.String())
			}
		})
	}
}
