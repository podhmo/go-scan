package integration_test

import (
	"strings"
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestEvalAssignStmt_Simple(t *testing.T) {
	source := `
package main
func main() int {
	var x int
	x = 10
	return x
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": source,
		},
		EntryPoint: "example.com/m.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		got := symgotest.AssertAs[*object.Integer](r, t, 0)
		if got.Value != 10 {
			t.Errorf("wrong value for 'x', want=10, got=%d", got.Value)
		}
	}
	symgotest.Run(t, tc, action)
}

func TestEvalAssignStmt_Tuple(t *testing.T) {
	source := `
package main

func f() (int, string) {
	return 1, "hello"
}

func main() (int, string) {
	var x int
	var y string
	x, y = f()
	return x, y
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": source,
		},
		EntryPoint: "example.com/m.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("unexpected error: %v", r.Error)
		}

		xVal := symgotest.AssertAs[*object.Integer](r, t, 0)
		if want, got := int64(1), xVal.Value; want != got {
			t.Errorf("x: want %d, got %d", want, got)
		}

		yVal := symgotest.AssertAs[*object.String](r, t, 1)
		if want, got := "hello", yVal.Value; want != got {
			t.Errorf("y: want %q, got %q", want, got)
		}
	}
	symgotest.Run(t, tc, action)
}

func TestEvalAssignStmt_ShortVariableDecl(t *testing.T) {
	source := `
package main

func main() (int, string, bool) {
	x := 10
	y := "hello"
	x, z := 20, true // z is new
	return x, y, z
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": source,
		},
		EntryPoint: "example.com/m.main",
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("unexpected error: %v", r.Error)
		}

		xVal := symgotest.AssertAs[*object.Integer](r, t, 0)
		if want, got := int64(20), xVal.Value; want != got {
			t.Errorf("x: want %d, got %d", want, got)
		}

		yVal := symgotest.AssertAs[*object.String](r, t, 1)
		if want, got := "hello", yVal.Value; want != got {
			t.Errorf("y: want %q, got %q", want, got)
		}

		zVal := symgotest.AssertAs[*object.Boolean](r, t, 2)
		if want, got := true, zVal.Value; want != got {
			t.Errorf("z: want %v, got %v", want, got)
		}
	}
	symgotest.Run(t, tc, action)
}

func TestEvalAssignStmt_ErrorOnUndefined(t *testing.T) {
	source := `
package main
func main() {
	x = 10
}
`
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod":  "module example.com/m",
			"main.go": source,
		},
		EntryPoint:  "example.com/m.main",
		ExpectError: true,
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error == nil {
			t.Fatal("expected an error but got none")
		}
		if !strings.Contains(r.Error.Error(), "identifier not found: x") {
			t.Errorf("expected error message about undefined identifier 'x', but got: %q", r.Error.Error())
		}
	}
	symgotest.Run(t, tc, action)
}