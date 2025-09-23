package evaluator_test

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestIfOkAssertion_OutOfPolicy(t *testing.T) {
	source := map[string]string{
		"go.mod": `
module example.com/myapp

go 1.22
`,
		"main.go": `
package main

import "example.com/myapp/other"

func inspect(v interface{}) {}

func check(i interface{}) {
	if v, ok := i.(other.Person); ok {
		inspect(v)
	}
}
`,
		"other/other.go": `
package other

type Person struct {
	Name string
}
`,
	}

	var inspectedValue symgo.Object

	// Define a scan policy that excludes the "other" package.
	policy := func(path string) bool {
		return path == "example.com/myapp"
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/myapp.check",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(policy),
			symgotest.WithSetup(func(interp *symgo.Interpreter) error {
				interp.RegisterIntrinsic("example.com/myapp.inspect", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
					if len(args) > 0 {
						inspectedValue = args[0]
					}
					return nil
				})
				return nil
			}),
		},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}

		if inspectedValue == nil {
			t.Fatalf("inspect() was not called")
		}

		// The inspected value should be a symbolic placeholder because other.Person is out-of-policy.
		placeholder, ok := inspectedValue.(*object.SymbolicPlaceholder)
		if !ok {
			t.Errorf("expected inspected value to be a SymbolicPlaceholder, but got %T", inspectedValue)
		}

		// Check that the placeholder has the correct type information.
		if placeholder.TypeInfo() == nil {
			t.Errorf("placeholder has no TypeInfo")
		} else {
			ti := placeholder.TypeInfo()
			if ti.PkgPath != "example.com/myapp/other" || ti.Name != "Person" {
				t.Errorf("placeholder has wrong type info: expected example.com/myapp/other.Person, got %s.%s", ti.PkgPath, ti.Name)
			}
			if !ti.Unresolved {
				t.Errorf("expected placeholder's type to be marked as Unresolved")
			}
		}
	})
}

func TestIfOkAssertion_InPolicy(t *testing.T) {
	source := map[string]string{
		"go.mod": `
module example.com/myapp

go 1.22
`,
		"main.go": `
package main

import "example.com/myapp/other"

func inspect(v interface{}) {}

func check(i interface{}) {
	// This assertion should succeed, and v should be a concrete type.
	if v, ok := i.(other.Person); ok {
		inspect(v)
	}
}
`,
		"other/other.go": `
package other

type Person struct {
	Name string
}
`,
	}

	var inspectedValue symgo.Object

	// Define a scan policy that includes both packages.
	policy := func(path string) bool {
		return path == "example.com/myapp" || path == "example.com/myapp/other"
	}

	// The input to the 'check' function will be a concrete Person instance.
	args := []symgo.Object{
		&object.Instance{
			TypeName: "example.com/myapp/other.Person",
			Fields:   map[string]symgo.Object{"Name": &object.String{Value: "dave"}},
		},
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/myapp.check",
		Args:       args,
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(policy),
			symgotest.WithSetup(func(interp *symgo.Interpreter) error {
				interp.RegisterIntrinsic("example.com/myapp.inspect", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
					if len(args) > 0 {
						inspectedValue = args[0]
					}
					return nil
				})
				return nil
			}),
		},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}

		if inspectedValue == nil {
			t.Fatalf("inspect() was not called")
		}

		placeholder, ok := inspectedValue.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("expected inspected value to be a SymbolicPlaceholder, got %T", inspectedValue)
		}

		if placeholder.TypeInfo() == nil {
			t.Errorf("placeholder has no TypeInfo")
		} else {
			ti := placeholder.TypeInfo()
			if ti.PkgPath != "example.com/myapp/other" || ti.Name != "Person" {
				t.Errorf("placeholder has wrong type info: expected example.com/myapp/other.Person, got %s.%s", ti.PkgPath, ti.Name)
			}
			if ti.Unresolved {
				t.Errorf("expected placeholder's type to be resolved, but it was not")
			}
		}

		if placeholder.Original == nil {
			t.Errorf("placeholder does not link back to the original object")
		} else {
			if _, ok := placeholder.Original.(*object.Instance); !ok {
				t.Errorf("placeholder's original is not an *object.Instance, but %T", placeholder.Original)
			}
		}
	})
}

func TestTypeSwitch_OutOfPolicy(t *testing.T) {
	source := map[string]string{
		"go.mod": `
module example.com/myapp

go 1.22
`,
		"main.go": `
package main

import "example.com/myapp/other"

func inspect(v interface{}) {}

func check(i interface{}) {
	switch v := i.(type) {
	case other.Person:
		inspect(v)
	}
}
`,
		"other/other.go": `
package other

type Person struct {
	Name string
}
`,
	}

	var inspectedValue symgo.Object
	policy := func(path string) bool {
		return path == "example.com/myapp"
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/myapp.check",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(policy),
			symgotest.WithSetup(func(interp *symgo.Interpreter) error {
				interp.RegisterIntrinsic("example.com/myapp.inspect", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
					if len(args) > 0 {
						inspectedValue = args[0]
					}
					return nil
				})
				return nil
			}),
		},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}
		if inspectedValue == nil {
			t.Fatal("inspect() was not called")
		}
		placeholder, ok := inspectedValue.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("expected inspected value to be a SymbolicPlaceholder, but got %T", inspectedValue)
		}
		if ti := placeholder.TypeInfo(); ti == nil {
			t.Error("placeholder has no TypeInfo")
		} else {
			if ti.PkgPath != "example.com/myapp/other" || ti.Name != "Person" {
				t.Errorf("placeholder has wrong type info: expected example.com/myapp/other.Person, got %s.%s", ti.PkgPath, ti.Name)
			}
			if !ti.Unresolved {
				t.Errorf("expected placeholder's type to be marked as Unresolved")
			}
		}
	})
}

func TestTypeSwitch_InPolicy(t *testing.T) {
	source := map[string]string{
		"go.mod": `
module example.com/myapp

go 1.22
`,
		"main.go": `
package main

import "example.com/myapp/other"

func inspect(v interface{}) {}

func check(i interface{}) {
	switch v := i.(type) {
	case other.Person:
		inspect(v)
	}
}
`,
		"other/other.go": `
package other

type Person struct {
	Name string
}
`,
	}

	var inspectedValue symgo.Object
	policy := func(path string) bool {
		return path == "example.com/myapp" || path == "example.com/myapp/other"
	}
	args := []symgo.Object{
		&object.Instance{
			TypeName: "example.com/myapp/other.Person",
			Fields:   map[string]symgo.Object{"Name": &object.String{Value: "dave"}},
		},
	}

	tc := symgotest.TestCase{
		Source:     source,
		EntryPoint: "example.com/myapp.check",
		Args:       args,
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(policy),
			symgotest.WithSetup(func(interp *symgo.Interpreter) error {
				interp.RegisterIntrinsic("example.com/myapp.inspect", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
					if len(args) > 0 {
						inspectedValue = args[0]
					}
					return nil
				})
				return nil
			}),
		},
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("symgotest.Run failed: %+v", r.Error)
		}
		if inspectedValue == nil {
			t.Fatal("inspect() was not called")
		}
		placeholder, ok := inspectedValue.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("expected inspected value to be a SymbolicPlaceholder, but got %T", inspectedValue)
		}
		if ti := placeholder.TypeInfo(); ti == nil {
			t.Error("placeholder has no TypeInfo")
		} else {
			if ti.PkgPath != "example.com/myapp/other" || ti.Name != "Person" {
				t.Errorf("placeholder has wrong type info: expected example.com/myapp/other.Person, got %s.%s", ti.PkgPath, ti.Name)
			}
			if ti.Unresolved {
				t.Errorf("expected placeholder's type to be resolved, but it was not")
			}
		}
	})
}
