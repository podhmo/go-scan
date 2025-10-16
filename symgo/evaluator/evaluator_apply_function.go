package evaluator

import (
	"context"
	"fmt"
	"go/token"
	"log/slog"
	"strings"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) applyFunction(ctx context.Context, fn object.Object, args []object.Object, pkg *scan.PackageInfo, callPos token.Pos) object.Object {
	if f, ok := fn.(*object.Function); ok {
		if e.memoize && f.Decl != nil {
			if cachedResult, found := e.memoizationCache[f.Decl.Pos()]; found {
				if symbolName, pkgPath, ok := e.getSymbolInfoForLog(fn); ok {
					if e.resolver.ScanPolicy(pkgPath) {
						slog.InfoContext(ctx, fmt.Sprintf("**#%s apply function", strings.Repeat("#", len(e.callStack))),
							"depth", len(e.callStack),
							"symbol", symbolName,
							"pkg", pkgPath,
							"pos", callPos,
						)
					}
				}
				e.logc(ctx, slog.LevelDebug, "returning memoized result for function", "function", f.Name)
				return cachedResult
			}
		}
	}

	if symbolName, pkgPath, ok := e.getSymbolInfoForLog(fn); ok {
		if e.resolver.ScanPolicy(pkgPath) {
			slog.InfoContext(ctx, fmt.Sprintf("***%s apply function", strings.Repeat("*", len(e.callStack))),
				"depth", len(e.callStack),
				"symbol", symbolName,
				"pkg", pkgPath,
				"pos", callPos,
			)
		}
	}

	result := e.applyFunctionImpl(ctx, fn, args, pkg, callPos)

	if f, ok := fn.(*object.Function); ok {
		if e.memoize && !isError(result) && f.Decl != nil {
			e.logc(ctx, slog.LevelDebug, "caching result for function", "function", f.Name)
			e.memoizationCache[f.Decl.Pos()] = result
		}
	}

	return result
}

func (e *Evaluator) applyFunctionImpl(ctx context.Context, fn object.Object, args []object.Object, pkg *scan.PackageInfo, callPos token.Pos) object.Object {
	var name string

	if f, ok := fn.(*object.Function); ok {
		if f.Name != nil {
			name = f.Name.Name
		} else {
			name = "<closure>"
		}
	} else if v, ok := fn.(*object.Variable); ok {
		name = v.Name
	} else {
		name = fn.Inspect()
	}

	if len(e.callStack) >= MaxCallStackDepth {
		e.logc(ctx, slog.LevelWarn, "call stack depth exceeded, aborting recursion", "function", name)
		return &object.SymbolicPlaceholder{Reason: "max call stack depth exceeded"}
	}

	// New recursion check based on function definition (for named functions)
	// or function literal position (for anonymous functions).
	if f, ok := fn.(*object.Function); ok {
		// Determine which call stack to use for recursion detection.
		// If the function has a BoundCallStack, it means it was passed as an argument,
		// and that stack represents the true logical path leading to this call.
		stackToScan := e.callStack
		if f.BoundCallStack != nil {
			stackToScan = f.BoundCallStack
		}

		recursionCount := 0
		for _, frame := range stackToScan {
			if frame.Fn == nil {
				continue
			}

			// Case 1: Named function with a definition. Compare declaration positions.
			if f.Def != nil && f.Def.AstDecl != nil && frame.Fn.Def != nil && frame.Fn.Def.AstDecl != nil {
				if f.Def.AstDecl.Pos() == frame.Fn.Def.AstDecl.Pos() {
					recursionCount++
				}
				continue
			}

			// Case 2: Anonymous function (function literal). Compare literal positions.
			if f.Lit != nil && frame.Fn.Lit != nil {
				if f.Lit.Pos() == frame.Fn.Lit.Pos() {
					recursionCount++
				}
			}
		}

		// Allow one level of recursion, but stop at the second call.
		if recursionCount > 0 { // Changed from > 1 to > 0 to be more strict.
			e.logc(ctx, slog.LevelDebug, "bounded recursion depth exceeded, halting analysis for this path", "function", name)
			// Return a symbolic placeholder that matches the function's return signature.
			if f.Def != nil && f.Def.AstDecl.Type.Results != nil {
				numResults := len(f.Def.AstDecl.Type.Results.List)
				if numResults > 1 {
					results := make([]object.Object, numResults)
					for i := 0; i < numResults; i++ {
						results[i] = &object.SymbolicPlaceholder{Reason: "bounded recursion halt"}
					}
					return &object.MultiReturn{Values: results}
				}
			}
			// Default to a single placeholder if signature is not available or has <= 1 return values.
			return &object.SymbolicPlaceholder{Reason: "bounded recursion halt"}
		}
	}

	frame := &object.CallFrame{Function: name, Pos: callPos, Args: args}
	if f, ok := fn.(*object.Function); ok {
		frame.Fn = f
		if f.Receiver != nil {
			frame.ReceiverPos = f.ReceiverPos
		}
	}
	e.callStack = append(e.callStack, frame)
	defer func() {
		e.callStack = e.callStack[:len(e.callStack)-1]
	}()

	if e.logger.Enabled(ctx, slog.LevelDebug) {
		argStrs := make([]string, len(args))
		for i, arg := range args {
			argStrs[i] = arg.Inspect()
		}
		e.logc(ctx, slog.LevelDebug, "applyFunction", "in_func", name, "in_func_pos", e.scanner.Fset().Position(callPos), "exec_pos", callPos, "type", fn.Type(), "value", inspectValuer{fn}, "args", strings.Join(argStrs, ", "))
	}

	// If `fn` is a variable, we need to evaluate it to get the underlying function.
	if v, ok := fn.(*object.Variable); ok {
		underlyingFn := e.forceEval(ctx, v, pkg) // Use forceEval to handle chained variables
		if isError(underlyingFn) {
			return underlyingFn
		}
		// Recursively call applyFunction with the resolved function object.
		return e.applyFunction(ctx, underlyingFn, args, pkg, callPos)
	}

	switch fn := fn.(type) {
	case *object.AmbiguousSelector:
		// If applyFunction encounters an ambiguous selector, it means the expression
		// is being used in a call context `expr()`. We resolve the ambiguity
		// by assuming it's a method call.
		var typeName string
		if typeInfo := fn.Receiver.TypeInfo(); typeInfo != nil {
			typeName = typeInfo.Name
		}
		e.logc(ctx, slog.LevelWarn, "assuming method exists on unresolved embedded type", "method_name", fn.Sel.Name, "type_name", typeName)
		placeholder := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed method %s on type with unresolved embedded part", fn.Sel.Name)}
		// The placeholder is now the function, so we recursively call applyFunction with it.
		return e.applyFunction(ctx, placeholder, args, pkg, callPos)
	case *object.InstantiatedFunction:
		// When applying an instantiated generic function, we need to create a new
		// environment for the call. This environment must:
		// 1. Be enclosed by the environment where the generic function was defined (fn.Function.Env).
		// 2. Contain the bindings for the type parameters (e.g., T -> int).
		// 3. Contain the bindings for the regular function arguments.

		// Create a new environment for the call, enclosing the function's definition environment.
		callEnv := object.NewEnclosedEnvironment(fn.Function.Env)

		// Bind type parameters to their concrete types in this new call environment.
		if fn.TypeParamMap != nil {
			for name, typeInfo := range fn.TypeParamMap {
				if typeInfo != nil {
					typeObj := &object.Type{
						TypeName:     typeInfo.Name,
						ResolvedType: typeInfo,
					}
					typeObj.SetTypeInfo(typeInfo)
					callEnv.SetLocal(name, typeObj)
				}
			}
		}

		// Now, extend this environment with the regular function arguments.
		// We pass `callEnv` as the base environment to `extendFunctionEnv`.
		finalEnv, err := e.extendFunctionEnv(ctx, fn.Function, args, callEnv)
		if err != nil {
			return e.newError(ctx, fn.Function.Decl.Pos(), "failed to extend generic function env: %v", err)
		}

		// Evaluate the function body within the fully prepared environment.
		evaluated := e.Eval(ctx, fn.Function.Body, finalEnv, fn.Function.Package)

		if ret, ok := evaluated.(*object.ReturnValue); ok {
			return ret.Value
		}
		if evaluated == nil {
			return object.NIL
		}
		return evaluated

	case *object.Function:
		// If the function has no body, it's a declaration (e.g., in an interface, or an external function).
		// Treat it as an external call and create a symbolic result based on its signature.
		if fn.Body == nil {
			return e.createSymbolicResultForFunc(ctx, fn)
		}

		// When calling a function, ensure its defining package's environment is fully populated.
		if fn.Package != nil {
			pkgObj, err := e.getOrLoadPackage(ctx, fn.Package.ImportPath)
			if err == nil {
				e.ensurePackageEnvPopulated(ctx, pkgObj)
			}
		}

		// Check the scan policy before executing the body.
		if fn.Package != nil && !e.resolver.ScanPolicy(fn.Package.ImportPath) {
			// If the package is not in the primary analysis scope, treat the call
			// as symbolic, just like an external function call.
			return e.createSymbolicResultForFunc(ctx, fn)
		}

		// When applying a function, the evaluation context switches to that function's
		// package. We must pass fn.Package to both extendFunctionEnv and Eval.
		extendedEnv, err := e.extendFunctionEnv(ctx, fn, args, nil) // Pass nil for non-generic calls
		if err != nil {
			return e.newError(ctx, fn.Decl.Pos(), "failed to extend function env: %v", err)
		}

		// Populate the new environment with the imports from the function's source file.
		if fn.Package != nil && fn.Package.Fset != nil && fn.Decl != nil {
			file := fn.Package.Fset.File(fn.Decl.Pos())
			if file != nil {
				if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
					for _, imp := range astFile.Imports {
						var name string
						if imp.Name != nil {
							name = imp.Name.Name
						} else {
							parts := strings.Split(strings.Trim(imp.Path.Value, `"`), "/")
							name = parts[len(parts)-1]
						}
						path := strings.Trim(imp.Path.Value, `"`)
						// Set ScannedInfo to nil to force on-demand loading.
						extendedEnv.Set(name, &object.Package{Path: path, ScannedInfo: nil, Env: object.NewEnclosedEnvironment(e.UniverseEnv)})
					}
				}
			}
		}

		evaluated := e.Eval(ctx, fn.Body, extendedEnv, fn.Package)
		if evaluated != nil {
			if isError(evaluated) || evaluated.Type() == object.PANIC_OBJ {
				return evaluated
			}
		}

		evaluatedValue := evaluated
		if ret, ok := evaluated.(*object.ReturnValue); ok {
			evaluatedValue = ret.Value
		}

		// If the evaluated result is a Go nil (from a naked return), wrap it.
		if evaluatedValue == nil {
			return &object.ReturnValue{Value: object.NIL}
		}

		return &object.ReturnValue{Value: evaluatedValue}

	case *object.Intrinsic:
		return fn.Fn(ctx, args...)

	case *object.SymbolicPlaceholder:
		// This now handles both external function calls and interface method calls.
		if fn.UnderlyingFunc != nil {
			// If it has an AST declaration, it's a real function from source.
			if fn.UnderlyingFunc.AstDecl != nil {
				return e.createSymbolicResultForFuncInfo(ctx, fn.UnderlyingFunc, fn.Package, "result of external call to %s", fn.UnderlyingFunc.Name)
			}

			// Otherwise, it's a constructed FunctionInfo for an interface method.
			// We create the result based on the Parameters/Results fields directly.
			method := fn.UnderlyingFunc
			var result object.Object
			if len(method.Results) <= 1 {
				var resultTypeInfo *scan.TypeInfo
				var resultFieldType *scan.FieldType
				if len(method.Results) == 1 {
					resultFieldType = method.Results[0].Type
					if resultFieldType != nil {
						resultType := e.resolver.ResolveType(ctx, resultFieldType)
						if resultType == nil && resultFieldType.IsBuiltin {
							resultType = &scan.TypeInfo{Name: resultFieldType.Name}
						}
						resultTypeInfo = resultType
					}
				}
				result = &object.SymbolicPlaceholder{
					Reason:     fmt.Sprintf("result of interface method call %s", method.Name),
					BaseObject: object.BaseObject{ResolvedTypeInfo: resultTypeInfo, ResolvedFieldType: resultFieldType},
				}
			} else {
				// Multiple return values from interface method
				results := make([]object.Object, len(method.Results))
				for i, resFieldInfo := range method.Results {
					resultFieldType := resFieldInfo.Type
					var resultType *scan.TypeInfo
					if resultFieldType != nil {
						resultType = e.resolver.ResolveType(ctx, resultFieldType)
						if resultType == nil && resultFieldType.IsBuiltin && resultFieldType.Name == "error" {
							resultType = ErrorInterfaceTypeInfo
						}
					}
					results[i] = &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("result %d of interface method call %s", i, method.Name),
						BaseObject: object.BaseObject{ResolvedTypeInfo: resultType, ResolvedFieldType: resultFieldType},
					}
				}
				result = &object.MultiReturn{Values: results}
			}
			return &object.ReturnValue{Value: result}
		}

		// Case 3: A placeholder representing a callable variable (like flag.Usage)
		if typeInfo := fn.TypeInfo(); typeInfo != nil && typeInfo.Kind == scan.FuncKind && typeInfo.Func != nil {
			funcInfo := typeInfo.Func
			var pkgInfo *scan.PackageInfo
			var err error
			if fn.FieldType() != nil && fn.FieldType().FullImportPath != "" {
				pkg, loadErr := e.getOrLoadPackage(ctx, fn.FieldType().FullImportPath)
				if loadErr == nil && pkg != nil {
					pkgInfo = pkg.ScannedInfo
				}
				err = loadErr
			}
			if pkgInfo == nil {
				e.logc(ctx, slog.LevelWarn, "could not load package for function variable type", "path", typeInfo.PkgPath, "error", err)
				return &object.ReturnValue{Value: &object.SymbolicPlaceholder{Reason: "result of calling function variable with unloadable type"}}
			}
			return e.createSymbolicResultForFuncInfo(ctx, funcInfo, pkgInfo, "result of call to var %s", fn.Reason)
		}

		// Case 4: A placeholder representing a built-in type, used in a conversion.
		if strings.HasPrefix(fn.Reason, "built-in type") {
			result := &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of conversion to %s", fn.Reason)}
			return &object.ReturnValue{Value: result}
		}
		// Fallback for any other kind of placeholder is to treat it as a symbolic call.
		result := &object.SymbolicPlaceholder{Reason: "result of calling " + fn.Inspect()}
		return &object.ReturnValue{Value: result}

	case *object.UnresolvedType:
		// This is a symbol from an out-of-policy package, which we are now attempting to call.
		// We treat it as a function call, mirroring the logic for UnresolvedFunction.
		e.logc(ctx, slog.LevelDebug, "applying unresolved type as function", "package", fn.PkgPath, "function", fn.TypeName)

		key := fn.PkgPath + "." + fn.TypeName
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return intrinsicFn(ctx, args...)
		}

		scannedPkg, err := e.resolver.ResolvePackage(ctx, fn.PkgPath)
		if err != nil {
			e.logc(ctx, slog.LevelDebug, "could not scan package for unresolved symbol (or denied by policy)", "package", fn.PkgPath, "symbol", fn.TypeName, "error", err)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved symbol %s.%s", fn.PkgPath, fn.TypeName)}
		}

		var funcInfo *scan.FunctionInfo
		for _, f := range scannedPkg.Functions {
			if f.Name == fn.TypeName {
				funcInfo = f
				break
			}
		}

		if funcInfo == nil {
			e.logc(ctx, slog.LevelWarn, "could not find function signature in package for symbol", "package", fn.PkgPath, "symbol", fn.TypeName)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved symbol %s.%s", fn.PkgPath, fn.TypeName)}
		}
		return e.createSymbolicResultForFuncInfo(ctx, funcInfo, scannedPkg, "result of call to %s.%s", fn.PkgPath, fn.TypeName)

	case *object.UnresolvedFunction:
		// This is a function that could not be resolved during symbol lookup.
		// We make a best effort to find its signature now.
		e.logc(ctx, slog.LevelDebug, "attempting to resolve and apply unresolved function", "package", fn.PkgPath, "function", fn.FuncName)

		// Before trying to scan the package, check if there's a registered intrinsic for it.
		key := fn.PkgPath + "." + fn.FuncName
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return intrinsicFn(ctx, args...)
		}

		// Use the policy-enforcing method to resolve the package.
		scannedPkg, err := e.resolver.ResolvePackage(ctx, fn.PkgPath)
		if err != nil {
			e.logc(ctx, slog.LevelInfo, "could not scan package for unresolved function (or denied by policy)", "package", fn.PkgPath, "function", fn.FuncName, "error", err)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved function %s.%s", fn.PkgPath, fn.FuncName)}
		}

		var funcInfo *scan.FunctionInfo
		for _, f := range scannedPkg.Functions {
			if f.Name == fn.FuncName {
				funcInfo = f
				break
			}
		}

		if funcInfo == nil {
			e.logc(ctx, slog.LevelWarn, "could not find function signature in package", "package", fn.PkgPath, "function", fn.FuncName)
			return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of calling unresolved function %s.%s", fn.PkgPath, fn.FuncName)}
		}

		// We found the function info. Now create a symbolic result based on its signature.
		return e.createSymbolicResultForFuncInfo(ctx, funcInfo, scannedPkg, "result of call to %s.%s", fn.PkgPath, fn.FuncName)

	case *object.Type:
		// This handles type conversions like string(b) or int(x).
		if len(args) != 1 {
			return e.newError(ctx, callPos, "wrong number of arguments for type conversion: got=%d, want=1", len(args))
		}
		// The result is a symbolic value of the target type.
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("result of conversion to %s", fn.TypeName),
		}
		placeholder.SetTypeInfo(fn.ResolvedType)
		return &object.ReturnValue{Value: placeholder}

	default:
		return e.newError(ctx, callPos, "not a function: %s", fn.Type())
	}
}

// createSymbolicResultForFuncInfo creates a symbolic result for a function call based on its FunctionInfo.
// This is used for functions that are not deeply executed (e.g., due to scan policy or being unresolved).
func (e *Evaluator) createSymbolicResultForFuncInfo(ctx context.Context, funcInfo *scan.FunctionInfo, pkgInfo *scan.PackageInfo, reasonFormat string, reasonArgs ...any) object.Object {
	if funcInfo.AstDecl == nil || funcInfo.AstDecl.Type == nil || pkgInfo == nil {
		return &object.SymbolicPlaceholder{Reason: "result of call with incomplete info"}
	}
	reason := fmt.Sprintf(reasonFormat, reasonArgs...)

	results := funcInfo.AstDecl.Type.Results
	if results == nil || len(results.List) == 0 {
		return &object.SymbolicPlaceholder{Reason: reason + " (no return value)"}
	}

	var importLookup map[string]string
	if file := pkgInfo.Fset.File(funcInfo.AstDecl.Pos()); file != nil {
		if astFile, ok := pkgInfo.AstFiles[file.Name()]; ok {
			importLookup = e.scanner.BuildImportLookup(astFile)
		}
	}

	if len(results.List) == 1 {
		resultASTExpr := results.List[0].Type
		fieldType := e.scanner.TypeInfoFromExpr(ctx, resultASTExpr, nil, pkgInfo, importLookup)
		resolvedType := e.resolver.ResolveType(ctx, fieldType)

		// Special handling for the built-in error interface.
		if resolvedType == nil && fieldType.IsBuiltin && fieldType.Name == "error" {
			resolvedType = ErrorInterfaceTypeInfo
		}

		return &object.ReturnValue{
			Value: &object.SymbolicPlaceholder{
				Reason:     reason,
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			},
		}
	}

	// Multiple return values
	returnValues := make([]object.Object, 0, len(results.List))
	for i, field := range results.List {
		fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, pkgInfo, importLookup)
		resolvedType := e.resolver.ResolveType(ctx, fieldType)

		// Special handling for the built-in error interface.
		if resolvedType == nil && fieldType.IsBuiltin && fieldType.Name == "error" {
			resolvedType = ErrorInterfaceTypeInfo
		}

		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("%s (result %d)", reason, i),
			BaseObject: object.BaseObject{
				ResolvedTypeInfo:  resolvedType,
				ResolvedFieldType: fieldType,
			},
		}
		returnValues = append(returnValues, placeholder)
	}
	return &object.ReturnValue{Value: &object.MultiReturn{Values: returnValues}}
}

// createSymbolicResultForFunc creates a symbolic result for a function call
// that is not being deeply executed due to scan policy.
func (e *Evaluator) createSymbolicResultForFunc(ctx context.Context, fn *object.Function) object.Object {
	if fn.Def == nil {
		return &object.SymbolicPlaceholder{Reason: "result of external call with incomplete info"}
	}
	return e.createSymbolicResultForFuncInfo(ctx, fn.Def, fn.Package, "result of out-of-policy call to %s", fn.Name.Name)
}
