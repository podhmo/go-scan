package symgo

import (
	"context"
	"fmt"
	"testing"

	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo/evaluator"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestInterfaceBinding(t *testing.T) {
	source := `
package main
import "io"

// TargetFunc is the function we will analyze.
func TargetFunc(writer io.Writer) {
	writer.Write([]byte("hello"))
}`

	files := map[string]string{
		"go.mod":  "module main",
		"main.go": source,
	}

	var intrinsicCalled bool

	action := func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		mainPkg := pkgs[0]
		eval := evaluator.New(s, nil, nil, func(path string) bool { return true })

		// Manually find the bytes.Buffer type from the scanner's cache
		// This is necessary because the test needs a type from a standard library package.
		bufferPkg, err := s.ScanPackageFromImportPath(ctx, "bytes")
		if err != nil {
			return fmt.Errorf("could not scan bytes package: %w", err)
		}
		var bytesBufferType *scan.TypeInfo
		for _, typ := range bufferPkg.Types {
			if typ.Name == "Buffer" {
				bytesBufferType = typ
				break
			}
		}
		if bytesBufferType == nil {
			return fmt.Errorf("could not find bytes.Buffer type")
		}

		eval.BindInterface("io.Writer", bytesBufferType, true) // true for pointer

		// Register intrinsic for the concrete type's method
		eval.RegisterIntrinsic("(*bytes.Buffer).Write", func(ctx context.Context, args ...object.Object) object.Object {
			intrinsicCalled = true
			return object.NIL
		})

		// Find the TargetFunc
		var targetFunc *object.Function
		for _, f := range mainPkg.Functions {
			if f.Name == "TargetFunc" {
				pkgObj := &object.Package{ScannedInfo: mainPkg, Env: object.NewEnvironment()}
				targetFunc = eval.GetOrResolveFunctionForTest(ctx, pkgObj, f).(*object.Function)
				break
			}
		}
		if targetFunc == nil {
			return fmt.Errorf("function 'TargetFunc' not found")
		}

		// Find the io.Writer type to create a symbolic argument
		ioPkg, err := s.ScanPackageFromImportPath(ctx, "io")
		if err != nil {
			return fmt.Errorf("could not scan io package: %w", err)
		}
		var writerType *scan.TypeInfo
		for _, typ := range ioPkg.Types {
			if typ.Name == "Writer" {
				writerType = typ
				break
			}
		}
		if writerType == nil {
			return fmt.Errorf("could not find io.Writer type")
		}

		symbolicWriter := &object.SymbolicPlaceholder{Reason: "symbolic writer"}
		symbolicWriter.SetTypeInfo(writerType)

		// Apply the function
		eval.Apply(ctx, targetFunc, []object.Object{symbolicWriter}, mainPkg, object.NewEnvironment())

		// Assert that the binding worked and the intrinsic was called
		if !intrinsicCalled {
			return fmt.Errorf("expected intrinsic for (*bytes.Buffer).Write to be called, but it was not")
		}
		return nil
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	if _, err := scantest.Run(t, context.Background(), dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run() failed: %+v", err)
	}
}
