package scanner

import (
	"context"
	"fmt"
	"go/token"
	"path/filepath"
	"testing"
)

// MockResolver is a mock implementation of PackageResolver for tests.
type MockResolver struct {
	ScanPackageByImportFunc func(ctx context.Context, importPath string) (*PackageInfo, error)
}

func (m *MockResolver) ScanPackageByImport(ctx context.Context, importPath string) (*PackageInfo, error) {
	if m.ScanPackageByImportFunc != nil {
		return m.ScanPackageByImportFunc(ctx, importPath)
	}
	return nil, nil // Default mock behavior
}

func TestNewScanner(t *testing.T) {
	modulePath := "example.com/test"
	rootDir := "/tmp/test"

	t.Run("nil_fset", func(t *testing.T) {
		_, err := New(nil, nil, nil, modulePath, rootDir)
		if err == nil {
			t.Error("Expected error when creating scanner with nil fset, got nil")
		}
	})

	t.Run("valid_fset", func(t *testing.T) {
		fset := token.NewFileSet()
		s, err := New(fset, nil, nil, modulePath, rootDir)
		if err != nil {
			t.Errorf("Expected no error when creating scanner with valid fset, got %v", err)
		}
		if s == nil {
			t.Error("Scanner should not be nil with valid fset")
		} else if s.fset != fset {
			t.Error("Scanner fset not set correctly")
		}
	})
}

func TestScanPackageFeatures(t *testing.T) {
	fset := token.NewFileSet()
	testDir := filepath.Join("..", "testdata", "features")
	absTestDir, _ := filepath.Abs(testDir)
	s, err := New(fset, nil, nil, "example.com/test/features", absTestDir)
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}

	// Scan only features.go and another.go, which belong to the same package "features"
	filesToScan := []string{
		filepath.Join(testDir, "features.go"),
		filepath.Join(testDir, "another.go"),
		filepath.Join(testDir, "variadic.go"),
	}

	pkgInfo, err := s.ScanFiles(context.Background(), filesToScan, testDir, &MockResolver{})
	if err != nil {
		t.Fatalf("ScanFiles failed for %v: %v", filesToScan, err)
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

	// Test 5: Variadic function
	funcs := make(map[string]*FunctionInfo)
	for _, fi := range pkgInfo.Functions {
		funcs[fi.Name] = fi
	}

	variadicFunc, ok := funcs["VariadicFunc"]
	if !ok {
		t.Fatal("Function 'VariadicFunc' not found")
	}
	if !variadicFunc.IsVariadic {
		t.Error("Expected VariadicFunc to be variadic")
	}
	if len(variadicFunc.Parameters) != 2 {
		t.Errorf("Expected 2 parameters for VariadicFunc, got %d", len(variadicFunc.Parameters))
	}

	nonVariadicFunc, ok := funcs["NonVariadicFunc"]
	if !ok {
		t.Fatal("Function 'NonVariadicFunc' not found")
	}
	if nonVariadicFunc.IsVariadic {
		t.Error("Expected NonVariadicFunc to not be variadic")
	}
}

func TestScanFiles(t *testing.T) {
	fset := token.NewFileSet()
	testdataDir := filepath.Join("..", "testdata", "features")
	absTestdataDir, _ := filepath.Abs(testdataDir)
	s, err := New(fset, nil, nil, "example.com/test/features", absTestdataDir)
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}
	mockResolver := &MockResolver{}

	t.Run("scan_single_file", func(t *testing.T) {
		filePath := filepath.Join(testdataDir, "features.go")
		pkgInfo, err := s.ScanFiles(context.Background(), []string{filePath}, testdataDir, mockResolver)
		if err != nil {
			t.Fatalf("ScanFiles single file failed: %v", err)
		}
		if pkgInfo.Name != "features" {
			t.Errorf("Expected package name 'features', got '%s'", pkgInfo.Name)
		}
		if len(pkgInfo.Files) != 1 || filepath.Base(pkgInfo.Files[0]) != "features.go" {
			t.Errorf("Expected 1 file 'features.go', got %v", pkgInfo.Files)
		}
		if len(pkgInfo.Types) == 0 { // Check based on content of features.go
			t.Error("Expected types to be parsed from features.go")
		}
	})

	t.Run("scan_multiple_files_same_package", func(t *testing.T) {
		filePaths := []string{
			filepath.Join(testdataDir, "features.go"),
			filepath.Join(testdataDir, "another.go"),
		}
		pkgInfo, err := s.ScanFiles(context.Background(), filePaths, testdataDir, mockResolver)
		if err != nil {
			t.Fatalf("ScanFiles multiple files failed: %v", err)
		}
		if pkgInfo.Name != "features" {
			t.Errorf("Expected package name 'features', got '%s'", pkgInfo.Name)
		}
		if len(pkgInfo.Files) != 2 {
			t.Errorf("Expected 2 files, got %d", len(pkgInfo.Files))
		}
		// Check if types from both files are present
		typeNames := make(map[string]bool)
		for _, ti := range pkgInfo.Types {
			typeNames[ti.Name] = true
		}
		if !typeNames["Item"] || !typeNames["Point"] { // Item from features.go, Point from another.go
			t.Errorf("Expected types from both files to be present. Found: %v", typeNames)
		}
	})

	t.Run("scan_files_different_packages", func(t *testing.T) {
		filePaths := []string{
			filepath.Join(testdataDir, "features.go"),     // package features
			filepath.Join(testdataDir, "differentpkg.go"), // package otherfeatures
		}
		_, err := s.ScanFiles(context.Background(), filePaths, testdataDir, mockResolver)
		if err == nil {
			t.Error("Expected error when scanning files from different packages, got nil")
		}
	})

	t.Run("scan_empty_file_list", func(t *testing.T) {
		_, err := s.ScanFiles(context.Background(), []string{}, testdataDir, mockResolver)
		if err == nil {
			t.Error("Expected error when scanning an empty file list, got nil")
		}
	})

	t.Run("scan_non_existent_file", func(t *testing.T) {
		filePaths := []string{filepath.Join(testdataDir, "nonexistent.go")}
		_, err := s.ScanFiles(context.Background(), filePaths, testdataDir, mockResolver)
		if err == nil {
			t.Error("Expected error when scanning non-existent file, got nil")
		}
	})
}

func TestFieldType_Resolve(t *testing.T) {
	// Setup a mock resolver that returns a predefined package info
	resolver := &MockResolver{
		ScanPackageByImportFunc: func(ctx context.Context, importPath string) (*PackageInfo, error) {
			if importPath == "example.com/models" {
				return &PackageInfo{
					Fset: token.NewFileSet(),
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
	def, err := ft.Resolve(context.Background(), make(map[string]bool))
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
	resolver.ScanPackageByImportFunc = nil // To ensure resolver is not called again
	def2, err := ft.Resolve(context.Background(), make(map[string]bool))
	if err != nil {
		t.Fatalf("Second Resolve() call failed: %v", err)
	}
	if def2.Name != "User" {
		t.Errorf("Expected cached resolved type to be 'User', got %q", def2.Name)
	}
}

func TestScanWithOverlay(t *testing.T) {
	fset := token.NewFileSet()
	testDir := filepath.Join("..", "testdata", "basic")
	absTestDir, _ := filepath.Abs(testDir)
	modulePath := "example.com/basic"

	overlayContent := fmt.Sprintf("package basic\n\n// In-memory version of a struct\ntype User struct {\n\tID   int    `json:\"id\"`\n\tName string `json:\"name\"`\n}\n")
	overlay := Overlay{
		"basic.go": []byte(overlayContent),
	}

	// Here, we provide an absolute path for moduleRootDir
	s, err := New(fset, nil, overlay, modulePath, absTestDir)
	if err != nil {
		t.Fatalf("scanner.New with overlay failed: %v", err)
	}

	// ScanFiles expects absolute paths, so we construct one.
	scanFilePath := filepath.Join(absTestDir, "basic.go")

	// The pkgDirPath should also be an absolute path to the package directory.
	pkgInfo, err := s.ScanFiles(context.Background(), []string{scanFilePath}, absTestDir, &MockResolver{})
	if err != nil {
		t.Fatalf("ScanFiles with overlay failed: %v", err)
	}

	userType := pkgInfo.Lookup("User")
	if userType == nil {
		t.Fatal("Type 'User' from overlay not found")
	}

	if len(userType.Struct.Fields) != 2 {
		t.Fatalf("Expected 2 fields in User struct from overlay, got %d", len(userType.Struct.Fields))
	}
	if userType.Struct.Fields[0].Name != "ID" || userType.Struct.Fields[1].Name != "Name" {
		t.Error("Field names in User struct from overlay are incorrect")
	}
}
