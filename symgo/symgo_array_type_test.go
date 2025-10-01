package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestArrayTypeExpression(t *testing.T) {
	source := `
package main
func main() {
	_ = []byte("hello")
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/me/myapp",
			"main.go": source,
		},
		EntryPoint: "example.com/me/myapp.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
		// The main check is that it doesn't panic and returns no error.
		// The original test returned no value to check.
		// The main check is that it doesn't panic and returns no error.
		// The original test returned no value to check, and the actual return value
		// seems to be a symbolic placeholder, so we won't assert on it.
	}

	symgotest.Run(t, tc, action)
}
