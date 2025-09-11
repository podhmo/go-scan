package scanner_test

import (
	"context"
	"strings"
	"testing"
	"time"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
	"github.com/podhmo/go-scan/symgo"
	"github.com/podhmo/go-scan/symgo/object"
)

func TestFieldType_String_RuntimeInfiniteRecursion(t *testing.T) {
	t.Log("This test verifies the RUNTIME bug where FieldType.String() causes a stack overflow on cyclic types.")
	ft := &scanner.FieldType{
		Name:    "T",
		IsSlice: true,
	}
	ft.Elem = ft // Create the cycle: T is a slice of itself.

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = ft.String()
		close(done)
	}()

	select {
	case <-done:
		t.Errorf("FieldType.String() completed, but was expected to hang due to infinite recursion")
	case <-ctx.Done():
		t.Log("FieldType.String() call timed out as expected, successfully reproducing the runtime bug.")
	}
}

func TestSymgo_EvalStringOnRecursiveType_WithFix(t *testing.T) {
	t.Log("This test verifies the SYMBOLIC evaluation fix, ensuring symgo's recursion detection now works for methods on cyclic types.")

	const testGoMod = `
module example.com/m

go 1.21

replace github.com/podhmo/go-scan => ../../../
`
	const testMainGo = `
package main
type T []*T
`

	files := map[string]string{
		"go.mod":  testGoMod,
		"main.go": testMainGo,
	}
	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	action := func(ctx context.Context, s *goscan.Scanner, pkgs []*goscan.Package) error {
		interp, err := symgo.NewInterpreter(s)
		if err != nil {
			return err
		}

		mainPkg := pkgs[0]
		var typeInfo *scanner.TypeInfo
		for _, ti := range mainPkg.Types {
			if ti.Name == "T" {
				typeInfo = ti
				break
			}
		}
		if typeInfo == nil {
			t.Fatal(`could not find type info for "T"`)
		}
		cyclicFieldType := typeInfo.Underlying

		scannerPkg, err := s.ScanPackageByImport(ctx, "github.com/podhmo/go-scan/scanner")
		if err != nil {
			return err
		}

		var stringMethod *scanner.FunctionInfo
		for _, f := range scannerPkg.Functions {
			if f.Name == "String" && f.Receiver != nil && f.Receiver.Type.Name == "FieldType" {
				stringMethod = f
				break
			}
		}
		if stringMethod == nil {
			t.Fatal("could not find FunctionInfo for scanner.FieldType.String")
		}

		evaluator := interp.EvaluatorForTest()
		pkgObj, err := evaluator.GetOrLoadPackageForTest(ctx, "github.com/podhmo/go-scan/scanner")
		if err != nil {
			return err
		}
		baseFnObj := evaluator.GetOrResolveFunctionForTest(pkgObj, stringMethod)
		baseFn, ok := baseFnObj.(*object.Function)
		if !ok {
			t.Fatalf("resolved method is not a function object")
		}

		receiverPlaceholder := &object.SymbolicPlaceholder{Reason: "cyclic type for test"}
		receiverPlaceholder.SetFieldType(cyclicFieldType)
		boundFn := baseFn.WithReceiver(receiverPlaceholder, 0)

		result, applyErr := interp.Apply(ctx, boundFn, nil, scannerPkg)
		if applyErr != nil {
			t.Fatalf("interp.Apply failed: %v", applyErr)
		}

		retVal, ok := result.(*object.ReturnValue)
		if !ok {
			t.Fatalf("Apply() did not return a ReturnValue, got %T", result)
		}
		placeholder, ok := retVal.Value.(*object.SymbolicPlaceholder)
		if !ok {
			t.Fatalf("ReturnValue does not contain a SymbolicPlaceholder, got %T", retVal.Value)
		}
		if !strings.Contains(placeholder.Reason, "bounded recursion halt") {
			t.Errorf("Expected placeholder reason to be 'bounded recursion halt', but got %q", placeholder.Reason)
		}
		return nil
	}

	if _, err := scantest.Run(t, dir, []string{"."}, action); err != nil {
		t.Fatalf("scantest.Run failed: %+v", err)
	}
}

func TestNewUnresolvedTypeInfo(t *testing.T) {
	pkgPath := "example.com/foo"
	name := "Bar"

	ti := scanner.NewUnresolvedTypeInfo(pkgPath, name)

	if ti == nil {
		t.Fatal("NewUnresolvedTypeInfo returned nil")
	}
	if ti.PkgPath != pkgPath {
		t.Errorf("got PkgPath %q, want %q", ti.PkgPath, pkgPath)
	}
	if ti.Name != name {
		t.Errorf("got Name %q, want %q", ti.Name, name)
	}
	if !ti.Unresolved {
		t.Error("got Unresolved = false, want true")
	}
}

func TestTypeInfo_Annotation(t *testing.T) {
	tests := []struct {
		name      string
		doc       string
		annoName  string
		wantValue string
		wantOk    bool
	}{
		{
			name:      "basic case with colon",
			doc:       "// @foo: bar",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "basic case with space",
			doc:       "// @foo bar",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "no value",
			doc:       "// @foo",
			annoName:  "foo",
			wantValue: "",
			wantOk:    true,
		},
		{
			name:      "leading and trailing spaces on line",
			doc:       "   // @foo: bar   ",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "space around separator",
			doc:       "// @foo : bar",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "value with spaces",
			doc:       "// @foo: bar baz qux",
			annoName:  "foo",
			wantValue: "bar baz qux",
			wantOk:    true,
		},
		{
			name:      "multi-line doc comment",
			doc:       "// This is a struct.\n// @foo: bar\n// More comments.",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "annotation not present",
			doc:       "// This is a struct.",
			annoName:  "foo",
			wantValue: "",
			wantOk:    false,
		},
		{
			name:      "multiple annotations, find first",
			doc:       "// @foo: bar\n// @bar: baz",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "complex name with value",
			doc:       `// @deriving:binding in:"body" required`,
			annoName:  "deriving:binding",
			wantValue: `in:"body" required`,
			wantOk:    true,
		},
		{
			name:      "complex name with colon separator",
			doc:       `// @deriving:binding: in:"body" required`,
			annoName:  "deriving:binding",
			wantValue: `in:"body" required`,
			wantOk:    true,
		},
		{
			name:      "empty doc",
			doc:       "",
			annoName:  "foo",
			wantValue: "",
			wantOk:    false,
		},
		{
			name:      "annotation is the whole line",
			doc:       "@foo:bar",
			annoName:  "foo",
			wantValue: "bar",
			wantOk:    true,
		},
		{
			name:      "annotation with only spaces after colon",
			doc:       "@foo:   ",
			annoName:  "foo",
			wantValue: "",
			wantOk:    true,
		},
		{
			name:      "annotation name is a prefix of another",
			doc:       "@foobar: baz",
			annoName:  "foo",
			wantValue: "",
			wantOk:    false,
		},
		{
			name:      "annotation name followed by non-separator",
			doc:       "@foobar",
			annoName:  "foo",
			wantValue: "",
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := &scanner.TypeInfo{Doc: tt.doc}
			gotValue, gotOk := ti.Annotation(context.Background(), tt.annoName)
			if gotOk != tt.wantOk {
				t.Errorf("TypeInfo.Annotation() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
			if gotValue != tt.wantValue {
				t.Errorf("TypeInfo.Annotation() gotValue = %q, want %q", gotValue, tt.wantValue)
			}
		})
	}
}
