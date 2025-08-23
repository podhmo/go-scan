package goscan

import (
	"go/token"
	"testing"

	"github.com/podhmo/go-scan/locator"
)

func TestConfigSharing(t *testing.T) {
	t.Run("it should share components when created with the same config", func(t *testing.T) {
		// 1. Create the shared components.
		fset := token.NewFileSet()
		loc, err := locator.New("./")
		if err != nil {
			t.Fatalf("locator.New() failed: %v", err)
		}

		config := &Config{
			Fset:    fset,
			Locator: loc,
		}

		// 2. Create a ModuleWalker and a Scanner with the same config.
		walker, err := NewModuleWalker(WithModuleWalkerConfig(config))
		if err != nil {
			t.Fatalf("NewModuleWalker() with config failed: %v", err)
		}

		scanner, err := New(WithConfig(config))
		if err != nil {
			t.Fatalf("New() with config failed: %v", err)
		}

		// 3. Assert that the internal components are the exact same instances.
		if walker.fset != scanner.fset {
			t.Errorf("FileSet instances are not shared. Walker: %p, Scanner: %p", walker.fset, scanner.fset)
		}
		if walker.locator != scanner.locator {
			t.Errorf("Locator instances are not shared. Walker: %p, Scanner: %p", walker.locator, scanner.locator)
		}

		// Also check against the original instances.
		if walker.fset != fset {
			t.Errorf("Walker FileSet is not the original instance. Walker: %p, Original: %p", walker.fset, fset)
		}
		if scanner.locator != loc {
			t.Errorf("Scanner Locator is not the original instance. Scanner: %p, Original: %p", scanner.locator, loc)
		}
	})

	t.Run("it should not share components when created without a config", func(t *testing.T) {
		walker, err := NewModuleWalker()
		if err != nil {
			t.Fatalf("NewModuleWalker() failed: %v", err)
		}

		scanner, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		if walker.fset == scanner.fset {
			t.Error("FileSet instances should not be shared when created without a config")
		}
		if walker.locator == scanner.locator {
			t.Error("Locator instances should not be shared when created without a config")
		}
	})
}
