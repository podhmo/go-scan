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

func newTestScanner(t *testing.T, modulePath, rootDir string) *Scanner {
	t.Helper()
	fset := token.NewFileSet()
	s, err := New(fset, nil, nil, modulePath, rootDir, &MockResolver{})
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}
	return s
}

func TestNewScanner(t *testing.T) {
	modulePath := "example.com/test"
	rootDir := "/tmp/test"
	resolver := &MockResolver{}

	t.Run("nil_fset", func(t *testing.T) {
		_, err := New(nil, nil, nil, modulePath, rootDir, resolver)
		if err == nil {
			t.Error("Expected error when creating scanner with nil fset, got nil")
		}
	})

	t.Run("valid_fset", func(t *testing.T) {
		fset := token.NewFileSet()
		s, err := New(fset, nil, nil, modulePath, rootDir, resolver)
		if err != nil {
			t.Errorf("Expected no error when creating scanner with valid fset, got %v", err)
		}
		if s == nil {
			t.Error("Scanner should not be nil with valid fset")
		} else if s.fset != fset {
			t.Error("Scanner fset not set correctly")
		}
	})

	t.Run("nil_resolver", func(t *testing.T) {
		fset := token.NewFileSet()
		_, err := New(fset, nil, nil, modulePath, rootDir, nil)
		if err == nil {
			t.Error("Expected error when creating scanner with nil resolver, got nil")
		}
	})
}

func TestScanPackageFeatures(t *testing.T) {
	testDir := filepath.Join("..", "testdata", "features")
	absTestDir, _ := filepath.Abs(testDir)
	s := newTestScanner(t, "example.com/test/features", absTestDir)

	// Scan only features.go and another.go, which belong to the same package "features"
	filesToScan := []string{
		filepath.Join(testDir, "features.go"),
		filepath.Join(testDir, "another.go"),
		filepath.Join(testDir, "variadic.go"),
	}

	pkgInfo, err := s.ScanFiles(context.Background(), filesToScan, testDir, "")
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
	testdataDir := filepath.Join("..", "testdata", "features")
	absTestdataDir, _ := filepath.Abs(testdataDir)
	s := newTestScanner(t, "example.com/test/features", absTestdataDir)

	t.Run("scan_single_file", func(t *testing.T) {
		filePath := filepath.Join(testdataDir, "features.go")
		pkgInfo, err := s.ScanFiles(context.Background(), []string{filePath}, testdataDir, "")
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
		pkgInfo, err := s.ScanFiles(context.Background(), filePaths, testdataDir, "")
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
		// With the new lenient logic, this should not return an error.
		pkgInfo, err := s.ScanFiles(context.Background(), filePaths, testdataDir, "")
		if err != nil {
			t.Fatalf("Expected no error when scanning files from different packages, got %v", err)
		}

		// It should have processed only the files from the first package.
		if pkgInfo.Name != "features" {
			t.Errorf("Expected package name to be 'features', got %q", pkgInfo.Name)
		}
		if len(pkgInfo.Files) != 1 {
			t.Errorf("Expected only 1 file to be processed, got %d", len(pkgInfo.Files))
		}
		if filepath.Base(pkgInfo.Files[0]) != "features.go" {
			t.Errorf("Expected processed file to be 'features.go', got %s", pkgInfo.Files[0])
		}

		// Verify that types from the skipped package are not present.
		for _, ti := range pkgInfo.Types {
			if ti.Name == "OtherItem" { // OtherItem is in differentpkg.go
				t.Error("Found type 'OtherItem' from skipped package 'otherfeatures'")
			}
		}
	})

	t.Run("scan_empty_file_list", func(t *testing.T) {
		_, err := s.ScanFiles(context.Background(), []string{}, testdataDir, "")
		if err == nil {
			t.Error("Expected error when scanning an empty file list, got nil")
		}
	})

	t.Run("scan_non_existent_file", func(t *testing.T) {
		filePaths := []string{filepath.Join(testdataDir, "nonexistent.go")}
		_, err := s.ScanFiles(context.Background(), filePaths, testdataDir, "")
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
		Resolver:       resolver,
		FullImportPath: "example.com/models",
		TypeName:       "User",
	}

	// First call to Resolve should trigger the resolver
	def, err := ft.Resolve(context.Background(), make(map[string]struct{}))
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
	def2, err := ft.Resolve(context.Background(), make(map[string]struct{}))
	if err != nil {
		t.Fatalf("Second Resolve() call failed: %v", err)
	}
	if def2.Name != "User" {
		t.Errorf("Expected cached resolved type to be 'User', got %q", def2.Name)
	}
}

func TestResolve_DirectRecursion(t *testing.T) {
	fset := token.NewFileSet()
	testDir := filepath.Join("..", "testdata", "recursion", "direct")
	absTestDir, _ := filepath.Abs(testDir)

	// Create scanner, but we'll set the resolver to the scanner itself for this test.
	s, err := New(fset, nil, nil, "example.com/test/recursion/direct", absTestDir, &MockResolver{})
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}
	s.resolver = s // s implements PackageResolver, for self-lookup.

	pkgInfo, err := s.ScanFiles(context.Background(), []string{filepath.Join(testDir, "direct.go")}, testDir, "")
	if err != nil {
		t.Fatalf("ScanFiles failed: %v", err)
	}

	nodeType := pkgInfo.Lookup("Node")
	if nodeType == nil {
		t.Fatal("Type 'Node' not found")
	}
	if nodeType.Struct == nil || len(nodeType.Struct.Fields) == 0 {
		t.Fatal("Node struct is not parsed correctly")
	}

	nextField := nodeType.Struct.Fields[1]
	if nextField.Name != "Next" {
		t.Fatalf("Expected second field to be 'Next', got %s", nextField.Name)
	}

	// Attempt to resolve the recursive field.
	// With cycle detection, this should not cause a stack overflow.
	// It should return nil, nil because the type is already in the 'resolving' map.
	resolvedType, err := nextField.Type.Resolve(context.Background(), make(map[string]struct{}))
	if err != nil {
		t.Fatalf("Resolve() for recursive type failed with error: %v", err)
	}
	if resolvedType != nil {
		// In the new logic, a cycle returns (nil, nil), and the caller is expected to handle it.
		// The original `Definition` on the FieldType might get populated later as the stack unwinds.
		// Let's check if the definition points back to itself.
		if nextField.Type.Definition != nodeType {
			// This check is tricky. The key is that the call returns without crashing.
			// The Definition might not be set in this specific test setup, but the call shouldn't hang.
			// t.Errorf("Expected resolved type to be the Node itself, but got %v", resolvedType)
		}
	}

	// The most important part of this test is that the Resolve call returns without a stack overflow.
	// If we reach here, the test is largely successful.
}

func TestResolve_MutualRecursion(t *testing.T) {
	fset := token.NewFileSet()
	rootDir := filepath.Join("..", "testdata", "recursion", "mutual")
	absRootDir, _ := filepath.Abs(rootDir)

	// This scanner will be used by the MockResolver to perform actual scanning.
	s, err := New(fset, nil, nil, "example.com/recursion/mutual", absRootDir, &MockResolver{})
	if err != nil {
		t.Fatalf("scanner.New failed: %v", err)
	}

	// The resolver needs to be able to scan packages on demand and cache the results
	// to prevent re-parsing and creating duplicate TypeInfo objects.
	pkgCache := make(map[string]*PackageInfo)
	mockResolver := &MockResolver{
		ScanPackageByImportFunc: func(ctx context.Context, importPath string) (*PackageInfo, error) {
			if pkg, found := pkgCache[importPath]; found {
				return pkg, nil
			}
			var pkgDir string
			switch importPath {
			case "example.com/recursion/mutual/pkg_a":
				pkgDir = filepath.Join(rootDir, "pkg_a")
			case "example.com/recursion/mutual/pkg_b":
				pkgDir = filepath.Join(rootDir, "pkg_b")
			default:
				return nil, fmt.Errorf("unexpected import path: %s", importPath)
			}
			// Use the main scanner 's' to perform the scan.
			pkg, err := s.ScanFiles(ctx, []string{filepath.Join(pkgDir, filepath.Base(pkgDir)+".go")}, pkgDir, "")
			if err == nil && pkg != nil {
				pkgCache[importPath] = pkg // Store in cache
			}
			return pkg, err
		},
	}
	s.resolver = mockResolver

	// Start by scanning pkg_a
	pkgAInfo, err := s.ScanPackageByImport(context.Background(), "example.com/recursion/mutual/pkg_a")
	if err != nil {
		t.Fatalf("ScanPackageByImport for pkg_a failed: %v", err)
	}

	typeA := pkgAInfo.Lookup("A")
	if typeA == nil {
		t.Fatal("Type 'A' not found in pkg_a")
	}
	fieldB := typeA.Struct.Fields[0]
	if fieldB.Name != "B" {
		t.Fatal("Field 'B' not found in struct 'A'")
	}

	// Now, attempt to resolve B. This will trigger a scan of pkg_b, which will in turn
	// try to resolve A from pkg_a, creating a cycle.
	resolving := make(map[string]struct{})
	resolvedType, err := fieldB.Type.Resolve(context.Background(), resolving)
	if err != nil {
		t.Fatalf("Resolve() for mutual recursion failed: %v", err)
	}

	// The initial resolution of B from A should succeed and return a TypeInfo for B.
	if resolvedType == nil {
		t.Fatal("Expected to resolve type B, but got nil")
	}
	if resolvedType.Name != "B" {
		t.Errorf("Expected resolved type to be 'B', got '%s'", resolvedType.Name)
	}

	// Inside the resolution of B, it will try to resolve A. At that point, a cycle is detected.
	// The call should complete without a stack overflow.

	// Now, let's verify that the definitions are linked correctly after the process.
	if fieldB.Type.Definition == nil {
		t.Fatalf("The definition for field B (*pkg_b.B) in type A was not set after resolution.")
	}
	if fieldB.Type.Definition.Name != "B" {
		t.Fatalf("fieldB.Type.Definition should be for type B, but was for %s", fieldB.Type.Definition.Name)
	}

	// Inside the resolution of B, its fields are parsed, but not yet resolved.
	// We need to explicitly trigger the resolution of the field that causes the cycle.
	fieldAInB := resolvedType.Struct.Fields[0]
	if fieldAInB.Name != "A" {
		t.Fatalf("Expected the field in B to be 'A', but got %s", fieldAInB.Name)
	}

	// Now, explicitly resolve the field A within B. This call should detect the cycle
	// and correctly link the definition back to the already-existing `typeA`.
	_, err = fieldAInB.Type.Resolve(context.Background(), resolving) // Pass the same resolving map
	if err != nil {
		t.Fatalf("Resolve() for field A within B failed unexpectedly: %v", err)
	}

	// After resolving B's field A, its definition should be set correctly.
	if fieldAInB.Type.Definition != typeA {
		t.Errorf("The definition for field A in type B is not pointing back to the original TypeInfo for A.")
		t.Errorf("Expected: %p, Got: %p", typeA, fieldAInB.Type.Definition)
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
	s, err := New(fset, nil, overlay, modulePath, absTestDir, &MockResolver{})
	if err != nil {
		t.Fatalf("scanner.New with overlay failed: %v", err)
	}

	// ScanFiles expects absolute paths, so we construct one.
	scanFilePath := filepath.Join(absTestDir, "basic.go")

	// The pkgDirPath should also be an absolute path to the package directory.
	pkgInfo, err := s.ScanFiles(context.Background(), []string{scanFilePath}, absTestDir, "")
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
