# Analysis and Fix for Unexported Symbol Resolution in `symgo`

## 1. Summary of the Problem

The `symgo` symbolic execution engine failed to resolve unexported package-level constants and variables when they were accessed through a nested, cross-package call. This occurred because the package environment (`object.Environment`) for the called function was not consistently and fully populated before the function's body was evaluated.

The root causes identified were:
1.  **Inconsistent Package Object Management**: `*object.Package` instances, which hold the environment, were being created ad-hoc in different parts of the evaluator, leading to a fragmented and incomplete view of a package's symbols.
2.  **Incorrect Environment in Method Resolution**: The `findDirectMethodOnType` function was incorrectly assigning the caller's environment to method objects instead of the environment of the package where the method was defined.
3.  **Flawed Pointer-Receiver Logic**: The logic for resolving pointer-receiver methods on addressable value types was too strict, failing to find valid methods.
4.  **Flawed Package Name Resolution**: The logic for resolving package names for imports without an explicit alias was based on a flawed heuristic (using the last part of the import path) which fails for package paths like `gopkg.in/yaml.v2`.

## 2. The Solution: Centralized Package Management

To fix this robustly, the management of package objects within the `symgo.Evaluator` was refactored to use a central cache. This ensures that for any given import path, a single, canonical `*object.Package` instance is created, populated, and used throughout the evaluation.

### Step 2.1: Modify `symgo/evaluator/evaluator.go`

#### 2.1.1. Update `Evaluator` Struct

A `pkgCache` map was added to the `Evaluator` struct to store canonical package objects.

```go
// symgo/evaluator/evaluator.go

// Evaluator is the main object that evaluates the AST.
type Evaluator struct {
	scanner           *goscan.Scanner
	intrinsics        *intrinsics.Registry
	logger            *slog.Logger
	tracer            object.Tracer // Tracer for debugging evaluation flow.
	callStack         []*callFrame
	interfaceBindings map[string]*goscan.TypeInfo
	defaultIntrinsic  intrinsics.IntrinsicFunc
	scanPolicy        object.ScanPolicyFunc
	initializedPkgs   map[string]bool // To track packages whose constants are loaded
	pkgCache          map[string]*object.Package
}
```

#### 2.1.2. Initialize `pkgCache` in `New`

The new map was initialized in the `New` function.

```go
// symgo/evaluator/evaluator.go

// New creates a new Evaluator.
func New(scanner *goscan.Scanner, logger *slog.Logger, tracer object.Tracer, scanPolicy object.ScanPolicyFunc) *Evaluator {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return &Evaluator{
		scanner:           scanner,
		intrinsics:        intrinsics.New(),
		logger:            logger,
		tracer:            tracer,
		interfaceBindings: make(map[string]*goscan.TypeInfo),
		scanPolicy:        scanPolicy,
		initializedPkgs:   make(map[string]bool),
		pkgCache:          make(map[string]*object.Package),
	}
}
```

#### 2.1.3. Add the `getOrLoadPackage` Helper Method

A new method was added to `Evaluator` that centralizes the creation and caching of package objects. This function is now the single source of truth for `*object.Package`.

```go
// symgo/evaluator/evaluator.go

func (e *Evaluator) getOrLoadPackage(ctx context.Context, path string) (*object.Package, error) {
	if pkg, ok := e.pkgCache[path]; ok {
		// Ensure even cached packages are populated if they were created as placeholders first.
		e.ensurePackageEnvPopulated(ctx, pkg)
		return pkg, nil
	}

	scannedPkg, err := e.scanner.ScanPackageByImport(ctx, path)
	if err != nil {
		// Even if scanning fails, we create a placeholder package object to cache the failure
		// and avoid re-scanning. The ScannedInfo will be nil.
		pkgObj := &object.Package{
			Name:        "", // We don't know the name
			Path:        path,
			Env:         object.NewEnvironment(),
			ScannedInfo: nil,
		}
		e.pkgCache[path] = pkgObj
		return nil, fmt.Errorf("could not scan package %q: %w", path, err)
	}

	pkgObj := &object.Package{
		Name:        scannedPkg.Name,
		Path:        scannedPkg.ImportPath,
		Env:         object.NewEnvironment(),
		ScannedInfo: scannedPkg,
	}

	e.ensurePackageEnvPopulated(ctx, pkgObj)
	e.pkgCache[path] = pkgObj
	return pkgObj, nil
}
```

#### 2.1.4. Remove `findPackageByPath`

The `findPackageByPath` function became redundant and was removed completely.

#### 2.1.5. Refactor `evalIdent`

`evalIdent` was updated to use the robust import resolution logic from the original implementation, adapted to use the new `getOrLoadPackage` helper. This correctly handles package name aliases and cases where the package name differs from the import path.

```go
// symgo/evaluator/evaluator.go

func (e *Evaluator) evalIdent(ctx context.Context, n *ast.Ident, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	if pkg != nil {
		key := pkg.ImportPath + "." + n.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			e.logger.Debug("evalIdent: found intrinsic, overriding", "key", key)
			return &object.Intrinsic{Fn: intrinsicFn}
		}
	}

	if val, ok := env.Get(n.Name); ok {
		e.logger.Debug("evalIdent: found in env", "name", n.Name, "type", val.Type())
		if v, ok := val.(*object.Variable); ok {
			value := v.Value
			if value.TypeInfo() == nil && v.TypeInfo() != nil {
				value.SetTypeInfo(v.TypeInfo())
			}
			return value
		}
		return val
	}

	// If the identifier is not in the environment, it might be a package name.
	if pkg != nil && pkg.Fset != nil {
		file := pkg.Fset.File(n.Pos())
		if file != nil {
			if astFile, ok := pkg.AstFiles[file.Name()]; ok {
				for _, imp := range astFile.Imports {
					importPath, _ := strconv.Unquote(imp.Path.Value)

					// Case 1: The import has an alias.
					if imp.Name != nil {
						if n.Name == imp.Name.Name {
							pkgObj, _ := e.getOrLoadPackage(ctx, importPath)
							if pkgObj != nil {
								env.Set(n.Name, pkgObj)
							}
							return pkgObj
						}
						continue
					}

					// Case 2: No alias. The identifier might be the package's actual name.
					pkgObj, err := e.getOrLoadPackage(ctx, importPath)
					if err != nil || pkgObj == nil || pkgObj.ScannedInfo == nil {
						e.logWithContext(ctx, slog.LevelDebug, "could not scan potential package for ident", "ident", n.Name, "path", importPath, "error", err)
						continue
					}

					if n.Name == pkgObj.ScannedInfo.Name {
						env.Set(n.Name, pkgObj)
						return pkgObj
					}
				}
			}
		}
	}

	// Fallback to universe scope for built-in values, types, and functions.
	if val, ok := universe.GetValue(n.Name); ok {
		return val
	}
	if typ, ok := universe.GetType(n.Name); ok {
		return typ
	}
	if fn, ok := universe.GetFunction(n.Name); ok {
		return &object.Intrinsic{Fn: fn}
	}

	e.logger.Debug("evalIdent: not found in env or intrinsics", "name", n.Name)

	if pkg != nil && e.scanPolicy != nil && !e.scanPolicy(pkg.ImportPath) {
		e.logger.DebugContext(ctx, "treating undefined identifier as symbolic in out-of-policy package", "ident", n.Name, "package", pkg.ImportPath)
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("undefined identifier %s in out-of-policy package", n.Name)}
	}

	return e.newError(n.Pos(), "identifier not found: %s", n.Name)
}
```

#### 2.1.6. Refactor `findDirectMethodOnType`

`findDirectMethodOnType` was updated to use `getOrLoadPackage` and the incorrect pointer-receiver check was removed.

```go
// symgo/evaluator/evaluator.go

func (e *Evaluator) findDirectMethodOnType(ctx context.Context, typeInfo *scanner.TypeInfo, methodName string, env *object.Environment, receiver object.Object) (*object.Function, error) {
	if typeInfo == nil || typeInfo.PkgPath == "" {
		return nil, nil
	}

	if e.scanPolicy != nil && !e.scanPolicy(typeInfo.PkgPath) {
		return nil, nil
	}

	pkgObj, err := e.getOrLoadPackage(ctx, typeInfo.PkgPath)
	if err != nil || pkgObj.ScannedInfo == nil {
		if err != nil && strings.Contains(err.Error(), "cannot find package") {
			return nil, nil
		}
		e.logWithContext(ctx, slog.LevelWarn, "could not get or load package for method resolution", "package", typeInfo.PkgPath, "error", err)
		return nil, nil
	}
	methodPkg := pkgObj.ScannedInfo

	for _, fn := range methodPkg.Functions {
		if fn.Receiver == nil || fn.Name != methodName {
			continue
		}

		recvTypeName := fn.Receiver.Type.TypeName
		if recvTypeName == "" {
			recvTypeName = fn.Receiver.Type.Name
		}
		baseRecvTypeName := strings.TrimPrefix(recvTypeName, "*")
		baseTypeName := strings.TrimPrefix(typeInfo.Name, "*")

		if baseRecvTypeName == baseTypeName {
			return &object.Function{
				Name:       fn.AstDecl.Name,
				Parameters: fn.AstDecl.Type.Params,
				Body:       fn.AstDecl.Body,
				Env:        pkgObj.Env, // Use the canonical environment from the cached package object.
				Decl:       fn.AstDecl,
				Package:    methodPkg,
				Receiver:   receiver,
				Def:        fn,
			}, nil
		}
	}

	return nil, nil // Not found
}
```

## 3. Respecting the Scan Policy

A critical aspect of the fix was to correctly handle packages that are outside the defined `scanPolicy`. For these packages, the evaluator should not have access to internal details like unexported constants.

The `getOrLoadPackage` function still uses `ScanPackageByImport` to get package metadata (like the correct package name), but the subsequent population of the environment respects the policy.

### Step 3.1: Modify `ensurePackageEnvPopulated`

The `ensurePackageEnvPopulated` function in `symgo/evaluator/evaluator.go` was modified to check the `scanPolicy` before adding unexported symbols to a package's environment.

```go
// symgo/evaluator/evaluator.go

func (e *Evaluator) ensurePackageEnvPopulated(ctx context.Context, pkgObj *object.Package) {
	if pkgObj.ScannedInfo == nil {
		return // Not scanned yet.
	}
	pkgInfo := pkgObj.ScannedInfo
	env := pkgObj.Env

	shouldScan := e.scanPolicy == nil || e.scanPolicy(pkgInfo.ImportPath)

	if !e.initializedPkgs[pkgInfo.ImportPath] {
		e.logger.DebugContext(ctx, "populating package-level constants", "package", pkgInfo.ImportPath)
		for _, c := range pkgInfo.Constants {
			// If the package is out-of-policy, only load exported constants.
			if !shouldScan && !c.IsExported {
				continue
			}
			constObj := e.convertGoConstant(c.ConstVal, token.NoPos)
			if isError(constObj) {
				e.logger.Warn("could not convert constant to object", "const", c.Name, "error", constObj)
				continue
			}
			env.Set(c.Name, constObj)
		}
		e.initializedPkgs[pkgInfo.ImportPath] = true
	}

	if len(pkgInfo.Functions) > 0 {
		if _, ok := env.Get(pkgInfo.Functions[0].Name); ok {
			return
		}
	} else if len(pkgInfo.Constants) == 0 {
		return
	}

	e.logger.DebugContext(ctx, "populating package environment", "package", pkgInfo.ImportPath)

	for _, f := range pkgInfo.Functions {
		if _, ok := env.Get(f.Name); ok {
			continue
		}
		
		// If the package is out-of-policy, only create objects for exported functions.
		if !shouldScan && !ast.IsExported(f.Name) {
			continue
		}

		var fnObject object.Object
		if shouldScan {
			fnObject = &object.Function{
				Name:       f.AstDecl.Name,
				Parameters: f.AstDecl.Type.Params,
				Body:       f.AstDecl.Body,
				Env:        env,
				Decl:       f.AstDecl,
				Package:    pkgInfo,
				Def:        f,
			}
		} else {
			// For out-of-policy packages, exported functions become placeholders.
			fnObject = &object.SymbolicPlaceholder{
				Reason:         fmt.Sprintf("external function %s.%s", pkgInfo.ImportPath, f.Name),
				UnderlyingFunc: f,
				Package:        pkgInfo,
			}
		}
		env.SetLocal(f.Name, fnObject)
	}
}

```

### Step 3.2: Add Test Cases to `symgo/symgo_unexported_const_test.go`

Two test cases were appended to this file to verify the fix and prevent regressions.

```go
// Test case for nested function call
func TestSymgo_UnexportedConstantResolution_NestedCall(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"loglib/go.mod": `
module example.com/loglib
go 1.21
replace example.com/driver => ../driver
`,
		"loglib/context.go": `
package loglib
import "example.com/driver"
func FuncA() string {
	return driver.FuncB()
}
`,
		"driver/go.mod": `
module example.com/driver
go 1.21
`,
		"driver/db.go": `
package driver
const privateConst = "hello from private"
func FuncB() string {
	return privateConst
}
`,
	})
	defer cleanup()
	loglibModuleDir := filepath.Join(tmpdir, "loglib")
	goscanner, err := goscan.New(
		goscan.WithWorkDir(loglibModuleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}
	policy := func(importPath string) bool {
		return strings.HasPrefix(importPath, "example.com/loglib") || strings.HasPrefix(importPath, "example.com/driver")
	}
	interp, err := symgo.NewInterpreter(goscanner, symgo.WithScanPolicy(policy))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}
	loglibPkg, err := goscanner.ScanPackage(ctx, loglibModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}
	loglibFile := findFile(t, loglibPkg, "context.go")
	if _, err := interp.Eval(ctx, loglibFile, loglibPkg); err != nil {
		t.Fatalf("Eval loglib file failed: %v", err)
	}
	entrypointObj, ok := interp.FindObject("FuncA")
	if !ok {
		t.Fatal("FuncA function not found in interpreter environment")
	}
	entrypointFunc, ok := entrypointObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'FuncA' is not a function, but %T", entrypointObj)
	}
	result, err := interp.Apply(ctx, entrypointFunc, nil, loglibPkg)
	if err != nil {
		t.Fatalf("Apply FuncA function failed: %v", err)
	}
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}
	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}
	expected := "hello from private"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}

// Test case for nested method call
func TestSymgo_UnexportedConstantResolution_NestedMethodCall(t *testing.T) {
	ctx := context.Background()
	tmpdir, cleanup := scantest.WriteFiles(t, map[string]string{
		"main/go.mod": `
module example.com/main
go 1.21
replace example.com/remotedb => ../remotedb
`,
		"main/main.go": `
package main
import "example.com/remotedb"
func DoWork() string {
	var client remotedb.Client
	return client.GetValue()
}
`,
		"remotedb/go.mod": `
module example.com/remotedb
go 1.21
`,
		"remotedb/db.go": `
package remotedb
const secretKey = "unexported-secret-key"
type Client struct{}
func (c *Client) GetValue() string {
	return secretKey
}
`,
	})
	defer cleanup()
	mainModuleDir := filepath.Join(tmpdir, "main")
	goscanner, err := goscan.New(
		goscan.WithWorkDir(mainModuleDir),
		goscan.WithGoModuleResolver(),
	)
	if err != nil {
		t.Fatalf("New scanner failed: %v", err)
	}
	policy := func(importPath string) bool {
		return strings.HasPrefix(importPath, "example.com/main") || strings.HasPrefix(importPath, "example.com/remotedb")
	}
	interp, err := symgo.NewInterpreter(goscanner, symgo.WithScanPolicy(policy))
	if err != nil {
		t.Fatalf("NewInterpreter failed: %v", err)
	}
	mainPkg, err := goscanner.ScanPackage(ctx, mainModuleDir)
	if err != nil {
		t.Fatalf("ScanPackage failed: %v", err)
	}
	mainFile := findFile(t, mainPkg, "main.go")
	if _, err := interp.Eval(ctx, mainFile, mainPkg); err != nil {
		t.Fatalf("Eval main file failed: %v", err)
	}
	entrypointObj, ok := interp.FindObject("DoWork")
	if !ok {
		t.Fatal("DoWork function not found in interpreter environment")
	}
	entrypointFunc, ok := entrypointObj.(*symgo.Function)
	if !ok {
		t.Fatalf("entrypoint 'DoWork' is not a function, but %T", entrypointObj)
	}
	result, err := interp.Apply(ctx, entrypointFunc, nil, mainPkg)
	if err != nil {
		t.Fatalf("Apply DoWork function failed: %v", err)
	}
	retVal, ok := result.(*object.ReturnValue)
	if !ok {
		t.Fatalf("expected result to be a *object.ReturnValue, but got %T", result)
	}
	str, ok := retVal.Value.(*object.String)
	if !ok {
		t.Fatalf("expected return value to be *object.String, but got %T", retVal.Value)
	}
	expected := "unexported-secret-key"
	if str.Value != expected {
		t.Errorf("expected result to be %q, but got %q", expected, str.Value)
	}
}
```
