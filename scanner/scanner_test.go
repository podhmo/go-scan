package scanner

import (
	"path/filepath"
	"testing"
)

// MockResolver is a mock implementation of PackageResolver for tests.
type MockResolver struct {
	ScanPackageByImportFunc func(importPath string) (*PackageInfo, error)
}

func (m *MockResolver) ScanPackageByImport(importPath string) (*PackageInfo, error) {
	if m.ScanPackageByImportFunc != nil {
		return m.ScanPackageByImportFunc(importPath)
	}
	return nil, nil
}

func TestScanPackageFeatures(t *testing.T) {
	s := New()
	pkgInfo, err := s.ScanPackage(filepath.Join("..", "testdata", "features"), &MockResolver{})
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}

	types := make(map[string]*TypeInfo)
	for _, ti := range pkgInfo.Types {
		types[ti.Name] = ti
	}

	// Test 1: Type with doc comment
	itemStruct, ok := types["Item"]
	if !ok {
		t.Fatal("Type 'Item' not found")
	}
	expectedDoc := "Item represents a product with an ID and Name."
	if itemStruct.Doc != expectedDoc {
		t.Errorf("Expected Item doc %q, got %q", expectedDoc, itemStruct.Doc)
	}

	// Test 2: Field with doc comment and line comment
	if len(itemStruct.Struct.Fields) < 2 {
		t.Fatal("Expected at least 2 fields in Item")
	}
	idField := itemStruct.Struct.Fields[0]
	if idField.Name != "ID" || idField.Doc != "The unique identifier for the item." {
		t.Errorf("ID field doc mismatch. Got: %q", idField.Doc)
	}

	// Test 3: Type alias with underlying type
	userIDAlias, ok := types["UserID"]
	if !ok {
		t.Fatal("Type 'UserID' not found")
	}
	if userIDAlias.Kind != AliasKind {
		t.Errorf("Expected UserID kind to be AliasKind, got %v", userIDAlias.Kind)
	}
	if userIDAlias.Underlying == nil || userIDAlias.Underlying.Name != "int64" {
		t.Errorf("Expected UserID underlying type to be 'int64', got %v", userIDAlias.Underlying)
	}

	// Test 4: Function type
	handlerFunc, ok := types["HandlerFunc"]
	if !ok {
		t.Fatal("Type 'HandlerFunc' not found")
	}
	if handlerFunc.Kind != FuncKind {
		t.Errorf("Expected HandlerFunc kind to be FuncKind, got %v", handlerFunc.Kind)
	}
}

func TestFieldType_Resolve(t *testing.T) {
	// Setup a mock resolver that returns a predefined package info
	resolver := &MockResolver{
		ScanPackageByImportFunc: func(importPath string) (*PackageInfo, error) {
			if importPath == "example.com/models" {
				return &PackageInfo{
					Types: []*TypeInfo{
						{Name: "User", Kind: StructKind},
					},
				}, nil
			}
			return nil, nil
		},
	}

	ft := &FieldType{
		Name:           "models.User",
		PkgName:        "models",
		resolver:       resolver,
		fullImportPath: "example.com/models",
		typeName:       "User",
	}

	// First call to Resolve should trigger the resolver
	def, err := ft.Resolve()
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}
	if def.Name != "User" {
		t.Errorf("Expected resolved type to be 'User', got %q", def.Name)
	}
	if ft.Definition == nil {
		t.Fatal("Definition should be cached after first call")
	}

	// Second call should use the cache (we can't easily test this, but we can nil out the func)
	resolver.ScanPackageByImportFunc = nil
	def2, err := ft.Resolve()
	if err != nil {
		t.Fatalf("Second Resolve() call failed: %v", err)
	}
	if def2.Name != "User" {
		t.Errorf("Expected cached resolved type to be 'User', got %q", def2.Name)
	}
}