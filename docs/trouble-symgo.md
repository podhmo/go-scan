# Troubleshooting `symgo` Infinite Recursion Issue

This document details the investigation and analysis of an infinite recursion issue related to the `symgo` symbolic execution engine, which occurred during the execution of the `find-orphans` tool.

## 1. Problem Overview

When running the `find-orphans` tool, it crashes with a stack overflow while analyzing certain code with recursive data structures. Investigation of the logs revealed that an infinite recursion was occurring within the `scanner.FieldType.String()` method.

This issue is caused by the way `symgo` symbolically represents recursive types and the inability of the `String()` method to safely handle that representation (specifically, cyclic references).

## 2. Relevant Logs

The key parts of the log at the time of the issue are as follows:

**Function Call Stack:**
```
stack.4.func=String
stack.4.pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:862:53
stack.5.func=String
stack.5.pos=$HOME/ghq/github.com/podhmo/go-scan/scanner/models.go:307:19
stack.6.func=String
stack.6.pos=$HOME/ghq/github.com/podhmo/go-scan/scanner/models.go:307:19
...
```
This stack trace shows that the `String()` method was called from a helper function (`getFullName`) in `find-orphans/main.go`, and subsequently, the `String()` method in `scanner/models.go` continued to call itself recursively.

**Symbolic Value:**
```
level=DEBUG msg="evalIdent: found in env" name=ft type=SYMBOLIC_PLACEHOLDER val="<Symbolic: field access on symbolic value <Symbolic: field access on symbolic value ... &instance<...FunctionInfo>.Receiver>.Type>.Elem>.Elem>"
```
This log indicates that the `FieldType` being processed by the `String()` method (variable name `ft`) is a deeply nested symbolic value created through repeated access to the `.Elem` field. This is a result of `symgo` analyzing a recursive data structure.

## 3. Detailed Cause Analysis

The infinite recursion is caused by the following chain of events:

### Step 1: `symgo` Creates a Cyclic `FieldType`

When `symgo` analyzes code with a recursive type definition (e.g., `type T []*T` or `type Node struct { Children []Node }`), it generates symbolic placeholder objects to represent values of that type.

As `symgo` traverses an expression like `node.Children[0].Children[0]`, it constructs `scanner.FieldType` objects to represent the type information of each intermediate value. To faithfully model the recursive nature of the type, this process creates a data structure in memory where a `FieldType`'s `Elem` field points to a `FieldType` that is structurally identical to itself, thus creating a cyclic reference.

### Step 2: `getFullName` Calls the `String()` Method

The `find-orphans` tool uses a helper function, `getFullName`, to generate unique names for functions and methods during its analysis. When it needs to format the type of a method's receiver as a string, it calls the `(*scanner.FieldType).String()` method.

In the problematic case, this `String()` method is called on one of the `FieldType` objects with a cyclic reference created in Step 1.

### Step 3: Infinite Recursion within the `String()` Method

The `String()` method implemented in `scanner/models.go` is not designed to handle cyclic references.

```go
// scanner/models.go
func (ft *FieldType) String() string {
    // ...
    if ft.IsSlice {
        sb.WriteString("[]")
        if ft.Elem != nil {
            // It simply calls String() recursively on the Elem field
            sb.WriteString(ft.Elem.String())
        }
        return sb.String()
    }
    // ...
}
```

When processing the type for a slice or pointer, this method unconditionally calls `String()` recursively on its element type (`ft.Elem`). However, if `ft.Elem` cyclically points back to `ft`, this recursive call will never terminate, eventually causing a stack overflow. The method lacks a mechanism, such as a "visited" set, to detect and break this loop.

## 4. Conclusion

- **Direct Cause**: The `scanner.FieldType.String()` method enters an infinite recursion when it fails to handle a `FieldType` object with a cyclic reference.
- **Root Cause**: `symgo` generates `FieldType` objects with cyclic references when symbolically representing recursive Go types. While this is correct behavior for `symgo`, the problem is that utility functions like `String()` cannot safely handle this output.

Therefore, the bug lies not in `symgo`'s symbolic execution logic itself, but in the lack of robustness of the utility function (`String()`) that consumes its results.
