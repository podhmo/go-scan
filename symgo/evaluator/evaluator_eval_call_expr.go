package evaluator

import (
	"context"
	"fmt"
	"go/ast"
	"log/slog"

	scan "github.com/podhmo/go-scan/scanner"
	"github.com/podhmo/go-scan/symgo/object"
)

func (e *Evaluator) evalCallExpr(ctx context.Context, n *ast.CallExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	if e.logger.Enabled(ctx, slog.LevelDebug) {
		stackAttrs := make([]any, 0, len(e.callStack))
		for i, frame := range e.callStack {
			posStr := ""
			if e.scanner != nil && e.scanner.Fset() != nil && frame.Pos.IsValid() {
				posStr = e.scanner.Fset().Position(frame.Pos).String()
			}
			stackAttrs = append(stackAttrs, slog.Group(fmt.Sprintf("%d", i),
				slog.String("func", frame.Function),
				slog.String("pos", posStr),
			))
		}
		e.logger.Log(ctx, slog.LevelDebug, "call", slog.Group("stack", stackAttrs...))
	}

	function := e.Eval(ctx, n.Fun, env, pkg)
	if isError(function) {
		return function
	}

	// If the function expression itself resolves to a return value (e.g., from an interface method call
	// that we intercept), we need to unwrap it before applying it.
	if ret, ok := function.(*object.ReturnValue); ok {
		function = ret.Value
	}

	args := e.evalExpressions(ctx, n.Args, env, pkg)
	if len(args) == 1 && isError(args[0]) {
		return args[0]
	}

	// If the call includes `...`, the last argument is a slice to be expanded.
	// We wrap it in a special Variadic object to signal this to `applyFunction`.
	if n.Ellipsis.IsValid() {
		if len(args) == 0 {
			return e.newError(ctx, n.Ellipsis, "invalid use of ... with no arguments")
		}
		lastArg := args[len(args)-1]
		// The argument should be a slice, but we don't check it here.
		// `extendFunctionEnv` will handle the type logic.
		args[len(args)-1] = &object.Variadic{Value: lastArg}
	}

	// After evaluating arguments, check if any of them are function literals.
	// If so, we need to "scan" inside them to find usages. This must be done
	// before the default intrinsic is called, so the usage map is populated
	// before the parent function call is even registered.
	for _, arg := range args {
		if fn, ok := arg.(*object.Function); ok {
			e.scanFunctionLiteral(ctx, fn)
		}
	}

	if e.defaultIntrinsic != nil {
		// The default intrinsic is a "catch-all" handler that can be used for logging,
		// dependency tracking, etc. It receives the function object itself as the first
		// argument, followed by the regular arguments.

		// Pass the current call frame in the context so the intrinsic can know the caller.
		intrinsicCtx := ctx
		if len(e.callStack) > 0 {
			callerFrame := e.callStack[len(e.callStack)-1]
			intrinsicCtx = context.WithValue(ctx, callFrameKey, callerFrame)
		}
		e.defaultIntrinsic(intrinsicCtx, append([]object.Object{function}, args...)...)
	}

	result := e.applyFunction(ctx, function, args, pkg, n.Pos())
	if isError(result) {
		return result
	}
	return result
}

// scanFunctionLiteral evaluates the body of a function literal or method value in a new,
// symbolic environment. This is used to find function calls inside anonymous functions or
// method values that are passed as arguments, without needing to fully execute the function
// they are passed to.
func (e *Evaluator) scanFunctionLiteral(ctx context.Context, fn *object.Function) {
	if fn.Body == nil || fn.Package == nil {
		return // Nothing to scan.
	}

	// Prevent infinite recursion.
	if e.scanLiteralInProgress[fn.Body] {
		return
	}
	e.scanLiteralInProgress[fn.Body] = true
	defer delete(e.scanLiteralInProgress, fn.Body)

	e.logger.DebugContext(ctx, "scanning function literal/method value to find usages", "pos", fn.Package.Fset.Position(fn.Body.Pos()))

	// Create a new environment for the function's execution.
	// It's enclosed by the environment where the function was defined.
	fnEnv := object.NewEnclosedEnvironment(fn.Env)

	// Bind receiver if it's a method value.
	if fn.Receiver != nil && fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
		recvField := fn.Decl.Recv.List[0]
		if len(recvField.Names) > 0 && recvField.Names[0].Name != "" {
			receiverName := recvField.Names[0].Name
			fnEnv.SetLocal(receiverName, fn.Receiver)
			e.logger.DebugContext(ctx, "scanFunctionLiteral: bound receiver", "name", receiverName, "type", fn.Receiver.Type())
		}
	}

	// Populate the environment with symbolic placeholders for the parameters.
	if fn.Parameters != nil {
		var importLookup map[string]string
		file := fn.Package.Fset.File(fn.Body.Pos())
		if file != nil {
			if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
				importLookup = e.scanner.BuildImportLookup(astFile)
			}
		}
		if importLookup == nil && len(fn.Package.AstFiles) > 0 {
			for _, astFile := range fn.Package.AstFiles {
				importLookup = e.scanner.BuildImportLookup(astFile)
				break
			}
		}

		for _, field := range fn.Parameters.List {
			fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, fn.Package, importLookup)
			var resolvedType *scan.TypeInfo
			if fieldType != nil {
				resolvedType = e.resolver.ResolveType(ctx, fieldType)
			}

			placeholder := &object.SymbolicPlaceholder{
				Reason: "symbolic parameter for function scan",
				BaseObject: object.BaseObject{
					ResolvedTypeInfo:  resolvedType,
					ResolvedFieldType: fieldType,
				},
			}

			for _, name := range field.Names {
				if name.Name != "_" {
					v := &object.Variable{Name: name.Name, Value: placeholder}
					v.SetFieldType(fieldType)
					v.SetTypeInfo(resolvedType)
					fnEnv.Set(name.Name, v)
				}
			}
		}
	}

	// Now evaluate the body. The result is ignored; we only care about the side effects.
	e.Eval(ctx, fn.Body, fnEnv, fn.Package)
}

func (e *Evaluator) extendFunctionEnv(ctx context.Context, fn *object.Function, args []object.Object, baseEnv *object.Environment) (*object.Environment, error) {
	var env *object.Environment
	if baseEnv != nil {
		env = baseEnv
	} else {
		env = object.NewEnclosedEnvironment(fn.Env)
	}

	// 1. Bind receiver
	if fn.Decl != nil && fn.Decl.Recv != nil && len(fn.Decl.Recv.List) > 0 {
		recvField := fn.Decl.Recv.List[0]
		if len(recvField.Names) > 0 && recvField.Names[0].Name != "" && recvField.Names[0].Name != "_" {
			receiverName := recvField.Names[0].Name
			receiverToBind := fn.Receiver
			if receiverToBind == nil {
				var importLookup map[string]string
				if file := fn.Package.Fset.File(fn.Decl.Pos()); file != nil {
					if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
						importLookup = e.scanner.BuildImportLookup(astFile)
					}
				}
				fieldType := e.scanner.TypeInfoFromExpr(ctx, recvField.Type, nil, fn.Package, importLookup)
				resolvedType := e.resolver.ResolveType(ctx, fieldType)
				receiverToBind = &object.SymbolicPlaceholder{
					Reason:     "symbolic receiver for entry point method",
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
				}
			}
			env.SetLocal(receiverName, receiverToBind)
		}
	}

	// 2. Bind named return values (if any)
	// This must be done before binding parameters, in case a parameter has the same name.
	if fn.Decl != nil && fn.Decl.Type.Results != nil {
		for _, field := range fn.Decl.Type.Results.List {
			if len(field.Names) == 0 {
				continue // Unnamed return value
			}
			var importLookup map[string]string
			if file := fn.Package.Fset.File(field.Pos()); file != nil {
				if astFile, ok := fn.Package.AstFiles[file.Name()]; ok {
					importLookup = e.scanner.BuildImportLookup(astFile)
				}
			}

			fieldType := e.scanner.TypeInfoFromExpr(ctx, field.Type, nil, fn.Package, importLookup)
			resolvedType := e.resolver.ResolveType(ctx, fieldType)

			for _, name := range field.Names {
				if name.Name == "_" {
					continue
				}
				// The zero value for any type in symbolic execution is a placeholder.
				// This placeholder carries the type information of the variable.
				zeroValue := &object.SymbolicPlaceholder{
					Reason:     "zero value for named return",
					BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
				}
				v := &object.Variable{
					Name:        name.Name,
					Value:       zeroValue,
					IsEvaluated: true, // It has its zero value.
				}
				v.SetFieldType(fieldType)
				v.SetTypeInfo(resolvedType)
				env.SetLocal(name.Name, v)
			}
		}
	}

	// 3. Bind parameters
	if fn.Def != nil {
		// Bind parameters using the reliable FunctionInfo definition
		argIndex := 0
		for i, paramDef := range fn.Def.Parameters {
			var arg object.Object
			if argIndex < len(args) {
				arg = args[argIndex]
				argIndex++
			} else {
				arg = &object.SymbolicPlaceholder{Reason: "symbolic parameter for entry point"}
			}

			if paramDef.Name != "" && paramDef.Name != "_" {
				// If the argument is a function, we need to "tag" it with the current call stack
				// to enable recursion detection through higher-order functions.
				if funcArg, ok := arg.(*object.Function); ok {
					clonedFunc := funcArg.Clone()
					// Create a copy of the call stack to avoid shared state issues.
					stackCopy := make([]*object.CallFrame, len(e.callStack))
					copy(stackCopy, e.callStack)
					clonedFunc.BoundCallStack = stackCopy
					arg = clonedFunc // Use the tagged clone for the binding
				}

				v := &object.Variable{
					Name:        paramDef.Name,
					Value:       arg,
					IsEvaluated: true,
				}

				// The static type from the function signature is the most reliable source.
				staticFieldType := paramDef.Type
				if staticFieldType != nil {
					staticTypeInfo := e.resolver.ResolveType(ctx, staticFieldType)
					v.SetFieldType(staticFieldType)
					v.SetTypeInfo(staticTypeInfo)
				} else {
					// Fallback to the dynamic type from the argument.
					v.SetFieldType(arg.FieldType())
					v.SetTypeInfo(arg.TypeInfo())
				}

				// If the argument is NIL and we have static type info, preserve it on the object.
				if nilObj, ok := arg.(*object.Nil); ok && staticFieldType != nil {
					nilObj.SetFieldType(v.FieldType())
					nilObj.SetTypeInfo(v.TypeInfo())
				}
				env.SetLocal(paramDef.Name, v)
			}

			// Handle variadic parameters using the flag on the FunctionInfo.
			if fn.Def.IsVariadic && i == len(fn.Def.Parameters)-1 {
				// This is the variadic parameter. The logic here would need to collect remaining args into a slice.
				// For now, we assume the single variadic argument is handled correctly by the caller providing a slice.
				// This part of the refactoring is left as a TODO if complex variadic cases fail.
				break
			}
		}
	} else if fn.Parameters != nil {
		// Fallback for function literals which don't have a FunctionInfo
		e.logc(ctx, slog.LevelDebug, "function definition not available in extendFunctionEnv, falling back to AST", "function", fn.Name)
		argIndex := 0
		for _, field := range fn.Parameters.List {
			// Handle variadic parameters indicated by Ellipsis in the AST
			isVariadic := false
			if _, ok := field.Type.(*ast.Ellipsis); ok {
				isVariadic = true
			}

			for _, name := range field.Names {
				if argIndex >= len(args) {
					break
				}
				if name.Name != "_" {
					var valToBind object.Object
					if isVariadic {
						// Collect remaining args into a slice for the variadic parameter
						sliceElements := args[argIndex:]
						valToBind = &object.Slice{Elements: sliceElements}
						// We could try to infer a field type here if needed
					} else {
						valToBind = args[argIndex]
					}

					v := &object.Variable{
						Name:        name.Name,
						Value:       valToBind,
						IsEvaluated: true,
					}
					v.SetTypeInfo(valToBind.TypeInfo())
					v.SetFieldType(valToBind.FieldType())
					env.SetLocal(name.Name, v)
				}
				if !isVariadic {
					argIndex++
				}
			}
			if isVariadic {
				break // Variadic parameter is always the last one
			}
		}
	}

	return env, nil
}
