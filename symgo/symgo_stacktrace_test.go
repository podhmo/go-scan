package symgo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgotest"
)

func TestStackTrace(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/me",
			"main.go": `
package main

func errorFunc() {
	var x = 1
	x()
}

func caller() {
	errorFunc()
}

func main() {
	caller()
}
`,
		},
		EntryPoint:  "example.com/me.main",
		ExpectError: true,
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error == nil {
			t.Fatal("Expected an error, but got nil")
		}

		errMsg := r.Error.Error()
		t.Logf("Full error message:\n---\n%s\n---", errMsg)

		expectedToContain := []string{
			"symgo runtime error: not a function: INTEGER",
			"x()",
			"in errorFunc",
			"in caller",
		}

		for _, expected := range expectedToContain {
			if !strings.Contains(errMsg, expected) {
				t.Errorf("error message should contain %q, but it was:\n---\n%s\n---", expected, errMsg)
			}
		}
	}

	symgotest.Run(t, tc, action)
}
