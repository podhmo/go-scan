package evaluator

import (
	"io"
	"log/slog"
	"testing"

	"github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/scantest"
)

func TestResolver(t *testing.T) {
	ctx := t.Context()

	// Define a scan policy that only allows packages within "example.com/myapp".
	scanPolicy := func(pkgPath string) bool {
		return pkgPath == "example.com/myapp" || pkgPath == "example.com/myapp/models"
	}

	// Use scantest to set up the files in a temporary directory.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": `
module example.com/myapp

replace example.com/external/money => ./external/money
`,
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
		"external/money/go.mod": "module example.com/external/money\n",
		"external/money/money.go": `
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

	var reqType *scanner.TypeInfo
	for _, ti := range mainPkg.Types {
		if ti.Name == "Request" {
			reqType = ti
			break
		}
	}

	if reqType == nil {
		t.Fatal("Request type not found")
	}
	if reqType.Struct == nil {
		t.Fatal("Request is not a struct")
	}

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

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	resolver := NewResolver(scanPolicy, s, logger)

	t.Run("ResolveType with policy (allowed)", func(t *testing.T) {
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
		result := resolver.ResolveType(ctx, priceFieldType)
		if result == nil {
			t.Fatal("should return a type info for disallowed type, but got nil")
		}
		if !result.Unresolved {
			t.Error("should be unresolved due to policy")
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
	})
}

func TestResolver_withError(t *testing.T) {
	ctx := t.Context()

	// Use scantest to set up a file with an import that cannot be resolved.
	dir, cleanup := scantest.WriteFiles(t, map[string]string{
		"go.mod": "module example.com/myapp\n",
		"main.go": `
package main
import "example.com/nonexistent"
type Request struct {
	Thing nonexistent.Thing
}`,
	})
	defer cleanup()

	s, err := goscan.New(
		goscan.WithWorkDir(dir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("goscan.New() failed: %v", err)
	}

	// Scanning itself won't fail, but type resolution will.
	pkgs, err := s.Scan(ctx, ".")
	if err != nil {
		t.Fatalf("s.Scan() failed: %v", err)
	}
	mainPkg := pkgs[0]
	reqType := mainPkg.Types[0]
	thingFieldType := reqType.Struct.Fields[0].Type

	// Create a resolver with a permissive policy.
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	resolver := NewResolver(func(pkgPath string) bool { return true }, s, logger)

	t.Run("ResolveType with resolution error returns placeholder", func(t *testing.T) {
		result := resolver.ResolveType(ctx, thingFieldType)
		if result == nil {
			t.Fatal("expected a placeholder for resolution error, but got nil")
		}
		if !result.Unresolved {
			t.Error("expected placeholder to be unresolved")
		}
	})

	t.Run("resolveTypeWithoutPolicyCheck with resolution error returns placeholder", func(t *testing.T) {
		result := resolver.resolveTypeWithoutPolicyCheck(ctx, thingFieldType)
		if result == nil {
			t.Fatal("expected a placeholder for resolution error, but got nil")
		}
		if !result.Unresolved {
			t.Error("expected placeholder to be unresolved")
		}
	})
}
