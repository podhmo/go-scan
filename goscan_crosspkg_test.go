package goscan_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	scan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scantest"
)

func TestFieldType_Resolve_CrossPackage(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/test",
		"models/user.go": `
package models
type User struct {
	ID   string
	Name string
}`,
		"api/handler.go": `
package api
import "example.com/test/models"
type Handler struct {
	User models.User
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// We scan the 'api' package. The 'models' package should be resolved on-demand.
	_, err := scantest.Run(t, dir, []string{"api"}, func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		if len(pkgs) != 1 {
			return fmt.Errorf("expected 1 package, got %d", len(pkgs))
		}
		apiPkg := pkgs[0]
		handlerType := apiPkg.Lookup("Handler")
		if handlerType == nil {
			return fmt.Errorf("type Handler not found in package api")
		}
		if handlerType.Struct == nil {
			return fmt.Errorf("Handler is not a struct")
		}
		if len(handlerType.Struct.Fields) != 1 {
			return fmt.Errorf("expected 1 field in Handler, got %d", len(handlerType.Struct.Fields))
		}

		userField := handlerType.Struct.Fields[0]
		if userField.Name != "User" {
			return fmt.Errorf("expected field name 'User', got %s", userField.Name)
		}

		// --- Act ---
		// Resolve the type of the 'User' field.
		resolvedType, err := userField.Type.Resolve(ctx, make(map[string]struct{}))
		if err != nil {
			return fmt.Errorf("Resolve() failed: %w", err)
		}

		// --- Assert ---
		if resolvedType == nil {
			return fmt.Errorf("Resolve() returned nil, expected a TypeInfo for models.User")
		}

		if diff := cmp.Diff("User", resolvedType.Name); diff != "" {
			return fmt.Errorf("resolved type name mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("example.com/test/models", resolvedType.PkgPath); diff != "" {
			return fmt.Errorf("resolved type pkgPath mismatch (-want +got):\n%s", diff)
		}

		// Test for idempotency: resolve again.
		resolvedType2, err := userField.Type.Resolve(ctx, make(map[string]struct{}))
		if err != nil {
			return fmt.Errorf("second Resolve() failed: %w", err)
		}
		if resolvedType2 == nil {
			return fmt.Errorf("second Resolve() returned nil")
		}
		// Check if it's the same instance (pointer equality)
		if resolvedType != resolvedType2 {
			return fmt.Errorf("Resolve() is not idempotent; returned different instances")
		}

		return nil
	})

	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}

func TestFieldType_Resolve_NestedCrossPackage(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/test",
		"models/profile.go": `
package models
type Profile struct {
	Email string
}`,
		"models/user.go": `
package models
type User struct {
	ID   string
	Name string
}`,
		"dmodels/profile.go": `
package dmodels
// d is for destination
type Profile struct {
	Email string
}`,
		"api/payloads.go": `
package api
import "example.com/test/models"
type UserCreateRequest struct {
	Name string
	Profile models.Profile
}`,
		"destination/types.go": `
package destination
import "example.com/test/dmodels"
type User struct {
	ID string
	Name string
	Profile dmodels.Profile
}`,
	}

	dir, cleanup := scantest.WriteFiles(t, files)
	defer cleanup()

	// We scan 'api' and 'destination' packages initially.
	// 'models' and 'dmodels' should be resolved on-demand.
	_, err := scantest.Run(t, dir, []string{"api", "destination"}, func(ctx context.Context, s *scan.Scanner, pkgs []*scan.Package) error {
		if len(pkgs) != 2 {
			return fmt.Errorf("expected 2 packages, got %d", len(pkgs))
		}

		var apiPkg, destPkg *scan.Package
		for _, p := range pkgs {
			if p.Name == "api" {
				apiPkg = p
			}
			if p.Name == "destination" {
				destPkg = p
			}
		}
		if apiPkg == nil || destPkg == nil {
			return fmt.Errorf("could not find both api and destination packages in initial scan")
		}

		// Find the source struct: api.UserCreateRequest
		srcType := apiPkg.Lookup("UserCreateRequest")
		if srcType == nil {
			return fmt.Errorf("type UserCreateRequest not found in package api")
		}
		srcProfileField := srcType.Struct.Fields[1] // Profile models.Profile

		// Find the destination struct: destination.User
		dstType := destPkg.Lookup("User")
		if dstType == nil {
			return fmt.Errorf("type User not found in package destination")
		}
		dstProfileField := dstType.Struct.Fields[2] // Profile dmodels.Profile

		// --- Act & Assert ---
		// 1. Resolve the source profile type
		srcProfileResolved, err := srcProfileField.Type.Resolve(ctx, make(map[string]struct{}))
		if err != nil {
			return fmt.Errorf("resolving source Profile failed: %w", err)
		}
		if srcProfileResolved == nil {
			return fmt.Errorf("resolving source Profile returned nil")
		}
		if diff := cmp.Diff("Profile", srcProfileResolved.Name); diff != "" {
			return fmt.Errorf("source resolved name mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("example.com/test/models", srcProfileResolved.PkgPath); diff != "" {
			return fmt.Errorf("source resolved pkgPath mismatch (-want +got):\n%s", diff)
		}

		// 2. Resolve the destination profile type
		dstProfileResolved, err := dstProfileField.Type.Resolve(ctx, make(map[string]struct{}))
		if err != nil {
			return fmt.Errorf("resolving destination Profile failed: %w", err)
		}
		if dstProfileResolved == nil {
			return fmt.Errorf("resolving destination Profile returned nil")
		}
		if diff := cmp.Diff("Profile", dstProfileResolved.Name); diff != "" {
			return fmt.Errorf("destination resolved name mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff("example.com/test/dmodels", dstProfileResolved.PkgPath); diff != "" {
			return fmt.Errorf("destination resolved pkgPath mismatch (-want +got):\n%s", diff)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("scantest.Run() failed: %v", err)
	}
}
