# Trouble Analysis: `find-orphans` and Interface Method Calls

This document details the investigation and resolution of a bug in the `find-orphans` tool related to tracking the usage of interface methods.

## 1. Initial Problem

The `find-orphans` tool was not correctly identifying that a concrete method was "used" when it was called via an interface variable.

A test case was created with the following structure:
- An interface `Speaker` with a method `Speak()`.
- Two structs, `Dog` and `Cat`, that both implement `Speaker`.
- A `main` function that creates a `Dog` instance, assigns it to a `Speaker` variable, and calls the `Speak()` method.

**Expected Behavior:** The tool should recognize that `Speaker.Speak()` was called. The analysis should then mark both `Dog.Speak()` and `Cat.Speak()` as "used".

**Actual Behavior:** The tool marked `Dog.Speak()` as used, but incorrectly flagged `Cat.Speak()` as an orphan.

## 2. Investigation and Root Cause in `symgo`

The investigation revealed that the `symgo` symbolic execution engine was too aggressively resolving method calls.

1.  When analyzing `var s Speaker = &Dog{}`, `symgo` correctly identified the static type of `s` as `Speaker`.
2.  However, upon assignment, it overwrote this static type information with the concrete type `*Dog`.
3.  Therefore, when it encountered the call `s.Speak()`, it resolved it directly to a concrete method call on `(*Dog).Speak()`.
4.  This meant the `find-orphans` tool's intrinsic was only ever notified of a concrete call. It never received a generic, polymorphic `SymbolicPlaceholder` for `Speaker.Speak()`, which is the trigger it needs to find all other implementations.

## 3. Solution Part 1: Fixing `symgo` (Complete)

The behavior of `symgo` was corrected. A call on a variable statically typed as an interface should be treated as a polymorphic call.

The fix was implemented in `symgo/evaluator/evaluator.go`, in the `assignIdentifier` function. The logic was changed to preserve the static type of a variable if it is an interface.

**Fixed logic:**
```go
// Preserves the interface type if it was declared as such
if val.TypeInfo() != nil {
    originalType := v.TypeInfo()
    if originalType == nil || originalType.Kind != scanner.InterfaceKind {
        v.ResolvedTypeInfo = val.TypeInfo()
    }
}
```
This fix has been implemented, tested with a new test case (`TestEval_InterfaceMethodCall_OnConcreteType`), and is considered complete.

## 4. Remaining Issue in `find-orphans`

After fixing `symgo`, the `find-orphans` test still fails. With `symgo` now correctly generating a `SymbolicPlaceholder`, the handler in `find-orphans` fails to process it correctly, marking *all* implementations (`Dog.Speak` and `Cat.Speak`) as orphans.

This indicates a latent bug in the `SymbolicPlaceholder` handler in `examples/find-orphans/main.go`.

The bug is almost certainly in this line:
```go
// from the loop inside the SymbolicPlaceholder intrinsic
if m.Name == methodName && m.Receiver != nil && m.Receiver.Type.Definition == impl {
    // ... mark as used ...
}
```
The pointer comparison `m.Receiver.Type.Definition == impl` is likely failing because the two `*scanner.TypeInfo` pointers, despite representing the same type, are not the same instance in memory.

**Next Step:**
The remaining task is to fix this comparison. It should be changed to a more robust check, for example by comparing the names of the types:
```go
// Proposed fix
if m.Name == methodName && m.Receiver != nil && m.Receiver.Type.Definition != nil && m.Receiver.Type.Definition.Name == impl.Name {
    // ...
}
```
Applying this change should resolve the final part of the issue. The combination of the `symgo` fix and this `find-orphans` fix is required for the feature to work correctly.
