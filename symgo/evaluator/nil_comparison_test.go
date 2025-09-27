package evaluator_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgotest"
)

func TestNilFunctionComparison(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `package main
func MyFunc(fn func()) {
	if fn != nil {
		fn()
	}
}
func main() {
	MyFunc(nil)
}
`,
		},
		EntryPoint: "example.com/me.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed unexpectedly: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}