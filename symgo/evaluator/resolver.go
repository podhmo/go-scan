package evaluator

import (
	"context"
	"fmt"

	goscan "github.com/podhmo/go-scan"
	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// Resolver handles decisions about whether to scan or resolve types.
//
// Exported methods must perform a policy check. If a method does not perform
// a policy check, it should be unexported.
// Bypassing the policy check is considered unsafe and should only be done when
// the caller can guarantee the safety of the operation.
type Resolver struct {
	ScanPolicy object.ScanPolicyFunc
	scanner    *goscan.Scanner
}

// NewResolver creates a new Resolver.
func NewResolver(policy object.ScanPolicyFunc, scanner *goscan.Scanner) *Resolver {
	if policy == nil {
		policy = func(pkgPath string) bool { return true }
	}
	return &Resolver{
		ScanPolicy: policy,
		scanner:    scanner,
	}
}

// ResolveType is a helper to resolve a FieldType to a TypeInfo while respecting the scan policy.
func (r *Resolver) ResolveType(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
	if fieldType == nil {
		return nil
	}

	// Policy check is performed here.
	if fieldType.FullImportPath != "" && !r.ScanPolicy(fieldType.FullImportPath) {
		// Policy says NO. Create a placeholder for the unresolved type.
		return scanner.NewUnresolvedTypeInfo(fieldType.FullImportPath, fieldType.TypeName)
	}

	// Policy allows scanning, or it's a local/built-in type.
	resolvedType, _ := fieldType.Resolve(ctx)
	return resolvedType
}

// resolveTypeWithoutPolicyCheck resolves a FieldType to a TypeInfo without enforcing the scan policy.
// This is considered unsafe and should only be used when the caller has already
// performed the necessary policy checks or when analyzing types that are known
// to be safe to resolve.
func (r *Resolver) resolveTypeWithoutPolicyCheck(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
	if fieldType == nil {
		return nil
	}

	// Policy check is intentionally skipped.
	resolvedType, _ := fieldType.Resolve(ctx)
	return resolvedType
}

// ResolveFunction creates a function object or a symbolic placeholder based on the scan policy.
func (r *Resolver) ResolveFunction(pkg *object.Package, funcInfo *scanner.FunctionInfo) object.Object {
	if r.ScanPolicy(pkg.Path) {
		return &object.Function{
			Name:       funcInfo.AstDecl.Name,
			Parameters: funcInfo.AstDecl.Type.Params,
			Body:       funcInfo.AstDecl.Body,
			Env:        pkg.Env,
			Decl:       funcInfo.AstDecl,
			Package:    pkg.ScannedInfo,
			Def:        funcInfo,
		}
	}
	// For out-of-policy packages, exported functions become placeholders.
	return &object.SymbolicPlaceholder{
		Reason:         "external function " + pkg.Path + "." + funcInfo.Name,
		UnderlyingFunc: funcInfo,
		Package:        pkg.ScannedInfo,
	}
}

// ResolveCompositeLit resolves the type for a composite literal, respecting the scan policy,
// and returns an appropriate object (Instance or SymbolicPlaceholder).
func (r *Resolver) ResolveCompositeLit(ctx context.Context, fieldType *scanner.FieldType) object.Object {
	// The initial check is done in the evaluator before calling this.
	// This function performs the type resolution and object creation.
	resolvedType := r.ResolveType(ctx, fieldType)
	if resolvedType == nil || resolvedType.Unresolved {
		// This can happen for built-in types or if resolution fails for other reasons.
		placeholder := &object.SymbolicPlaceholder{
			Reason: "unresolved composite literal of type " + fieldType.String(),
		}
		placeholder.SetFieldType(fieldType)
		return placeholder
	}

	instance := &object.Instance{
		TypeName: resolvedType.PkgPath + "." + resolvedType.Name,
		BaseObject: object.BaseObject{
			ResolvedTypeInfo: resolvedType,
		},
	}
	instance.SetFieldType(fieldType)
	return instance
}

// ResolveSymbolicField creates a symbolic placeholder for a field access on a symbolic value.
func (r *Resolver) ResolveSymbolicField(ctx context.Context, field *scanner.FieldInfo, receiver object.Object) object.Object {
	fieldTypeInfo := r.ResolveType(ctx, field.Type)
	var reason string
	if v, ok := receiver.(*object.Variable); ok {
		reason = "field access " + v.Name + "." + field.Name
	} else {
		reason = "field access on symbolic value " + receiver.Inspect() + "." + field.Name
	}
	return &object.SymbolicPlaceholder{
		BaseObject: object.BaseObject{ResolvedTypeInfo: fieldTypeInfo, ResolvedFieldType: field.Type},
		Reason:     reason,
	}
}


// ResolvePackage is a helper to get package info while respecting the scan policy.
func (r *Resolver) ResolvePackage(ctx context.Context, path string) (*scanner.PackageInfo, error) {
	if !r.ScanPolicy(path) {
		return nil, fmt.Errorf("package %q is excluded by scan policy", path)
	}
	return r.resolvePackageWithoutPolicyCheck(ctx, path)
}

// resolvePackageWithoutPolicyCheck resolves a package without enforcing the scan policy.
func (r *Resolver) resolvePackageWithoutPolicyCheck(ctx context.Context, path string) (*scanner.PackageInfo, error) {
	return r.scanner.ScanPackageByImport(ctx, path)
}
