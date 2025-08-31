package evaluator

import (
	"context"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// PolicyResolver handles decisions about whether to scan or resolve types.
type PolicyResolver struct {
	scanPolicy object.ScanPolicyFunc
}

// NewPolicyResolver creates a new PolicyResolver.
func NewPolicyResolver(policy object.ScanPolicyFunc) *PolicyResolver {
	return &PolicyResolver{
		scanPolicy: policy,
	}
}

// ResolveType is a helper to resolve a FieldType to a TypeInfo while respecting the scan policy.
func (pc *PolicyResolver) ResolveType(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
	if fieldType == nil {
		return nil
	}
	if pc.scanPolicy != nil && fieldType.FullImportPath != "" && !pc.scanPolicy(fieldType.FullImportPath) {
		return scanner.NewUnresolvedTypeInfo(fieldType.FullImportPath, fieldType.TypeName)
	}
	// Policy allows scanning, or it's a local/built-in type.
	resolvedType, _ := fieldType.Resolve(ctx)
	return resolvedType
}
