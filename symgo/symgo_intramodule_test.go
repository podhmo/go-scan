package symgo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestIntraModuleCall(t *testing.T) {
	// This test simulates a call to a function in another package within the same module.
	// The symgo engine should recursively evaluate this call, not treat it as a symbolic placeholder.
	source := map[string]string{
		"go.mod": "module example.com/intramodule\ngo 1.22",
		"main/main.go": `
package main
import "example.com/intramodule/helper"
func main() string {
	return helper.GetMessage()
}
`,
		"helper/helper.go": `
package helper
func GetMessage() string {
	return "hello from helper"
}
`,
	}

	scanPolicy := func(path string) bool {
		return strings.HasPrefix(path, "example.com/intramodule")
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/intramodule/main.main",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(scanPolicy),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Apply main function failed: %v", r.Error)
		}

		str := symgotest.AssertAs[*object.String](t, r.ReturnValue)

		expected := "hello from helper"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}
