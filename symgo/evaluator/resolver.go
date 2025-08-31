package evaluator

import (
	"context"

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
	scanPolicy object.ScanPolicyFunc
}

// NewResolver creates a new Resolver.
func NewResolver(policy object.ScanPolicyFunc) *Resolver {
	return &Resolver{
		scanPolicy: policy,
	}
}

// ResolveType is a helper to resolve a FieldType to a TypeInfo while respecting the scan policy.
func (r *Resolver) ResolveType(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
	return r.resolveType(ctx, fieldType, true)
}

// resolveTypeWithoutPolicyCheck resolves a FieldType to a TypeInfo without enforcing the scan policy.
// This is considered unsafe and should only be used when the caller has already
// performed the necessary policy checks or when analyzing types that are known
// to be safe to resolve.
func (r *Resolver) resolveTypeWithoutPolicyCheck(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
	return r.resolveType(ctx, fieldType, false)
}

// resolveType is the internal implementation for resolving a FieldType to a TypeInfo.
// It respects the scan policy only if shouldScan is true.
func (r *Resolver) resolveType(ctx context.Context, fieldType *scanner.FieldType, shouldScan bool) *scanner.TypeInfo {
	if fieldType == nil {
		return nil
	}

	// Only perform policy check if requested.
	if shouldScan && r.scanPolicy != nil && fieldType.FullImportPath != "" && !r.scanPolicy(fieldType.FullImportPath) {
		// Policy says NO. Create a placeholder for the unresolved type.
		return scanner.NewUnresolvedTypeInfo(fieldType.FullImportPath, fieldType.TypeName)
	}

	// Policy allows scanning, or it's a local/built-in type, or scanning was skipped.
	resolvedType, _ := fieldType.Resolve(ctx)
	return resolvedType
}
