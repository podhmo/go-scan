package evaluator

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestAccessor(t *testing.T) {
	source := `
package main
type S struct {
	Name string
}
func (s *S) GetName() string {
	return s.Name
}
`
	files := map[string]string{
		"go.mod":  "module my-test",
		"main.go": source,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		if len(pkgs) != 1 {
			t.Fatalf("expected 1 package, but got %d", len(pkgs))
		}
		pkg := pkgs[0]

		var sTypeInfo *scanner.TypeInfo
		for _, ti := range pkg.Types {
			if ti.Name == "S" {
				sTypeInfo = ti
				break
			}
		}
		if sTypeInfo == nil {
			t.Fatal("struct S not found")
		}

		eval := New(s, nil, nil, func(pkgpath string) bool { return true })

		t.Run("findFieldOnType", func(t *testing.T) {
			field, err := eval.accessor.findFieldOnType(ctx, sTypeInfo, "Name")
			if err != nil {
				t.Fatalf("findFieldOnType failed: %+v", err)
			}
			if field == nil {
				t.Fatal("field 'Name' not found")
			}
			if diff := cmp.Diff("Name", field.Name); diff != "" {
				t.Errorf("field name mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("findMethodOnType", func(t *testing.T) {
			receiver := &object.SymbolicPlaceholder{Reason: "test receiver"}
			method, err := eval.accessor.findMethodOnType(ctx, sTypeInfo, "GetName", eval.UniverseEnv, receiver, 0)
			if err != nil {
				t.Fatalf("findMethodOnType failed: %+v", err)
			}
			if method == nil {
				t.Fatal("method 'GetName' not found")
			}
			if diff := cmp.Diff("GetName", method.Name.Name); diff != "" {
				t.Errorf("method name mismatch (-want +got):\n%s", diff)
			}
		})
		return nil
	}

	_, err := scantest.Run(t, t.Context(), dir, []string{"."}, action)
	if err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}
