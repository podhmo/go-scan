package evaluator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestPolicy_TypeAssert_WithUnresolvedType(t *testing.T) {
	code := `
package main
import "example.com/me/foreign/lib"

var ok bool
var val lib.ForeignType

func MyFunction(v any) {
	// This type assertion uses a type from an out-of-policy package.
	// The bug is that the evaluator calls Resolve() without a policy check,
	// which then tries to ScanPackageByImport on the foreign package.
	val, ok = v.(lib.ForeignType)
	Sentinel()
}

func Sentinel() {}
`
	files := map[string]string{
		"go.mod":             "module example.com/me",
		"main.go":            code,
		"foreign/lib/lib.go": "package lib; type ForeignType struct{}",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	var sentinelReached bool
	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		policy := func(path string) bool {
			return !strings.HasPrefix(path, "example.com/me/foreign/")
		}

		evaluator := New(s, nil, nil, policy)
		evaluator.RegisterIntrinsic("example.com/me.Sentinel", func(args ...object.Object) object.Object {
			sentinelReached = true
			return nil
		})

		mainPkg := findPackage(t, pkgs, "example.com/me")
		env := object.NewEnvironment()
		for _, file := range mainPkg.AstFiles {
			if res := evaluator.Eval(ctx, file, env, mainPkg); res != nil && res.Type() == object.ERROR_OBJ {
				return fmt.Errorf("initial Eval failed: %s", res.Inspect())
			}
		}

		fn := findFunc(t, mainPkg, "MyFunction")
		fn.Env = env

		// The input argument doesn't matter much, as the type assertion is symbolic.
		arg := &object.SymbolicPlaceholder{Reason: "test argument"}
		result := evaluator.Apply(ctx, fn, []object.Object{arg}, mainPkg)
		if err, ok := result.(*object.Error); ok {
			return fmt.Errorf("Apply() returned an error: %s", err.Inspect())
		}

		if !sentinelReached {
			return fmt.Errorf("sentinel was not reached")
		}

		// Check that the 'val' variable holds a placeholder with an unresolved type.
		valVar, ok := env.Get("val")
		if !ok {
			return fmt.Errorf("variable 'val' not found")
		}
		v, ok := valVar.(*object.Variable)
		if !ok {
			return fmt.Errorf("object 'val' is not a variable, but %T", v)
		}
		placeholder, ok := v.Value.(*object.SymbolicPlaceholder)
		if !ok {
			return fmt.Errorf("expected value of 'val' to be a SymbolicPlaceholder, got %T", v.Value)
		}

		wantUnresolvedType := scanner.NewUnresolvedTypeInfo("example.com/me/foreign/lib", "ForeignType")
		if diff := cmp.Diff(wantUnresolvedType, placeholder.TypeInfo()); diff != "" {
			t.Errorf("result placeholder TypeInfo mismatch (-want +got):\n%s", diff)
		}

		return nil
	}

	// This test will fail if evalAssignStmt calls Resolve() without a policy check.
	if _, err := scantest.Run(t, context.Background(), dir, []string{"./..."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
