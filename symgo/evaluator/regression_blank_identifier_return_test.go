package evaluator_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestRegressionBlankIdentifierAssignmentShouldNotBeReturnValue(t *testing.T) {
	source := `
package main

func returnsInt() int {
	return 100
}

func main() string {
	_ = returnsInt()
	return "hello"
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/main",
			"main.go": source,
		},
		EntryPoint: "example.com/main.main",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		// We expect the return value to be a string "hello".
		// The bug would cause it to be an int 100 from returnsInt().
		s := symgotest.AssertAs[*object.String](r, t, 0)
		if s.Value != "hello" {
			t.Errorf(`expected return value to be "hello", but got %q`, s.Value)
		}
	})
}