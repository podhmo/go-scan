package evaluator

import (
	"context"

	"github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

// PolicyChecker handles decisions about whether to scan or resolve types.
type PolicyChecker struct {
	scanPolicy object.ScanPolicyFunc
}

// NewPolicyChecker creates a new PolicyChecker.
func NewPolicyChecker(policy object.ScanPolicyFunc) *PolicyChecker {
	return &PolicyChecker{
		scanPolicy: policy,
	}
}

// ResolveType is a helper to resolve a FieldType to a TypeInfo while respecting the scan policy.
func (pc *PolicyChecker) ResolveType(ctx context.Context, fieldType *scanner.FieldType) *scanner.TypeInfo {
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
