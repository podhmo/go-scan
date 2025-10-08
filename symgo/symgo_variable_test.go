package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestIntraPackage_UnexportedConstant(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module myapp",
			"main.go": `
package main

const unexportedConstant = "hello world"

func GetValue() string {
	return unexportedConstant
}
`,
		},
		EntryPoint: "myapp.GetValue",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}

		str := symgotest.AssertAs[*object.String](r, t, 0)
		expected := "hello world"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}
