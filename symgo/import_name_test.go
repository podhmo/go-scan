package symgo_test

import (
	"context"
	"testing"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestImport_MismatchedPackageName(t *testing.T) {
	// This test verifies that `symgo` can correctly handle a package
	// whose declared name (`package <name>`) is different from the
	// last element of its import path.

	ctx := context.Background()
	files := map[string]string{
		"go.mod":        "module example.com\n",
		"pkgfoo/bar.go": "package pkgbar\n\nfunc Foo() {}", // import path is .../pkgfoo, but package name is pkgbar
		"main/main.go":  "package main\n\nimport \"example.com/pkgfoo\"\n\nfunc Entry() {\n\tpkgbar.Foo()\n}",
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	s, err := goscan.New(goscan.WithWorkDir(dir))
	if err != nil {
		t.Fatalf("failed to create scanner: %v", err)
	}

	interp, err := symgo.NewInterpreter(s)
	if err != nil {
		t.Fatalf("failed to create interpreter: %v", err)
	}

	mainPkg, err := s.ScanPackageByImport(ctx, "example.com/main")
	if err != nil {
		t.Fatalf("scan main failed: %v", err)
	}

	for _, file := range mainPkg.AstFiles {
		if _, err := interp.Eval(ctx, file, mainPkg); err != nil {
			t.Fatalf("eval main failed: %v", err)
		}
	}

	entryObj, ok := interp.FindObject("Entry")
	if !ok {
		t.Fatal("could not find Entry function object in interpreter")
	}
	entryFn, ok := entryObj.(*object.Function)
	if !ok {
		t.Fatalf("Entry is not a function object, got %T", entryObj)
	}

	// With the fix in place, this should now succeed.
	_, err = interp.Apply(ctx, entryFn, nil, mainPkg)

	if err != nil {
		t.Fatalf("analysis was expected to succeed, but it failed: %v", err)
	}

	t.Log("Test succeeded as expected, verifying the fix.")
}
