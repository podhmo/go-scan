package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"

	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) getOrLoadPackage(ctx context.Context, path string) (*object.Package, error) {
	e.evaluatingMu.Lock()
	if e.evaluating[path] {
		e.evaluatingMu.Unlock()
		e.logc(ctx, slog.LevelError, "recursion detected: already evaluating package", "path", path)
		return nil, fmt.Errorf("infinite recursion detected in package loading: %s", path)
	}
	e.evaluating[path] = true
	e.evaluatingMu.Unlock()

	defer func() {
		e.evaluatingMu.Lock()
		delete(e.evaluating, path)
		e.evaluatingMu.Unlock()
	}()

	e.logc(ctx, slog.LevelDebug, "getOrLoadPackage: requesting package", "path", path)
	if pkg, ok := e.pkgCache[path]; ok {
		e.logc(ctx, slog.LevelDebug, "getOrLoadPackage: found in cache", "path", path, "scanned", pkg.ScannedInfo != nil)
		// Ensure even cached packages are populated if they were created as placeholders first.
		e.ensurePackageEnvPopulated(ctx, pkg)
		return pkg, nil
	}

	// Use the policy-enforcing ResolvePackage method.
	scannedPkg, err := e.resolver.ResolvePackage(ctx, path)
	if err != nil {
		// This error now occurs if the package is excluded by policy OR if scanning fails.
		// In either case, we create a placeholder package object to cache the result
		// and avoid re-scanning. The ScannedInfo will be nil.
		e.logc(ctx, slog.LevelDebug, "package resolution failed or denied by policy", "package", path, "error", err)
		pkgObj := &object.Package{
			Name:        "", // We don't know the name yet.
			Path:        path,
			Env:         object.NewEnclosedEnvironment(e.UniverseEnv),
			ScannedInfo: nil, // Mark as not scanned.
		}
		e.pkgCache[path] = pkgObj
		// We return the placeholder object itself, not an error, because failing to load
		// a package due to policy is not an evaluation-halting error.
		return pkgObj, nil
	}

	pkgObj := &object.Package{
		Name:        scannedPkg.Name,
		Path:        scannedPkg.ImportPath,
		Env:         object.NewEnclosedEnvironment(e.UniverseEnv),
		ScannedInfo: scannedPkg,
	}

	e.ensurePackageEnvPopulated(ctx, pkgObj)
	e.pkgCache[path] = pkgObj
	return pkgObj, nil
}

func (e *Evaluator) ensurePackageEnvPopulated(ctx context.Context, pkgObj *object.Package) {
	e.logc(ctx, slog.LevelDebug, "ensurePackageEnvPopulated: checking package", "path", pkgObj.Path, "scanned", pkgObj.ScannedInfo != nil)
	if pkgObj.ScannedInfo == nil {
		return // Not scanned yet, nothing to populate.
	}

	// If we have already populated this package's environment, do nothing.
	if e.initializedPkgs[pkgObj.Path] {
		return
	}

	pkgInfo := pkgObj.ScannedInfo
	env := pkgObj.Env
	shouldScan := e.resolver.ScanPolicy(pkgInfo.ImportPath)

	e.logger.DebugContext(ctx, "populating package-level constants", "package", pkgInfo.ImportPath)

	// Populate constants
	for _, c := range pkgInfo.Constants {
		if !shouldScan && !c.IsExported {
			continue
		}
		constObj := e.convertGoConstant(ctx, c.ConstVal, token.NoPos)
		if isError(constObj) {
			e.logc(ctx, slog.LevelWarn, "could not convert constant to object", "const", c.Name, "error", constObj)
			continue
		}
		env.SetLocal(c.Name, constObj)
	}

	// Populate types
	e.logger.DebugContext(ctx, "populating package-level types", "package", pkgInfo.ImportPath)
	for _, t := range pkgInfo.Types {
		if !shouldScan && !ast.IsExported(t.Name) {
			continue
		}
		typeObj := &object.Type{
			TypeName:     t.Name,
			ResolvedType: t,
		}
		typeObj.SetTypeInfo(t)
		env.SetLocal(t.Name, typeObj)
	}

	// Populate variables (lazily)
	for _, v := range pkgInfo.Variables {
		if !shouldScan && !v.IsExported {
			continue
		}
		if v.GenDecl == nil {
			continue
		}

		// A single var declaration can have multiple specs (e.g., var ( a=1; b=2 )).
		// We need to find the right spec for the current variable `v`.
		for _, spec := range v.GenDecl.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				// Check if this spec contains our variable `v.Name`.
				var valueIndex = -1
				for i, nameIdent := range vs.Names {
					if nameIdent.Name == v.Name {
						valueIndex = i
						break
					}
				}

				// If we found our variable in this spec, determine its initializer.
				if valueIndex != -1 {
					var initializer ast.Expr
					// Case 1: var a, b = 1, 2 (1-to-1 mapping)
					if len(vs.Values) == len(vs.Names) {
						initializer = vs.Values[valueIndex]
					}
					// Case 2: var a, b = f() (multi-value return from a single call)
					if len(vs.Values) == 1 {
						initializer = vs.Values[0]
					}
					// Case 3: var a, b string (no initializer) -> initializer remains nil.

					lazyVar := &object.Variable{
						Name:        v.Name,
						IsEvaluated: false,
						Initializer: initializer,
						DeclEnv:     env,
						DeclPkg:     pkgInfo,
					}
					lazyVar.SetFieldType(v.Type) // Set the static type from the declaration
					env.SetLocal(v.Name, lazyVar)
					break // Found the right spec, move to the next variable in pkgInfo.Variables
				}
			}
		}
	}

	// Populate functions
	for _, f := range pkgInfo.Functions {
		if !shouldScan && !ast.IsExported(f.Name) {
			continue
		}
		fnObject := e.getOrResolveFunction(ctx, pkgObj, f)
		env.SetLocal(f.Name, fnObject)
	}

	// Mark this package as fully populated.
	e.initializedPkgs[pkgObj.Path] = true
}
