package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgotest"
)

func TestMapTypeExpression(t *testing.T) {
	source := `
package main
func main() {
	_ = map[string]int(nil)
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
	}

	symgotest.Run(t, tc, action)
}
