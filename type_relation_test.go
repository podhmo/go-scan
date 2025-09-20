package goscan

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/scanner"
)

func TestImplements_New(t *testing.T) {
	ctx := context.Background()
	s, err := New(WithWorkDir("./testdata/implements2"))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Pre-scan all packages to populate the scanner's cache.
	// This simulates a real-world scenario where multiple packages are scanned.
	// Use absolute paths constructed from the locator's root to be robust.
	ifacesPath := filepath.Join(s.locator.RootDir(), "ifaces")
	implsPath := filepath.Join(s.locator.RootDir(), "impls")

	_, err = s.ScanPackage(ctx, ifacesPath)
	if err != nil {
		t.Fatalf("ScanPackage(%q) failed: %v", ifacesPath, err)
	}
	_, err = s.ScanPackage(ctx, implsPath)
	if err != nil {
		t.Fatalf("ScanPackage(%q) failed: %v", implsPath, err)
	}

	// Helper to get a TypeInfo from a fully qualified name
	getType := func(name string) *scanner.TypeInfo {
		parts := strings.Split(name, ".")
		pkgPath := strings.Join(parts[:len(parts)-1], ".")
		typeName := parts[len(parts)-1]

		pkgInfo, ok := s.packageCache[pkgPath]
		if !ok {
			// As a fallback for fully qualified paths that might not match the cache key exactly
			// (e.g. example.com/implements2/impls vs example.com/implements2/impls), we iterate.
			// This is not efficient but robust for tests.
			for path, pi := range s.packageCache {
				if strings.HasSuffix(path, pkgPath) {
					pkgInfo = pi
					ok = true
					break
				}
			}
		}

		if !ok {
			t.Fatalf("Package %q not found in scanner cache", pkgPath)
		}

		ti := pkgInfo.Lookup(typeName)
		if ti == nil {
			t.Fatalf("TypeInfo for %q not found in package %q", typeName, pkgPath)
		}
		return ti
	}

	tests := []struct {
		name                string
		structName          string
		interfaceName       string
		expectedToImplement bool
	}{
		// Positive cases
		{"MyReader implements SimpleReader", "example.com/implements2/impls.MyReader", "example.com/implements2/ifaces.SimpleReader", true},
		{"MyEmbeddedReader implements EmbeddedReader", "example.com/implements2/impls.MyEmbeddedReader", "example.com/implements2/ifaces.EmbeddedReader", true},
		{"MyEmbeddedReader implements SimpleReader (via EmbeddedReader)", "example.com/implements2/impls.MyEmbeddedReader", "example.com/implements2/ifaces.SimpleReader", true},
		{"StructWithEmbeddedConcrete implements AnotherInterface", "example.com/implements2/impls.StructWithEmbeddedConcrete", "example.com/implements2/ifaces.AnotherInterface", true},

		// Negative cases
		{"NonImplementer does not implement SimpleReader", "example.com/implements2/impls.NonImplementer", "example.com/implements2/ifaces.SimpleReader", false},
		{"PartialImplementer does not implement EmbeddedReader", "example.com/implements2/impls.PartialImplementer", "example.com/implements2/ifaces.EmbeddedReader", false},
		{"MyReader does not implement EmbeddedReader", "example.com/implements2/impls.MyReader", "example.com/implements2/ifaces.EmbeddedReader", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			structCandidate := getType(tt.structName)
			interfaceDef := getType(tt.interfaceName)

			actual := s.Implements(ctx, structCandidate, interfaceDef)
			if actual != tt.expectedToImplement {
				t.Errorf("s.Implements(%s, %s): expected %v, got %v", tt.structName, tt.interfaceName, tt.expectedToImplement, actual)
			}
		})
	}
}

func TestImplements_FromDerivingJSON(t *testing.T) {
	ctx := context.Background()
	s, err := New(WithWorkDir("./testdata/implements3"))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// This simulates the scenario where a package is scanned, and then later
	// an Implements check is performed. The initial scan might be incomplete
	// in some scenarios, which the Implements logic needs to handle.
	pkg, err := s.ScanPackage(ctx, "impls")
	if err != nil {
		t.Fatalf("ScanPackage(\"impls\") failed: %v", err)
	}

	eventInfo := pkg.Lookup("Event")
	if eventInfo == nil {
		t.Fatalf("Type 'Event' not found in package")
	}
	var eventDataField *scanner.FieldInfo
	for _, f := range eventInfo.Struct.Fields {
		if f.Name == "Data" {
			eventDataField = f
			break
		}
	}
	if eventDataField == nil {
		t.Fatal("Field 'Data' not found in 'Event' struct")
	}

	eventDataInfo, err := eventDataField.Type.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolving 'EventData' failed: %v", err)
	}
	if eventDataInfo.Kind != scanner.InterfaceKind {
		t.Fatalf("Expected 'EventData' to be an interface, but it's %v", eventDataInfo.Kind)
	}

	userCreatedInfo := pkg.Lookup("UserCreated")
	if userCreatedInfo == nil {
		t.Fatalf("Type 'UserCreated' not found in package")
	}

	if !s.Implements(ctx, userCreatedInfo, eventDataInfo) {
		t.Errorf("Expected UserCreated to implement EventData, but it did not")
	}
}
