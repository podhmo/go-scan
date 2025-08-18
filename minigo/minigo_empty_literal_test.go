package minigo_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/minigo"
)

func TestEmptyLiteralInference(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{
			name: "empty slice literal",
			script: `
package main
func f[T any](v []T) {}
func main() {
	f([]int{})
}`,
			wantErr: false,
		},
		{
			name: "empty map literal",
			script: `
package main
func f[K comparable, V any](v map[K]V) {}
func main() {
	f(map[string]int{})
}`,
			wantErr: false,
		},
	}

	for _, tt := range cases {
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
			if (err != nil) != tt.wantErr {
				t.Errorf("Eval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err == nil {
				t.Errorf("Expected an error, but got nil")
			}
		})
	}
}
