package symgo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestSymgo_ExtraModule_ConstantResolution(t *testing.T) {
	source := map[string]string{
		"main/go.mod": `
module example.com/main
go 1.21
replace example.com/helper => ../helper
`,
		"main/main.go": `
package main
import "example.com/helper"
func GetConstant() string {
    return helper.MyConstant
}
`,
		"helper/go.mod": `
module example.com/helper
go 1.21
`,
		"helper/helper.go": `
package helper
const MyConstant = "hello from another module"
`,
	}

	scanPolicy := func(path string) bool {
		return strings.HasPrefix(path, "example.com/main") || strings.HasPrefix(path, "example.com/helper")
	}

	tc := symgotest.TestCase{
		Source:     source,
		WorkDir:    "main",
		EntryPoint: "example.com/main.GetConstant",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(scanPolicy),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %v", r.Error)
		}

		str := symgotest.AssertAs[*object.String](r, t, 0)

		expected := "hello from another module"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}
