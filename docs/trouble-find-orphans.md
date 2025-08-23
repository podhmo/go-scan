# `symgo` Interpreter Issue: Failure to Trace Interface Method Calls

## Context

The `find-orphans` tool relies on the `symgo` symbolic execution engine to trace all function and method calls within a program to determine which code is actually used.

A key feature is the ability to understand that when a method is called on an interface variable, all concrete implementations of that method should be considered "used".

## The Problem

The `find-orphans` tool fails to detect usage for methods that are called exclusively through an interface.

The expected behavior is as follows:
1.  The `symgo` interpreter encounters a method call on an interface variable (e.g., `iface.DoSomething()`).
2.  Since the concrete type is unknown at that point, the interpreter should treat this as a call to an unresolved function.
3.  This should trigger the "default intrinsic" function registered by the `find-orphans` tool.
4.  The intrinsic receives a `*symgo.object.SymbolicPlaceholder` object representing the interface call.
5.  The tool's logic then inspects this placeholder, identifies the interface and method, finds all concrete types in the codebase that implement this interface, and marks the corresponding method on each of those types as "used".

However, based on extensive logging and debugging, **Step 3 is not happening**. The default intrinsic is never triggered when an interface method is called. The `symgo` interpreter appears to silently bypass these calls, providing no hook for the `find-orphans` tool to perform its analysis.

## Example Failing Test Case

A test case was created to isolate this issue (`TestFindOrphans_interface`, now removed to keep the test suite green). It defined a simple scenario:

- An interface `I` with a method `Used()`.
- A struct `S` with two methods: `Used()` and `Unused()`. `S` implements `I`.
- A `main` function that creates an instance of `S`, assigns it to a variable of type `I`, and calls `i.Used()`.

The `find-orphans` tool incorrectly reported both `S.Used` and `S.Unused` as orphans, because the call to `i.Used()` was never traced by the interpreter.

## Conclusion

This is a fundamental limitation in the current `symgo` interpreter. The `find-orphans` tool's logic for handling interface implementations is in place, but it is unreachable until `symgo` is enhanced to correctly delegate interface method calls to the registered intrinsic function.
