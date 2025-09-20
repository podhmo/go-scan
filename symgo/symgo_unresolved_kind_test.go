package symgo_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
	"github.com/podhmo/go-scan/symgotest"
)

func TestSymgo_UnresolvedKindInference(t *testing.T) {
	tc := symgotest.TestCase{
		Source: map[string]string{
			"go.mod": "module example.com/unresolvedkind",
			"ifaceandstruct/defs.go": `
package ifaceandstruct
type MyStruct struct { Name string }
type MyInterface interface { DoSomething() }
`,
			"main/main.go": `
package main
import "example.com/unresolvedkind/ifaceandstruct"
var VStruct ifaceandstruct.MyStruct
var VInterface ifaceandstruct.MyInterface
func main() {
	s := ifaceandstruct.MyStruct{Name: "test"}
	VStruct = s
	var x any
	i := x.(ifaceandstruct.MyInterface)
	VInterface = i
}
`,
		},
		EntryPoint: "example.com/unresolvedkind/main.main",
		Options: []symgotest.Option{
			symgotest.WithScanPolicy(func(pkgPath string) bool {
				// We want to test inference, so we treat the package
				// with the type definitions as "out of policy".
				return pkgPath != "example.com/unresolvedkind/ifaceandstruct"
			}),
		},
	}

	action := func(t *testing.T, r *symgotest.Result) {
		if r.Error != nil {
			t.Fatalf("Execution failed: %+v", r.Error)
		}

		ctx := context.Background() // Or pass a context from the test if needed.

		// Assertions
		vStructObj, ok := r.Interpreter.FindObjectInPackage(ctx, "example.com/unresolvedkind/main", "VStruct")
		if !ok {
			t.Fatal("global variable VStruct not found")
		}
		vStruct, ok := vStructObj.(*object.Variable)
		if !ok {
			t.Fatalf("VStruct is not a variable, but %T", vStructObj)
		}
		// We check the TypeInfo of the VALUE assigned to the variable,
		// as this is what gets the inferred kind.
		structTypeInfo := vStruct.Value.TypeInfo()
		if structTypeInfo == nil {
			t.Fatal("value of VStruct has no TypeInfo")
		}
		if !structTypeInfo.Unresolved {
			t.Error("MyStruct's TypeInfo should be Unresolved, but it was not")
		}
		if diff := cmp.Diff(scanner.StructKind, structTypeInfo.Kind); diff != "" {
			t.Errorf("VStruct kind mismatch (-want +got):\n%s", diff)
		}

		vIfaceObj, ok := r.Interpreter.FindObjectInPackage(ctx, "example.com/unresolvedkind/main", "VInterface")
		if !ok {
			t.Fatal("global variable VInterface not found")
		}
		vIface, ok := vIfaceObj.(*object.Variable)
		if !ok {
			t.Fatalf("VInterface is not a variable, but %T", vIfaceObj)
		}
		// We check the TypeInfo of the VALUE assigned to the variable.
		ifaceTypeInfo := vIface.Value.TypeInfo()
		if ifaceTypeInfo == nil {
			t.Fatal("value of VInterface has no TypeInfo")
		}
		if !ifaceTypeInfo.Unresolved {
			t.Error("MyInterface's TypeInfo should be Unresolved, but it was not")
		}
		if diff := cmp.Diff(scanner.InterfaceKind, ifaceTypeInfo.Kind); diff != "" {
			t.Errorf("VInterface kind mismatch (-want +got):\n%s", diff)
		}
	}

	symgotest.Run(t, tc, action)
}
