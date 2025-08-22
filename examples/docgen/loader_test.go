package main

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo/object"
	"github.com/podhmo/go-scan/scanner"
)

func TestPatternKeyFromFunc(t *testing.T) {
	t.Run("GoSourceFunction", func(t *testing.T) {
		fn := &object.GoSourceFunction{
			PkgPath: "github.com/user/project/api",
			Info: &scanner.FunctionInfo{
				Name: "ListUsers",
			},
		}

		want := "github.com/user/project/api.ListUsers"
		got, err := patternKeyFromFunc(fn)
		if err != nil {
			t.Fatalf("patternKeyFromFunc() returned error: %+v", err)
		}

		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("patternKeyFromFunc() mismatch (-want +got):\n%s", diff)
		}
	})

	// TODO: Add test for GoMethodValue once the underlying object model is enhanced
	// to provide the receiver's package path reliably.
}
