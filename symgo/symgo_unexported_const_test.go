package symgo_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestSymgo_UnexportedConstantResolution(t *testing.T) {
	tc := symgotest.TestCase{
		WorkDir: "main",
		Source: map[string]string{
			"main/go.mod": `
module example.com/main
go 1.21
replace example.com/helper => ../helper
`,
			"main/main.go": `
package main
import "example.com/helper"
func CallHelper() string {
    return helper.GetUnexportedConstant()
}
`,
			"helper/go.mod": `
module example.com/helper
go 1.21
`,
			"helper/helper.go": `
package helper
const myUnexportedConstant = "hello from unexported"
func GetUnexportedConstant() string {
	return myUnexportedConstant
}
`,
		},
		EntryPoint: "example.com/main.CallHelper",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(pkgPath string) bool {
				return strings.HasPrefix(pkgPath, "example.com/")
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}
		str := symgotest.AssertAs[*object.String](r, t, 0)
		expected := "hello from unexported"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestSymgo_IntraPackageConstantResolution(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/main\ngo 1.21\n",
			"main.go": `
package main
import "fmt"
const myConstant = "hello intra-package"

func formatConstant() string {
	return fmt.Sprintf("value is %s", myConstant)
}

func main() {
	_ = formatConstant()
}
`,
		},
		EntryPoint: "example.com/main.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestSymgo_UnexportedConstantResolution_NestedCall(t *testing.T) {
	tc := symgotest.TestCase{
		WorkDir: "loglib",
		Source: map[string]string{
			"loglib/go.mod": `
module example.com/loglib
go 1.21
replace example.com/driver => ../driver
`,
			"loglib/context.go": `
package loglib
import "example.com/driver"
func FuncA() string {
	return driver.FuncB()
}
`,
			"driver/go.mod": `
module example.com/driver
go 1.21
`,
			"driver/db.go": `
package driver
const privateConst = "hello from private"
func FuncB() string {
	return privateConst
}
`,
		},
		EntryPoint: "example.com/loglib.FuncA",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(pkgPath string) bool {
				return strings.HasPrefix(pkgPath, "example.com/")
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}
		str := symgotest.AssertAs[*object.String](r, t, 0)
		expected := "hello from private"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}

func TestSymgo_UnexportedConstantResolution_NestedMethodCall(t *testing.T) {
	tc := symgotest.TestCase{
		WorkDir: "main",
		Source: map[string]string{
			"main/go.mod": `
module example.com/main
go 1.21
replace example.com/remotedb => ../remotedb
`,
			"main/main.go": `
package main
import "example.com/remotedb"
func DoWork() string {
	var client remotedb.Client
	return client.GetValue()
}
`,
			"remotedb/go.mod": `
module example.com/remotedb
go 1.21
`,
			"remotedb/db.go": `
package remotedb
const secretKey = "unexported-secret-key"
type Client struct{}
func (c *Client) GetValue() string {
	return secretKey
}
`,
		},
		EntryPoint: "example.com/main.DoWork",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(pkgPath string) bool {
				return strings.HasPrefix(pkgPath, "example.com/")
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}
		str := symgotest.AssertAs[*object.String](r, t, 0)
		expected := "unexported-secret-key"
		if str.Value != expected {
			t.Errorf("expected result to be %q, but got %q", expected, str.Value)
		}
	}

	symgotest.Run(t, tc, action)
}
