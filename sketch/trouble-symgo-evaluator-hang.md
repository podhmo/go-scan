# Symgo Evaluator Hang due to Incorrect Recursion Handling

## Problem

When running static analysis on certain packages, the `symgo` tool would hang indefinitely. This was reproducible by running `make -C examples/find-orphans`.

## Analysis

The root cause was traced to the symbolic execution engine in `symgo/evaluator/evaluator.go`. The evaluator has a mechanism to detect and halt recursive function calls to prevent infinite loops.

The hang was triggered when the evaluator analyzed the `Get` method of the `minigo.object.Environment` struct. This method is recursive, traversing a linked-list-like structure of environments.

The evaluator's recursion detection correctly identified the recursive call to `Get`. However, the halting mechanism was too simplistic. It would always return a single `SymbolicPlaceholder` object, regardless of the original function's signature.

The `Get` method has two return values: `(Object, bool)`. The code that symbolically called `Get` expected two return values, but the recursion handler only provided one. This type mismatch caused the evaluator to enter an unrecoverable state, leading to the hang.

## Solution

The fix was implemented in the `applyFunction` method within `symgo/evaluator/evaluator.go`. The recursion detection logic was updated to be signature-aware.

When a recursive call is halted, the new logic now:
1.  Inspects the AST of the function definition (`f.Def.AstDecl.Type.Results`) to determine the number of return values.
2.  If the function returns multiple values, it constructs and returns an `object.MultiReturn` object. This object contains the correct number of `object.SymbolicPlaceholder`s.
3.  If the function returns one or zero values (or if the signature is unavailable), it returns a single `object.SymbolicPlaceholder` as before.

This ensures that the symbolic return value from a halted recursive call always matches the arity of the original function's signature, preventing type mismatches and allowing the analysis to proceed correctly.
