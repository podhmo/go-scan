package minigo_test

import (
	"context"
	"testing"
)

func TestTypedNilInference(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{
			name: "nil slice variable passed to generic function",
			script: `
package main
func f[T any](v []T) {}
func main() {
	var s []int
	f(s)
}`,
			wantErr: false,
		},
		{
			name: "nil map variable passed to generic function",
			script: `
package main
func f[K comparable, V any](v map[K]V) {}
func main() {
	var m map[string]int
	f(m)
}`,
			wantErr: false,
		},
		{
			name: "nil pointer variable passed to generic function",
			script: `
package main
type S struct {}
func f[T any](v *T) {}
func main() {
	var p *S
	f(p)
}`,
			wantErr: false,
		},
		{
			name: "comparison between nil and typed nil",
			script: `
package main
func main() {
	var s []int
	if s != nil {
		panic("s != nil should be false")
	}
	if !(s == nil) {
		panic("!(s == nil) should be false")
	}
}`,
			wantErr: false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			interp := newTestInterpreter(t)

			err := interp.LoadFile("test.mgo", []byte(tt.script))
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
