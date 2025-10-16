package symgo_test

import (
	"testing"

	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgo/symgotest"
)

func TestEval_LocalTypeDefinition(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/m",
			"main.go": `
package main

type S struct {
	Name string
}

func Do() *S {
	type Alias S
	s := Alias{Name: "foo"}
	return &s
}
`,
		},
		EntryPoint: "example.com/m.Do",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
		pointer, ok := r.ReturnValue.(*object.Pointer)
		if !ok {
			t.Fatalf("expected return value to be a pointer, but got %T", r.ReturnValue)
		}

		instance, ok := pointer.Value.(*object.Instance)
		if !ok {
			t.Fatalf("expected pointer to point to an instance, but got %T", pointer.Value)
		}

		if want := "example.com/m.S"; instance.TypeName != want {
			t.Errorf("expected instance type name to be %q, but got %q", want, instance.TypeName)
		}
	})
}

func TestEval_LocalTypeDefinitionInMethod(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/m",
			"main.go": `
package main
import "encoding/json"

type T struct {
	Name string
}

func (t *T) UnmarshalJSON(data []byte) error {
	type Alias T
	var aux Alias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*t = T(aux)
	return nil
}

// TestDo is a helper to provide a simple entry point.
func TestDo() {
	t := &T{}
	t.UnmarshalJSON([]byte("{\"Name\": \"Test\"}"))
}
`,
		},
		EntryPoint: "example.com/m.TestDo",
	}

	symgotest.Run(t, tc, func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("expected no error, but got: %+v", r.Error)
		}
		t.Log("Test ran without unexpected errors.")
	})
}
