package evaluator

import (
	"context"
	"fmt"
	"log/slog"

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
	logger     *slog.Logger
}

// NewResolver creates a new Resolver.
func NewResolver(policy object.ScanPolicyFunc, scanner *goscan.Scanner, logger *slog.Logger) *Resolver {
	if policy == nil {
		policy = func(pkgPath string) bool { return true }
	}
	return &Resolver{
		ScanPolicy: policy,
		scanner:    scanner,
		logger:     logger,
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
	resolvedType, err := fieldType.Resolve(ctx)
	if err != nil {
		// If resolution fails (e.g., package not found for shallow scan),
		// it's not a fatal error for the resolver. Return a placeholder.
		r.logger.DebugContext(ctx, "type resolution failed, returning placeholder", "type", fieldType.String(), "error", err)
		return scanner.NewUnresolvedTypeInfo(fieldType.FullImportPath, fieldType.TypeName)
	}
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
	resolvedType, err := fieldType.Resolve(ctx)
	if err != nil {
		// If resolution fails, return a placeholder.
		r.logger.DebugContext(ctx, "type resolution failed (without policy check), returning placeholder", "type", fieldType.String(), "error", err)
		return scanner.NewUnresolvedTypeInfo(fieldType.FullImportPath, fieldType.TypeName)
	}
	return resolvedType
}

// ResolveFunction creates a function object or a symbolic placeholder based on the scan policy.
func (r *Resolver) ResolveFunction(ctx context.Context, pkg *object.Package, funcInfo *scanner.FunctionInfo) object.Object {
	if r.ScanPolicy(pkg.Path) {
		fn := &object.Function{
			Name:       funcInfo.AstDecl.Name,
			Parameters: funcInfo.AstDecl.Type.Params,
			Body:       funcInfo.AstDecl.Body,
			Env:        pkg.Env,
			Decl:       funcInfo.AstDecl,
			Package:    pkg.ScannedInfo,
			Def:        funcInfo,
		}

		// If the function is a method, create a placeholder for the receiver.
		if funcInfo.Receiver != nil {
			receiverVar := &object.Variable{
				Name: funcInfo.Receiver.Name,
			}
			receiverVar.SetFieldType(funcInfo.Receiver.Type)
			// The receiver's type info can be resolved from its field type.
			// This must respect the scan policy.
			receiverVar.SetTypeInfo(r.ResolveType(ctx, funcInfo.Receiver.Type))
			fn.Receiver = receiverVar
		}

		return fn
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

// ResolvePackage is a helper to get package info while respecting the scan policy.
func (r *Resolver) ResolvePackage(ctx context.Context, path string) (*scanner.PackageInfo, error) {
	r.logger.DebugContext(ctx, "ResolvePackage: checking policy", "path", path)
	if !r.ScanPolicy(path) {
		r.logger.DebugContext(ctx, "ResolvePackage: denied by policy", "path", path)
		return nil, fmt.Errorf("package %q is excluded by scan policy", path)
	}
	r.logger.DebugContext(ctx, "ResolvePackage: allowed by policy, scanning", "path", path)
	return r.resolvePackageWithoutPolicyCheck(ctx, path)
}

// resolvePackageWithoutPolicyCheck resolves a package without enforcing the scan policy.
func (r *Resolver) resolvePackageWithoutPolicyCheck(ctx context.Context, path string) (*scanner.PackageInfo, error) {
	return r.scanner.ScanPackageFromImportPath(ctx, path)
}
