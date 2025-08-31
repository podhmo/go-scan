package evaluator

import (
	"context"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
)

func TestResolver(t *testing.T) {
	ctx := context.Background()

	// Define a scan policy that only allows packages within "example.com/myapp".
	scanPolicy := func(pkgPath string) bool {
		return pkgPath == "example.com/myapp" || pkgPath == "example.com/myapp/models"
	}

	// Use scantest to set up the files in a temporary directory.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/myapp\n",
		"main.go": `
package main
import (
	"example.com/myapp/models"
	"example.com/external/money"
)
type Request struct {
	User models.User
	Price money.Price
}`,
		"models/user.go": `
package models
type User struct {
	ID   string
	Name string
}`,
		"external/money.go": `
package money
type Price struct {
	Amount   int
	Currency string
}`,
	})
	defer cleanup()

	// Create a new scanner configured to work in the temp directory.
	s, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	// Scan the main package.
	pkgs, err := s.Scan(ctx, ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %v", err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, but got %d", len(pkgs))
	}

	mainPkg := pkgs[0]
	if mainPkg.ImportPath != "example.com/myapp" {
		t.Fatalf("expected main package, but got %s", mainPkg.ImportPath)
	}

	reqType, ok := mainPkg.Types["Request"]
	if !ok {
		t.Fatal("Request type not found")
	}
	if reqType.Struct == nil {
		t.Fatal("Request is not a struct")
	}

	// Get the FieldType for the User and Price fields.
	var userFieldType, priceFieldType *scanner.FieldType
	for _, f := range reqType.Struct.Fields {
		if f.Name == "User" {
			userFieldType = f.Type
		}
		if f.Name == "Price" {
			priceFieldType = f.Type
		}
	}
	if userFieldType == nil {
		t.Fatal("User field not found")
	}
	if priceFieldType == nil {
		t.Fatal("Price field not found")
	}

	// Create a resolver with the defined policy.
	resolver := NewResolver(scanPolicy)

	t.Run("ResolveType with policy (allowed)", func(t *testing.T) {
		_, err := userFieldType.Resolve(ctx)
		if err != nil {
			t.Fatalf("pre-resolve failed for allowed type: %v", err)
		}

		result := resolver.ResolveType(ctx, userFieldType)
		if result == nil {
			t.Fatal("should resolve allowed type, but got nil")
		}
		if result.Unresolved {
			t.Error("should not be unresolved")
		}
		if result.Name != "User" {
			t.Errorf("expected name User, but got %s", result.Name)
		}
		if result.PkgPath != "example.com/myapp/models" {
			t.Errorf("expected pkg path example.com/myapp/models, but got %s", result.PkgPath)
		}
	})

	t.Run("ResolveType with policy (disallowed)", func(t *testing.T) {
		_, err := priceFieldType.Resolve(ctx)
		if err != nil {
			t.Fatalf("pre-resolve failed for disallowed type: %v", err)
		}

		result := resolver.ResolveType(ctx, priceFieldType)
		if result == nil {
			t.Fatal("should return a type info for disallowed type, but got nil")
		}
		if !result.Unresolved {
			t.Error("should be unresolved due to policy")
		}
		if result.Name != "Price" {
			t.Errorf("expected name Price, but got %s", result.Name)
		}
		if result.PkgPath != "example.com/external/money" {
			t.Errorf("expected pkg path example.com/external/money, but got %s", result.PkgPath)
		}
	})

	t.Run("resolveTypeWithoutPolicyCheck (disallowed)", func(t *testing.T) {
		result := resolver.resolveTypeWithoutPolicyCheck(ctx, priceFieldType)
		if result == nil {
			t.Fatal("should resolve disallowed type when policy is skipped, but got nil")
		}
		if result.Unresolved {
			t.Error("should not be unresolved when policy is skipped")
		}
		if result.Name != "Price" {
			t.Errorf("expected name Price, but got %s", result.Name)
		}
		if result.PkgPath != "example.com/external/money" {
			t.Errorf("expected pkg path example.com/external/money, but got %s", result.PkgPath)
		}
	})

	t.Run("ResolveType with nil fieldType", func(t *testing.T) {
		result := resolver.ResolveType(ctx, nil)
		if result != nil {
			t.Error("should return nil for nil fieldType")
		}
	})

	t.Run("resolveTypeWithoutPolicyCheck with nil fieldType", func(t *testing.T) {
		result := resolver.resolveTypeWithoutPolicyCheck(ctx, nil)
		if result != nil {
			t.Error("should return nil for nil fieldType")
		}
	})
}
