package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymgo_LocalConstantResolution(t *testing.T) {
	source := map[string]string{
		"go.mod": `
module example.com/main
go 1.21
`,
		"main.go": `
package main

func GetLocalConstant() string {
	const MyLocalConstant = "hello from a local constant"
	return MyLocalConstant
}
`,
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/main.GetLocalConstant",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}

		str := symgotest.AssertAs[*object.String](r, t, 0)

		expected := "hello from a local constant"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}
