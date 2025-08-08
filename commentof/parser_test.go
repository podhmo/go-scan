package commentof

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFromFile_Functions(t *testing.T) {
	path := filepath.Join("testdata", "functions.go")
	docs, err := FromFile(path)
	if err != nil {
		t.Fatalf("FromFile failed: %v", err)
	}

	if len(docs) == 0 {
		t.Fatal("Expected documentation, but got none.")
	}

	// Test F2
	f2 := findFunc(docs, "F2")
	if f2 == nil {
		t.Fatal("Function 'F2' not found in parsed docs")
	}

	expectedFuncDoc := "F2 is function @FUN2"
	if f2.Doc != expectedFuncDoc {
		t.Errorf("F2: expected doc '%s', got '%s'", expectedFuncDoc, f2.Doc)
	}

	// Test F2 Params
	if len(f2.Params) != 3 {
		t.Fatalf("F2: expected 3 params, got %d", len(f2.Params))
	}
	if f2.Params[0].Names[0] != "x" || !strings.Contains(f2.Params[0].Doc, "x is int @arg1") {
		t.Errorf("F2: unexpected doc for param 'x': %s", f2.Params[0].Doc)
	}
	if f2.Params[1].Names[0] != "y" || !strings.Contains(f2.Params[1].Doc, "y is int @arg2") {
		t.Errorf("F2: unexpected doc for param 'y': %s", f2.Params[1].Doc)
	}
	if f2.Params[2].Names[0] != "args" || !strings.Contains(f2.Params[2].Doc, "args is int @arg3") {
		t.Errorf("F2: unexpected doc for param 'args': %s", f2.Params[2].Doc)
	}

	// Test F2 Results
	if len(f2.Results) != 2 {
		t.Fatalf("F2: expected 2 results, got %d", len(f2.Results))
	}
	if !strings.Contains(f2.Results[0].Doc, "result of F2 @ret1") {
		t.Errorf("F2: unexpected doc for first result: %s", f2.Results[0].Doc)
	}
	if !strings.Contains(f2.Results[1].Doc, "error of F2 @ret2") {
		t.Errorf("F2: unexpected doc for second result: %s", f2.Results[1].Doc)
	}

	// Test F4
	f4 := findFunc(docs, "F4")
	if f4 == nil {
		t.Fatal("Function 'F4' not found in parsed docs")
	}
	if !strings.Contains(f4.Params[0].Doc, "x of F4 @arg4") || !strings.Contains(f4.Params[0].Doc, "x of F4 @arg5") {
		t.Errorf("F4: missing comments for param 'x': %s", f4.Params[0].Doc)
	}
	if !strings.Contains(f4.Params[1].Doc, "y of F4 @arg6") || !strings.Contains(f4.Params[1].Doc, "y of F4 @arg7") {
		t.Errorf("F4: missing comments for param 'y': %s", f4.Params[1].Doc)
	}
	if !strings.Contains(f4.Params[2].Doc, "arg of F4 @arg8") {
		t.Errorf("F4: missing comments for param 'args': %s", f4.Params[2].Doc)
	}

	// Test F8
	f8 := findFunc(docs, "F8")
	if f8 == nil {
		t.Fatal("Function 'F8' not found in parsed docs")
	}
	if len(f8.Params) != 3 {
		t.Fatalf("F8: expected 3 params, got %d", len(f8.Params))
	}
	if f8.Params[1].Names[0] != "x" || f8.Params[1].Names[1] != "y" {
		t.Errorf("F8: expected grouped params 'x, y', got %v", f8.Params[1].Names)
	}
	if !strings.Contains(f8.Params[2].Doc, "pretty output or not") {
		t.Errorf("F8: missing comment for 'pretty': %s", f8.Params[2].Doc)
	}
	if !strings.Contains(f8.Results[0].Doc, "ret") {
		t.Errorf("F8: missing comment for result: %s", f8.Results[0].Doc)
	}
}

func TestFromFile_Structs(t *testing.T) {
	path := filepath.Join("testdata", "structs.go")
	docs, err := FromFile(path)
	if err != nil {
		t.Fatalf("FromFile failed: %v", err)
	}

	// Test Struct S
	s := findType(docs, "S")
	if s == nil {
		t.Fatal("Type 'S' not found in parsed docs")
	}
	if !strings.Contains(s.Doc, "S is struct @S0") {
		t.Errorf("S: unexpected doc: %s", s.Doc)
	}

	sDef, ok := s.Definition.(*Struct)
	if !ok {
		t.Fatal("S: definition is not a struct")
	}

	// Test fields of S
	f0 := findField(sDef.Fields, "ExportedString")
	if f0 == nil || !strings.Contains(f0.Doc, "ExportedString is exported string @F0") {
		t.Errorf("S.ExportedString: doc mismatch: %v", f0)
	}

	f1 := findField(sDef.Fields, "ExportedString2")
	if f1 == nil || !strings.Contains(f1.Doc, "ExportedString2 is exported string @F1") {
		t.Errorf("S.ExportedString2: doc mismatch: %v", f1)
	}

	f2 := findField(sDef.Fields, "ExportedString3")
	if f2 == nil || !strings.Contains(f2.Doc, "ExportedString3 is exported string @F2") || !strings.Contains(f2.Doc, "ExportedString3 is exported string @F3") {
		t.Errorf("S.ExportedString3: doc mismatch: %v", f2)
	}

	// Test nested struct
	nested := findField(sDef.Fields, "Nested")
	if nested == nil || !strings.Contains(nested.Doc, "Nested is struct @SS0") {
		t.Errorf("S.Nested: doc mismatch: %v", nested)
	}
}

// Helper to find a function by name from the parsed results.
func findFunc(docs []interface{}, name string) *Function {
	for _, doc := range docs {
		if f, ok := doc.(*Function); ok && f.Name == name {
			return f
		}
	}
	return nil
}

// Helper to find a type by name from the parsed results.
func findType(docs []interface{}, name string) *TypeSpec {
	for _, doc := range docs {
		if t, ok := doc.(*TypeSpec); ok && t.Name == name {
			return t
		}
	}
	return nil
}

// Helper to find a field by name from a list of fields.
func findField(fields []*Field, name string) *Field {
	for _, f := range fields {
		for _, n := range f.Names {
			if n == name {
				return f
			}
		}
	}
	return nil
}