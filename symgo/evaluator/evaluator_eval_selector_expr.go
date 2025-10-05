package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalSelectorExpr(ctx context.Context, n *ast.SelectorExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	e.logger.Debug("evalSelectorExpr", "selector", n.Sel.Name)

	// New, more robust check for interface method calls.
	// Instead of relying on the scanner's static analysis of the expression, we look up
	// the variable in our own environment and check the static type info we have stored on it.
	if ident, ok := n.X.(*ast.Ident); ok {
		if obj, found := env.Get(ident.Name); found {
			var staticType *scan.TypeInfo
			if v, isVar := obj.(*object.Variable); isVar {
				if ft := v.FieldType(); ft != nil {
					resolved, err := ft.Resolve(ctx)
					if err == nil && resolved != nil {
						staticType = resolved
					}
				} else if ti := v.TypeInfo(); ti != nil {
					staticType = ti
				}
			} else if sp, isSym := obj.(*object.SymbolicPlaceholder); isSym {
				if ti := sp.TypeInfo(); ti != nil {
					staticType = ti
				}
			}

			if staticType != nil && staticType.Kind == scan.InterfaceKind {
				// Handle union-type interfaces.
				if staticType.Interface != nil && len(staticType.Interface.Union) > 0 {
					e.logc(ctx, slog.LevelDebug, "evalSelectorExpr: detected union interface method call", "interface", staticType.Name, "method", n.Sel.Name)

					// Iterate over all types in the union.
					for _, memberFieldType := range staticType.Interface.Union {
						// The receiver for the method call is the original object `obj` which holds the interface type.
						// However, to find the method, we need to resolve the concrete type from the union.
						memberTypeInfo, err := memberFieldType.Resolve(ctx)
						if err != nil || memberTypeInfo == nil {
							e.logc(ctx, slog.LevelWarn, "could not resolve union member type", "member", memberFieldType.String(), "error", err)
							continue
						}

						// Create a symbolic receiver representing an instance of this specific member type.
						// This is crucial for `findMethodOnType` to correctly bind the receiver type information,
						// which is then inspected by the test's intrinsic.
						symbolicReceiver := &object.SymbolicPlaceholder{
							Reason: fmt.Sprintf("symbolic instance of union member %s", memberTypeInfo.Name),
						}
						symbolicReceiver.SetTypeInfo(memberTypeInfo)
						symbolicReceiver.SetFieldType(memberFieldType)

						// Find the method on this concrete type. The receiver passed here (symbolicReceiver)
						// is bound to the resulting function object.
						method, err := e.accessor.findMethodOnType(ctx, memberTypeInfo, n.Sel.Name, env, symbolicReceiver, n.X.Pos())
						if err != nil {
							e.logc(ctx, slog.LevelDebug, "method not found on union member", "member", memberTypeInfo.Name, "method", n.Sel.Name, "error", err)
							continue
						}

						// If a method is found, "call" the default intrinsic with the concrete function object
						// to mark it as used by analysis tools.
						if method != nil && e.defaultIntrinsic != nil {
							e.logc(ctx, slog.LevelDebug, "evalSelectorExpr: marking concrete method from union as used", "method", method.Inspect())
							e.defaultIntrinsic(ctx, method)
						}
					}

					// After processing all members, return a symbolic placeholder for the result of the call.
					// We can try to find the method on the interface definition to get a signature for the placeholder.
					var methodInfo *scan.MethodInfo
					if staticType.Interface != nil {
						allMethods := e.getAllInterfaceMethods(ctx, staticType, make(map[string]struct{}))
						for _, m := range allMethods {
							if m.Name == n.Sel.Name {
								methodInfo = m
								break
							}
						}
					}
					if methodInfo != nil {
						methodFuncInfo := &scan.FunctionInfo{
							Name:       methodInfo.Name,
							Parameters: methodInfo.Parameters,
							Results:    methodInfo.Results,
						}
						return &object.SymbolicPlaceholder{
							Reason:         fmt.Sprintf("union interface method call %s.%s", staticType.Name, n.Sel.Name),
							Receiver:       obj,
							UnderlyingFunc: methodFuncInfo,
							Package:        pkg,
						}
					}
					// Fallback placeholder if we can't find the method signature in the interface itself.
					return &object.SymbolicPlaceholder{
						Reason:   fmt.Sprintf("result of call to union interface method %s", n.Sel.Name),
						Receiver: obj,
					}
				}

				// Fallthrough to handle regular (non-union) interfaces.

				// Check for a registered intrinsic for this interface method call.
				key := fmt.Sprintf("(%s.%s).%s", staticType.PkgPath, staticType.Name, n.Sel.Name)
				if intrinsicFn, ok := e.intrinsics.Get(key); ok {
					// Create a closure that prepends the receiver to the arguments.
					boundIntrinsic := func(ctx context.Context, args ...object.Object) object.Object {
						return intrinsicFn(ctx, append([]object.Object{obj}, args...)...)
					}
					return &object.Intrinsic{Fn: boundIntrinsic}
				}

				// Correct approach: Return a callable placeholder.
				// a. Record the call if the interface is named.
				if staticType.Name != "" {
					key := fmt.Sprintf("%s.%s.%s", staticType.PkgPath, staticType.Name, n.Sel.Name)
					e.logger.DebugContext(ctx, "evalSelectorExpr: recording interface method call", "key", key)
					receiverObj := e.Eval(ctx, n.X, env, pkg)
					e.calledInterfaceMethods[key] = append(e.calledInterfaceMethods[key], receiverObj)
				}

				// b. Find the method definition, checking static, then synthetic, then creating a new one.
				var methodInfo *scan.MethodInfo
				ifaceKey := staticType.PkgPath + "." + staticType.Name

				// Check static methods first.
				if staticType.Interface != nil {
					allMethods := e.getAllInterfaceMethods(ctx, staticType, make(map[string]struct{}))
					for _, method := range allMethods {
						if method.Name == n.Sel.Name {
							methodInfo = method
							break
						}
					}
				}

				// If not found, check the synthetic cache.
				if methodInfo == nil {
					e.syntheticMethodsMutex.Lock()
					if methods, ok := e.syntheticMethods[ifaceKey]; ok {
						methodInfo = methods[n.Sel.Name]
					}
					e.syntheticMethodsMutex.Unlock()
				}

				// If still not found, create a new synthetic method and cache it.
				if methodInfo == nil {
					e.logc(ctx, slog.LevelInfo, "undefined method on interface, creating synthetic method", "interface", staticType.Name, "method", n.Sel.Name)
					methodInfo = &scan.MethodInfo{
						Name:       n.Sel.Name,
						Parameters: []*scan.FieldInfo{}, // Parameters are unknown
						Results:    []*scan.FieldInfo{}, // Results are unknown
					}

					e.syntheticMethodsMutex.Lock()
					if _, ok := e.syntheticMethods[ifaceKey]; !ok {
						e.syntheticMethods[ifaceKey] = make(map[string]*scan.MethodInfo)
					}
					e.syntheticMethods[ifaceKey][n.Sel.Name] = methodInfo
					e.syntheticMethodsMutex.Unlock()
				}

				// Convert the found/created MethodInfo to a FunctionInfo for the placeholder.
				methodFuncInfo := &scan.FunctionInfo{
					Name:       methodInfo.Name,
					Parameters: methodInfo.Parameters,
					Results:    methodInfo.Results,
				}

				// c. Return a callable SymbolicPlaceholder.
				return &object.SymbolicPlaceholder{
					Reason:         fmt.Sprintf("interface method %s.%s", staticType.Name, n.Sel.Name),
					Receiver:       obj, // Pass the variable object itself as the receiver
					UnderlyingFunc: methodFuncInfo,
					Package:        pkg,
				}
			}

			// NEW: Handle struct field access on variables directly
			if staticType != nil && staticType.Kind == scan.StructKind {
				if field, err := e.accessor.findFieldOnType(ctx, staticType, n.Sel.Name); err == nil && field != nil {
					var fieldValue object.Object
					if v, isVar := obj.(*object.Variable); isVar {
						fieldValue = e.evalVariable(ctx, v, pkg)
					} else {
						fieldValue = obj // Should be a placeholder or instance
					}
					return e.resolver.ResolveSymbolicField(ctx, field, fieldValue)
				}
			}
		}
	}

	leftObj := e.Eval(ctx, n.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}

	// Unwrap the result if it's a return value from a previous call in a chain.
	if ret, ok := leftObj.(*object.ReturnValue); ok {
		leftObj = ret.Value
	}

	// We must fully evaluate the left-hand side before trying to select a field or method from it.
	left := e.forceEval(ctx, leftObj, pkg)
	if isError(left) {
		return left
	}

	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", inspectValuer{left})

	switch val := left.(type) {
	case *object.SymbolicPlaceholder:
		return e.evalSymbolicSelection(ctx, val, n.Sel, env, val, n.X.Pos())

	case *object.Package:
		e.logc(ctx, slog.LevelDebug, "evalSelectorExpr: left is a package", "package", val.Path, "selector", n.Sel.Name)

		// If the package object is just a shell, try to fully load it now.
		if val.ScannedInfo == nil {
			e.logc(ctx, slog.LevelDebug, "evalSelectorExpr: package not scanned, attempting to load", "package", val.Path)
			loadedPkg, err := e.getOrLoadPackage(ctx, val.Path)
			if err != nil {
				// if loading fails, it's a real error
				return e.newError(ctx, n.Pos(), "failed to load package %s: %v", val.Path, err)
			}
			// Replace the shell package object with the fully loaded one for the rest of the logic.
			val = loadedPkg
		}

		// If ScannedInfo is still nil after trying to load, it means it's out of policy.
		if val.ScannedInfo == nil {
			e.logc(ctx, slog.LevelDebug, "package not scanned (out of policy), creating placeholder for symbol", "package", val.Path, "symbol", n.Sel.Name)
			// When a symbol is from an unscanned package, we assume it's a function call.
			// Returning a specific UnresolvedFunction allows the call graph to correctly
			// represent this as a terminal node. If it's a type, a subsequent operation
			// like `new()` might fail, but for call-graph analysis, this is the correct trade-off.
			unresolvedFn := &object.UnresolvedFunction{
				PkgPath:  val.Path,
				FuncName: n.Sel.Name,
			}
			val.Env.Set(n.Sel.Name, unresolvedFn)
			return unresolvedFn
		}

		// When we encounter a package selector, we must ensure its environment
		// is populated with all its top-level declarations. This is crucial
		// for closures to capture their environment correctly.
		e.ensurePackageEnvPopulated(ctx, val)

		key := val.Path + "." + n.Sel.Name
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			return &object.Intrinsic{Fn: intrinsicFn}
		}

		// If the symbol is already in the package's environment, return it.
		if symbol, ok := val.Env.Get(n.Sel.Name); ok {
			// If the cached symbol is not callable, but a function with the same name exists,
			// it's a sign of cache pollution. We prioritize the function.
			if !isCallable(symbol) {
				for _, f := range val.ScannedInfo.Functions {
					if f.Name == n.Sel.Name {
						e.logc(ctx, slog.LevelWarn, "correcting polluted cache: found function for non-callable symbol", "package", val.Path, "symbol", n.Sel.Name)
						fnObject := e.getOrResolveFunction(ctx, val, f)
						val.Env.Set(n.Sel.Name, fnObject) // Correct the cache
						return fnObject
					}
				}
			}
			return symbol
		}

		// Try to find the symbol as a function.
		for _, f := range val.ScannedInfo.Functions {
			if f.Name == n.Sel.Name {
				if !ast.IsExported(f.Name) {
					continue
				}

				// Delegate function object creation to the resolver.
				fnObject := e.getOrResolveFunction(ctx, val, f)
				val.Env.Set(n.Sel.Name, fnObject)
				return fnObject
			}
		}

		// If it's not a function, check for constants.
		for _, c := range val.ScannedInfo.Constants {
			if c.Name == n.Sel.Name {
				if !c.IsExported {
					continue // Cannot access unexported constants.
				}

				var constObj object.Object
				switch c.ConstVal.Kind() {
				case constant.String:
					constObj = &object.String{Value: constant.StringVal(c.ConstVal)}
				case constant.Int:
					val, ok := constant.Int64Val(c.ConstVal)
					if !ok {
						return e.newError(ctx, n.Pos(), "could not convert constant %s to int64", c.Name)
					}
					constObj = &object.Integer{Value: val}
				case constant.Bool:
					if constant.BoolVal(c.ConstVal) {
						constObj = object.TRUE
					} else {
						constObj = object.FALSE
					}
				default:
					// Other constant types (float, complex, etc.) are not yet supported.
					// Fall through to create a placeholder.
				}

				if constObj != nil {
					val.Env.Set(n.Sel.Name, constObj) // Cache the resolved constant.
					return constObj
				}
			}
		}

		// Check for types.
		for _, t := range val.ScannedInfo.Types {
			if t.Name == n.Sel.Name {
				if !ast.IsExported(t.Name) {
					continue
				}
				typeObj := &object.Type{
					TypeName:     t.Name,
					ResolvedType: t,
				}
				typeObj.SetTypeInfo(t)
				val.Env.Set(n.Sel.Name, typeObj) // Cache it
				return typeObj
			}
		}

		// Check for variables.
		for _, v := range val.ScannedInfo.Variables {
			if v.Name == n.Sel.Name {
				if !ast.IsExported(v.Name) {
					continue
				}
				resolvedType := e.resolver.ResolveType(ctx, v.Type)
				placeholder := &object.SymbolicPlaceholder{
					Reason: fmt.Sprintf("external variable %s.%s", val.Path, v.Name),
				}
				placeholder.SetFieldType(v.Type)
				placeholder.SetTypeInfo(resolvedType)

				val.Env.Set(n.Sel.Name, placeholder)
				return placeholder
			}
		}

		// If the symbol is not found, assume it's a function we can't see
		// due to the scan policy. Create an UnresolvedFunction object.
		// This allows `applyFunction` to handle it gracefully.
		unresolvedFn := &object.UnresolvedFunction{
			PkgPath:  val.Path,
			FuncName: n.Sel.Name,
		}
		val.Env.Set(n.Sel.Name, unresolvedFn)
		return unresolvedFn

	case *object.Instance:
		// First, check for direct field access on the underlying struct, if it exists.
		if structVal, ok := val.Underlying.(*object.Struct); ok {
			if field, ok := structVal.Get(n.Sel.Name); ok {
				return field
			}
		}

		// Next, check for intrinsics based on the instance's type name.
		key := fmt.Sprintf("(%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(ctx context.Context, args ...object.Object) object.Object {
				return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}
		key = fmt.Sprintf("(*%s).%s", val.TypeName, n.Sel.Name)
		if intrinsicFn, ok := e.intrinsics.Get(key); ok {
			self := val
			fn := func(ctx context.Context, args ...object.Object) object.Object {
				return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
			}
			return &object.Intrinsic{Fn: fn}
		}

		// Fallback to searching for methods and fields via static type info.
		if typeInfo := val.TypeInfo(); typeInfo != nil {
			method, methodErr := e.accessor.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val, n.X.Pos())
			if methodErr == nil && method != nil {
				return method
			}

			var field *scan.FieldInfo
			var fieldErr error
			if typeInfo.Struct != nil {
				field, fieldErr = e.accessor.findFieldOnType(ctx, typeInfo, n.Sel.Name)
				if fieldErr == nil && field != nil {
					return e.resolver.ResolveSymbolicField(ctx, field, val)
				}
			}

			// If we are here, both lookups failed or were ambiguous.
			// If both lookups resulted in an unresolved embedded error, we have an ambiguity.
			// Defer the decision by returning a special object.
			if methodErr == ErrUnresolvedEmbedded && fieldErr == ErrUnresolvedEmbedded {
				return &object.AmbiguousSelector{
					Receiver: val,
					Sel:      n.Sel,
				}
			}

			// If only one of them was an unresolved error, we can make a reasonable guess.
			if fieldErr == ErrUnresolvedEmbedded {
				e.logc(ctx, slog.LevelWarn, "assuming field exists on unresolved embedded type", "field_name", n.Sel.Name, "type_name", val.TypeName)
				return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed field %s on type with unresolved embedded part", n.Sel.Name)}
			}
			if methodErr == ErrUnresolvedEmbedded {
				e.logc(ctx, slog.LevelWarn, "assuming method exists on unresolved embedded type", "method_name", n.Sel.Name, "type_name", val.TypeName)
				return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed method %s on type with unresolved embedded part", n.Sel.Name)}
			}
		}

		return e.newError(ctx, n.Pos(), "undefined method or field: %s on %s", n.Sel.Name, val.TypeName)
	case *object.Pointer:
		// When we have a selector on a pointer, we look for the method on the
		// type of the object the pointer points to.
		pointee := val.Value

		// NEW: Unwrap ReturnValue if present. This handles method calls on pointers
		// returned directly from functions, e.g., `getPtr().Method()`.
		if ret, ok := pointee.(*object.ReturnValue); ok {
			pointee = ret.Value
		}

		// Generalize pointer method lookup. The pointee can be an Instance, a Map, etc.
		// As long as it has TypeInfo, we can find its methods.
		if typeInfo := pointee.TypeInfo(); typeInfo != nil {
			typeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)

			// First, check for intrinsics on both pointer and value receivers.
			// Go allows calling a value-receiver method on a pointer.
			key := fmt.Sprintf("(*%s).%s", typeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val // Receiver is the pointer itself
				fn := func(ctx context.Context, args ...object.Object) object.Object {
					return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}
			key = fmt.Sprintf("(%s).%s", typeName, n.Sel.Name)
			if intrinsicFn, ok := e.intrinsics.Get(key); ok {
				self := val // Receiver is still the pointer
				fn := func(ctx context.Context, args ...object.Object) object.Object {
					return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
				}
				return &object.Intrinsic{Fn: fn}
			}

			// If no intrinsic, resolve the method/field from type info.
			// The receiver for the method call is the pointer itself (`val`), not the pointee.
			method, methodErr := e.accessor.findMethodOnType(ctx, typeInfo, n.Sel.Name, env, val, n.X.Pos())
			if methodErr == nil && method != nil {
				return method
			}

			// Also check for fields if the underlying type is a struct.
			var field *scan.FieldInfo
			var fieldErr error
			if typeInfo.Struct != nil {
				field, fieldErr = e.accessor.findFieldOnType(ctx, typeInfo, n.Sel.Name)
				if fieldErr == nil && field != nil {
					// When accessing a field via a pointer, the receiver is the pointee.
					return e.resolver.ResolveSymbolicField(ctx, field, pointee)
				}
			}

			if methodErr == ErrUnresolvedEmbedded && fieldErr == ErrUnresolvedEmbedded {
				return &object.AmbiguousSelector{
					Receiver: val,
					Sel:      n.Sel,
				}
			}

			if fieldErr == ErrUnresolvedEmbedded {
				e.logc(ctx, slog.LevelWarn, "assuming field exists on unresolved embedded type", "field_name", n.Sel.Name, "type_name", typeName)
				return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed field %s on type with unresolved embedded part", n.Sel.Name)}
			}
			if methodErr == ErrUnresolvedEmbedded {
				e.logc(ctx, slog.LevelWarn, "assuming method exists on unresolved embedded type", "method_name", n.Sel.Name, "type_name", typeName)
				return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("assumed method %s on type with unresolved embedded part", n.Sel.Name)}
			}
		}

		// Handle pointers to symbolic placeholders, which can occur with pointers to unresolved types.
		if sp, ok := pointee.(*object.SymbolicPlaceholder); ok {
			// The logic for selecting from a symbolic placeholder is already well-defined.
			// We can simulate calling that logic with the pointee. The receiver for any
			// method call is the pointer `val`, not the placeholder `sp`.
			// This is effectively doing `(*p).N` where `*p` is a symbolic value.
			return e.evalSymbolicSelection(ctx, sp, n.Sel, env, val, n.X.Pos())
		}

		// If the pointee is not an instance or nothing is found, fall through to the error.
		return e.newError(ctx, n.Pos(), "undefined method or field: %s for pointer type %s", n.Sel.Name, pointee.Type())

	case *object.Nil:
		// Nil can have methods in Go (e.g., interface with nil value).
		// Check if we have type information for this nil (it might be a typed nil interface)
		placeholder := &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("method %s on nil", n.Sel.Name),
		}

		// If the NIL has type information (e.g., it's a typed interface nil),
		// try to find the method in the interface definition
		if left.TypeInfo() != nil && left.TypeInfo().Interface != nil {
			for _, method := range left.TypeInfo().Interface.Methods {
				if method.Name == n.Sel.Name {
					placeholder.UnderlyingFunc = &scan.FunctionInfo{
						Name:       method.Name,
						Parameters: method.Parameters,
						Results:    method.Results,
					}
					placeholder.Receiver = left
					break
				}
			}
		}

		return placeholder

	case *object.UnresolvedFunction:
		// If we are attempting to select a field or method from something we've already
		// determined to be an unresolved function, we can't proceed meaningfully.
		// Return a placeholder to allow analysis to continue without crashing.
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("selection from unresolved function %s.%s", val.PkgPath, val.FuncName),
		}
	case *object.UnresolvedType:
		// If we are selecting from an unresolved type, we can't know what the field or method is.
		// We return a placeholder to allow analysis to continue.
		return &object.SymbolicPlaceholder{
			Reason: fmt.Sprintf("selection from unresolved type %s.%s", val.PkgPath, val.TypeName),
		}
	default:
		return e.newError(ctx, n.Pos(), "expected a package, instance, or pointer on the left side of selector, but got %s", left.Type())
	}
}

// evalSymbolicSelection centralizes the logic for handling a selector expression (e.g., `x.Field` or `x.Method()`)
// where `x` is a symbolic placeholder. This is a common case when dealing with values of unresolved types.
func (e *Evaluator) evalSymbolicSelection(ctx context.Context, val *object.SymbolicPlaceholder, sel *ast.Ident, env *object.Environment, receiver object.Object, receiverPos token.Pos) object.Object {
	typeInfo := val.TypeInfo()
	if typeInfo == nil {
		// If we are calling a method on a placeholder that has no type info (e.g., from an
		// undefined identifier in an out-of-policy package), we can't resolve the method.
		// Instead of erroring, we return another placeholder representing the result of the call.
		return &object.SymbolicPlaceholder{Reason: fmt.Sprintf("result of call to method %q on typeless placeholder", sel.Name)}
	}
	fullTypeName := fmt.Sprintf("%s.%s", typeInfo.PkgPath, typeInfo.Name)
	key := fmt.Sprintf("(*%s).%s", fullTypeName, sel.Name)
	if intrinsicFn, ok := e.intrinsics.Get(key); ok {
		self := val
		fn := func(ctx context.Context, args ...object.Object) object.Object {
			return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
		}
		return &object.Intrinsic{Fn: fn}
	}
	key = fmt.Sprintf("(%s).%s", fullTypeName, sel.Name)
	if intrinsicFn, ok := e.intrinsics.Get(key); ok {
		self := val
		fn := func(ctx context.Context, args ...object.Object) object.Object {
			return intrinsicFn(ctx, append([]object.Object{self}, args...)...)
		}
		return &object.Intrinsic{Fn: fn}
	}

	// Fallback to searching for the method on the instance's type.
	if typeInfo := val.TypeInfo(); typeInfo != nil {
		if method, err := e.accessor.findMethodOnType(ctx, typeInfo, sel.Name, env, receiver, receiverPos); err == nil && method != nil {
			return method
		}

		// If it's not a method, check if it's a field on the struct (including embedded).
		// This must be done *before* the unresolved check, as an unresolved type can still have field info.
		if typeInfo.Struct != nil {
			if field, err := e.accessor.findFieldOnType(ctx, typeInfo, sel.Name); err == nil && field != nil {
				return e.resolver.ResolveSymbolicField(ctx, field, val)
			}
		}

		if typeInfo.Unresolved {
			placeholder := &object.SymbolicPlaceholder{
				Reason:   fmt.Sprintf("symbolic method call %s on unresolved symbolic type %s", sel.Name, typeInfo.Name),
				Receiver: val,
			}
			// Try to find method in interface definition if available
			if typeInfo.Interface != nil {
				for _, method := range typeInfo.Interface.Methods {
					if method.Name == sel.Name {
						// Convert MethodInfo to a temporary FunctionInfo
						placeholder.UnderlyingFunc = &scan.FunctionInfo{
							Name:       method.Name,
							Parameters: method.Parameters,
							Results:    method.Results,
						}
						break
					}
				}
			}
			return placeholder
		}
	}

	// For symbolic placeholders, don't error - return another placeholder
	// This allows analysis to continue even when types are unresolved
	return &object.SymbolicPlaceholder{
		Reason:   "method or field " + sel.Name + " on symbolic type " + val.Inspect(),
		Receiver: val,
	}
}

// getAllInterfaceMethods recursively collects all methods from an interface and its embedded interfaces.
// It handles cycles by keeping track of visited interface types.
// A duplicate of this method exists in `goscan.Scanner` for historical reasons;
// the evaluator needs its own copy to resolve interface method calls during symbolic execution.
func (e *Evaluator) getAllInterfaceMethods(ctx context.Context, ifaceType *scan.TypeInfo, visited map[string]struct{}) []*scan.MethodInfo {
	if ifaceType == nil || ifaceType.Interface == nil {
		return nil
	}

	// Cycle detection
	typeName := ifaceType.PkgPath + "." + ifaceType.Name
	if _, ok := visited[typeName]; ok {
		return nil
	}
	visited[typeName] = struct{}{}

	var allMethods []*scan.MethodInfo
	allMethods = append(allMethods, ifaceType.Interface.Methods...)

	for _, embeddedField := range ifaceType.Interface.Embedded {
		// Resolve the embedded type to get its full definition.
		// Note: embeddedField.Resolve(ctx) creates a new context, so our visited map won't propagate.
		// We need to use the resolver directly or pass the context. Let's assume the resolver handles cycles.
		embeddedTypeInfo, err := embeddedField.Resolve(ctx)
		if err != nil {
			e.logc(ctx, slog.LevelWarn, "could not resolve embedded interface", "type", embeddedField.String(), "error", err)
			continue
		}

		if embeddedTypeInfo != nil && embeddedTypeInfo.Kind == scan.InterfaceKind {
			// Recursively get methods from the embedded interface.
			embeddedMethods := e.getAllInterfaceMethods(ctx, embeddedTypeInfo, visited)
			allMethods = append(allMethods, embeddedMethods...)
		}
	}

	return allMethods
}

func (e *Evaluator) getOrResolveFunction(ctx context.Context, pkg *object.Package, funcInfo *scan.FunctionInfo) object.Object {
	// Generate a unique key for the function. For methods, the receiver type is crucial.
	key := ""
	if funcInfo.Receiver != nil && funcInfo.Receiver.Type != nil {
		// e.g., "example.com/me/impl.(*Dog).Speak"
		key = fmt.Sprintf("%s.(%s).%s", pkg.Path, funcInfo.Receiver.Type.String(), funcInfo.Name)
	} else {
		// e.g., "example.com/me.MyFunction"
		key = fmt.Sprintf("%s.%s", pkg.Path, funcInfo.Name)
	}

	// Check cache first.
	if fn, ok := e.funcCache[key]; ok {
		return fn
	}

	// Not in cache, resolve it.
	fn := e.resolver.ResolveFunction(ctx, pkg, funcInfo)

	// Store in cache for next time.
	e.funcCache[key] = fn
	return fn
}
