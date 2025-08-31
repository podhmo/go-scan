package evaluator

import (
	"context"
	"fmt"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// Resolver handles decisions about whether to scan or resolve types and packages.
//
// # Naming Convention for Policy Checks
//
// Methods on this struct follow a strict convention regarding policy checks:
//   - Exported methods (e.g., ResolvePackage, ResolveFunction) are "safe" and MUST
//     perform a policy check before proceeding.
//   - Unexported methods that bypass policy checks MUST be named with a suffix
//     like `...WithoutPolicyCheck`.
type Resolver struct {
	scanPolicy object.ScanPolicyFunc
	scanner    *goscan.Scanner
	pkgCache   map[string]*object.Package
}

// NewResolver creates a new Resolver.
func NewResolver(scanner *goscan.Scanner, policy object.ScanPolicyFunc) *Resolver {
	return &Resolver{
		scanner:    scanner,
		scanPolicy: policy,
		pkgCache:   make(map[string]*object.Package),
	}
}

// resolvePackageWithoutPolicyCheck is the underlying implementation for package resolution.
// It scans a package and caches it, but performs no policy checks.
// The caller is responsible for enforcing policy.
func (r *Resolver) resolvePackageWithoutPolicyCheck(ctx context.Context, path string) (*object.Package, error) {
	if pkg, ok := r.pkgCache[path]; ok {
		// If the cached item has been scanned, return it.
		// If it was a placeholder from a failed scan or policy block,
		// we continue and attempt to scan it properly now.
		if pkg.ScannedInfo != nil {
			return pkg, nil
		}
	}

	scannedPkg, err := r.scanner.ScanPackageByImport(ctx, path)
	if err != nil {
		// Cache the failure so we don't try again.
		pkgObj := &object.Package{Path: path, Env: object.NewEnvironment(), ScannedInfo: nil}
		r.pkgCache[path] = pkgObj
		return nil, fmt.Errorf("could not scan package %q: %w", path, err)
	}

	pkgObj := &object.Package{
		Name:        scannedPkg.Name,
		Path:        scannedPkg.ImportPath,
		Env:         object.NewEnvironment(),
		ScannedInfo: scannedPkg,
	}
	r.pkgCache[path] = pkgObj
	return pkgObj, nil
}

// ResolvePackage is the "safe" default method for package resolution.
// It respects the scan policy, returning a placeholder with no ScannedInfo for out-of-policy packages.
func (r *Resolver) ResolvePackage(ctx context.Context, path string) (*object.Package, error) {
	if pkg, ok := r.pkgCache[path]; ok {
		return pkg, nil
	}

	// Policy check: If the package is not allowed, we don't scan it.
	if !r.ScanPolicy(path) {
		pkgObj := &object.Package{
			Name:        "", // We don't know the name yet
			Path:        path,
			Env:         object.NewEnvironment(),
			ScannedInfo: nil, // Mark as not scanned
		}
		r.pkgCache[path] = pkgObj
		return pkgObj, nil
	}

	// Policy allows scanning.
	return r.resolvePackageWithoutPolicyCheck(ctx, path)
}

// ResolvePackageInfo is a special-purpose method to get package information (declarations)
// regardless of the scan policy. This is considered "unsafe" from an evaluation perspective
// as it loads packages that might otherwise be excluded. It should only be used for
// tasks like resolving function signatures.
func (r *Resolver) ResolvePackageInfo(ctx context.Context, path string) (*object.Package, error) {
	// This method intentionally bypasses the ScanPolicy check.
	return r.resolvePackageWithoutPolicyCheck(ctx, path)
}

// ResolveFunction creates a function object or a placeholder based on the scan policy.
func (r *Resolver) ResolveFunction(pkg *object.Package, f *scanner.FunctionInfo) object.Object {
	if r.ScanPolicy(pkg.Path) {
		return &object.Function{
			Name:       f.AstDecl.Name,
			Parameters: f.AstDecl.Type.Params,
			Body:       f.AstDecl.Body,
			Env:        pkg.Env,
			Decl:       f.AstDecl,
			Package:    pkg.ScannedInfo,
			Def:        f,
		}
	}
	return &object.SymbolicPlaceholder{
		Reason:         fmt.Sprintf("external function %s.%s", pkg.Path, f.Name),
		UnderlyingFunc: f,
		Package:        pkg.ScannedInfo,
	}
}

// ScanPolicy checks if a package path is allowed by the scan policy.
func (r *Resolver) ScanPolicy(path string) bool {
	if r.scanPolicy == nil {
		return true // No policy means allow all.
	}
	return r.scanPolicy(path)
}

// ResolveType is a helper to resolve a FieldType to a TypeInfo while respecting the scan policy.
func (r *Resolver) ResolveType(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
	if fieldType == nil {
		return nil
	}
	if fieldType.FullImportPath != "" && !r.ScanPolicy(fieldType.FullImportPath) {
		return scanner.NewUnresolvedTypeInfo(fieldType.FullImportPath, fieldType.TypeName)
	}
	// Policy allows scanning, or it's a local/built-in type.
	resolvedType, _ := fieldType.Resolve(ctx)
	return resolvedType
}
