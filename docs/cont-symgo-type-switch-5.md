# Continuation of Sym-Go Type Switch Implementation (5)

## Goal

The primary objective remains to fix the test failures related to `symgo`'s type switch (`switch v := i.(type)`) and type assertion (`if v, ok := i.(T)`) capabilities. The key failing tests are `TestInterfaceBinding`, `TestTypeSwitch_MethodCall`, `TestIfOk_FieldAccess`, and `TestTypeSwitch_Complex`.

## Work Summary & Current Status

1.  **Build & Test Suite Stabilization**: The initial state of the codebase was a non-building one due to a major refactoring of `applyFunction`. I have methodically fixed all build errors across the test suite. I also diagnosed and fixed several panics that arose after the build was repaired, which were caused by incorrect assumptions in test setup about how the evaluator's environment caching works. The test suite is now stable (no panics), providing a reliable baseline for feature work.

2.  **Root Cause Analysis**: After stabilizing the tests, I was able to reliably reproduce the core feature failures. My analysis, aided by extensive logging, has pinpointed the root cause:
    When a type switch or `if-ok` assertion creates a new, type-narrowed variable (e.g., `v`), the evaluator correctly identifies the new type but creates a new, generic `SymbolicPlaceholder` for it. This new placeholder loses the connection to the *original concrete object* that the interface variable (`i`) held. Consequently, when a method call or field access is performed on `v` (e.g., `v.Greet()` or `v.Name`), the evaluator cannot access the fields or dispatch to the methods of the original concrete object.

## The Confirmed Plan

To fix this, the link between the new variable `v` and the original object `i` must be maintained. The `symgo/object/object.go` file already contains the necessary field for this on the `SymbolicPlaceholder` struct: `Original object.Object`.

The plan consists of three parts:

1.  **Set the `Original` field**: In `evaluator.go`, modify `evalTypeSwitchStmt` and the `if-ok` logic within `evalAssignStmt`. When creating the `SymbolicPlaceholder` for the new type-narrowed variable, set its `Original` field to point to the original object that was being asserted.

2.  **Use the `Original` field**: In `evaluator.go`, modify `evalSelectorExpr`. This function handles expressions like `v.Name`. Add logic at the beginning to check if the receiver (`v`) is a placeholder with a non-nil `Original` field. If it is, the selector logic (finding the field or method) must be performed on the `placeholder.Original` object, not the placeholder itself. This will correctly resolve members on the concrete value.

3.  **Rename `evalSelectorExpr`**: To implement the above cleanly, the existing `evalSelectorExpr` logic should be renamed to `evalSelectorExprForObject`, and a new `evalSelectorExpr` wrapper function should be created to contain the new logic that checks for the `Original` field.

## Blockage

I have repeatedly failed to apply the necessary patches to `evaluator.go` with the `replace_with_git_merge_diff` tool. The tool consistently reports that the search block cannot be found, which indicates my local understanding of the file's content is out of sync with the true state on the machine, likely due to prior, partially successful patch attempts.

## The Exact Diffs for the Next Agent

The following changes should be applied to `symgo/evaluator/evaluator.go` to implement the plan. Please apply them carefully, one by one.

**Change 1: Modify `evalTypeSwitchStmt`**
```
replace_with_git_merge_diff
symgo/evaluator/evaluator.go
<<<<<<< SEARCH
					val := &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("type switch case variable %s", fieldType.String()),
						BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
					}
					v := &object.Variable{
						Name:        varName,
						Value:       val,
=======
					val := &object.SymbolicPlaceholder{
						Reason:     fmt.Sprintf("value from type switch to %s", fieldType.String()),
						Original:   originalObj,
						BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
					}
					v := &object.Variable{
						Name:        varName,
						Value:       val,
>>>>>>> REPLACE
```

**Change 2: Modify `evalAssignStmt` (for the `if-ok` case)**
```
replace_with_git_merge_diff
symgo/evaluator/evaluator.go
<<<<<<< SEARCH
			// Evaluate the source expression to trace calls
			e.Eval(ctx, typeAssert.X, env, pkg)

			// Resolve the asserted type (T).
=======
			originalObj := e.Eval(ctx, typeAssert.X, env, pkg)
			if isError(originalObj) {
				return originalObj
			}

			// Resolve the asserted type (T).
>>>>>>> REPLACE
<<<<<<< SEARCH
			valuePlaceholder := &object.SymbolicPlaceholder{
				Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			}
=======
			valuePlaceholder := &object.SymbolicPlaceholder{
				Reason:     fmt.Sprintf("value from type assertion to %s", fieldType.String()),
				Original:   originalObj,
				BaseObject: object.BaseObject{ResolvedTypeInfo: resolvedType, ResolvedFieldType: fieldType},
			}
>>>>>>> REPLACE
```

**Change 3: Refactor `evalSelectorExpr`**
This is a larger change. The goal is to rename the current `evalSelectorExpr` to `evalSelectorExprForObject` and create a new `evalSelectorExpr` wrapper.

*Step 3a: Rename existing function*
```
replace_with_git_merge_diff
symgo/evaluator/evaluator.go
<<<<<<< SEARCH
func (e *Evaluator) evalSelectorExpr(ctx context.Context, n *ast.SelectorExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	leftObj := e.Eval(ctx, n.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}

	// Unwrap return values early.
	if ret, ok := leftObj.(*object.ReturnValue); ok {
		leftObj = ret.Value
	}
	return e.evalSelectorExprForObject(ctx, n, leftObj, env, pkg)
}
=======
func (e *Evaluator) evalSelectorExprForObject(ctx context.Context, n *ast.SelectorExpr, left object.Object, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// We must fully evaluate the left-hand side before trying to select a field or method from it.
	left = e.forceEval(ctx, left, pkg)
	if isError(left) {
		return left
	}
	e.logger.Debug("evalSelectorExpr: evaluated left", "type", left.Type(), "value", inspectValuer{left})

	switch val := left.(type) {
	case *object.SymbolicPlaceholder:
		return e.evalSymbolicSelection(ctx, val, n.Sel, env, val, n.X.Pos())
	// ... (the rest of the original function body) ...
	default:
		return e.newError(ctx, n.Pos(), "expected a package, instance, or pointer on the left side of selector, but got %s", left.Type())
	}
}
>>>>>>> REPLACE
```
*Note: The replacement block for Step 3a should contain the full body of the original `evalSelectorExpr` function, with the signature changed and the initial `leftObj` evaluation removed.*

*Step 3b: Add the new wrapper function*
```
create_file_with_block
symgo/evaluator/evaluator.go
// (This would be an append operation, but using create_file_with_block as a placeholder for the logic)
// The following function should be added *before* the newly renamed evalSelectorExprForObject

func (e *Evaluator) evalSelectorExpr(ctx context.Context, n *ast.SelectorExpr, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	leftObj := e.Eval(ctx, n.X, env, pkg)
	if isError(leftObj) {
		return leftObj
	}

	// Unwrap return values early.
	if ret, ok := leftObj.(*object.ReturnValue); ok {
		leftObj = ret.Value
	}

	// Check for type-narrowed placeholders
	if placeholder, ok := leftObj.(*object.SymbolicPlaceholder); ok {
		if placeholder.Original != nil {
			narrowedFt := placeholder.FieldType()
			if narrowedFt == nil {
				return e.evalSelectorExprForObject(ctx, n, leftObj, env, pkg)
			}

			originalConcreteObj := e.forceEval(ctx, placeholder.Original, pkg)
			if isError(originalConcreteObj) {
				return originalConcreteObj
			}

			var concreteIsPointer bool
			var concreteTi *scanner.TypeInfo
			if ptr, ok := originalConcreteObj.(*object.Pointer); ok {
				concreteIsPointer = true
				if ptr.Value != nil {
					concreteTi = ptr.Value.TypeInfo()
				}
			} else {
				concreteIsPointer = false
				concreteTi = originalConcreteObj.TypeInfo()
			}

			if concreteTi != nil {
				// Check 1: Package path must match.
				// Check 2: Pointer-ness must match.
				// Check 3: Base type name must match.
				if narrowedFt.FullImportPath == concreteTi.PkgPath && narrowedFt.IsPointer == concreteIsPointer {
					baseNarrowedName := narrowedFt.TypeName
					if narrowedFt.IsPointer && narrowedFt.Elem != nil {
						baseNarrowedName = narrowedFt.Elem.TypeName
					}
					if baseNarrowedName == concreteTi.Name {
						// Compatible. Unwrap and evaluate on the original concrete object.
						return e.evalSelectorExprForObject(ctx, n, placeholder.Original, env, pkg)
					}
				}
			}
			// If not compatible, it's an impossible path. Prune it by returning a placeholder.
			return &object.SymbolicPlaceholder{Reason: "pruned path in type switch/assertion"}
		}
	}

	// Fallback to original logic for non-narrowed placeholders or other types.
	return e.evalSelectorExprForObject(ctx, n, leftObj, env, pkg)
}
```

## Next Steps for Successor

1.  Carefully apply the three logical changes described above to `symgo/evaluator/evaluator.go`.
2.  Run `go test -v github.com/podhmo/go-scan/symgo/evaluator` to confirm that `TestTypeSwitch_MethodCall`, `TestIfOk_FieldAccess`, and `TestTypeSwitch_Complex` now pass.
3.  Address any remaining test failures.
4.  Submit the final, passing code.
