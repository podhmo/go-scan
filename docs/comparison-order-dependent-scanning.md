# Comparison of Order-Dependent Scanning Approaches

This document analyzes three different approaches proposed to fix the scanner's inability to handle order-dependent type definitions within a package. The core problem is that the original scanner used single-pass processing, which failed when type references appeared before their definitions in the scanning order.

## Problem Statement

The go-scan tool was failing to correctly resolve types and interfaces when:
1. A type was referenced before its definition in the file scanning order
2. Forward references existed between types in different files within the same package
3. Interface method resolution required complete type information that wasn't available during initial scanning

This manifested as missing generated methods (e.g., `UnmarshalJSON` for the `Event` struct in the `deriving-all` example).

## Approach Comparison

### PR #857: Focused Two-Pass Type Resolution

**Implementation Strategy:**
- **Pass 1:** Create type stubs for all type declarations, collecting basic metadata
- **Pass 2:** Fill in detailed type information using the pre-populated stub registry

**Key Changes:**
- 119 additions, 70 deletions in `scanner/scanner.go`
- Introduces `fillTypeInfoDetails()` function for second-pass processing
- Maintains existing architecture with minimal disruption

**Technical Details:**
```go
// Pass 1: Collect all top-level type declarations (stubs)
for _, decl := range fileAst.Decls {
    if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
        // Create TypeInfo stub without resolving details
    }
}

// Pass 2: Fill in the details for all types
for _, typeInfo := range info.Types {
    s.fillTypeInfoDetails(ctx, typeInfo, info)
}
```

**Strengths:**
- ✅ **Minimal complexity:** Focused solution addressing only the core order-dependency issue
- ✅ **Low risk:** Preserves most existing logic and data structures
- ✅ **Clear separation:** Distinct stub creation and detail resolution phases
- ✅ **Targeted fix:** Specifically addresses type resolution without over-engineering

**Weaknesses:**
- ❌ **Limited scope:** Only handles top-level types, may miss edge cases
- ❌ **Function handling:** Doesn't restructure function processing, potential for future issues
- ❌ **Method association:** May not correctly handle method-to-type associations

### PR #858: Comprehensive Two-Pass Refactoring

**Implementation Strategy:**
- **Pass 1:** Create stubs for both types and functions
- **Pass 2:** Resolve details for all collected stubs
- **Pass 3:** Process constants and variables with full type context

**Key Changes:**
- 267 additions, 299 deletions across 4 files
- Major restructuring of `scanGoFiles()` function
- New data structures: `Methods` field in `TypeInfo`, `LocalTypes` in `FunctionInfo`
- Enhanced function processing with local type detection

**Technical Details:**
```go
// Pass 1: Create stubs for all types and functions
for _, decl := range fileAst.Decls {
    switch d := decl.(type) {
    case *ast.GenDecl:
        if d.Tok == token.TYPE {
            stub := s.createTypeInfoStub(ctx, ts, info, filePath, d)
            info.Types = append(info.Types, stub)
        }
    case *ast.FuncDecl:
        stub := s.createFunctionInfoStub(ctx, d, info, filePath)
        info.Functions = append(info.Functions, stub)
    }
}

// Pass 2: Resolve details
for _, typeInfo := range info.Types {
    s.resolveTypeInfoDetails(ctx, typeInfo, info)
}
for _, funcInfo := range info.Functions {
    s.resolveFunctionInfoDetails(ctx, funcInfo, info)
}
```

**Strengths:**
- ✅ **Comprehensive coverage:** Handles both types and functions systematically
- ✅ **Future-proof architecture:** Well-structured for handling complex scenarios
- ✅ **Method association:** Properly links methods to their receiver types
- ✅ **Local type support:** Handles types defined within function bodies
- ✅ **Clean separation:** Clear phases with distinct responsibilities

**Weaknesses:**
- ❌ **High complexity:** Significant architectural changes with increased maintenance burden
- ❌ **Large changeset:** 267/299 line changes increase review complexity and merge risk
- ❌ **Over-engineering risk:** May be solving problems that don't currently exist
- ❌ **Performance overhead:** Three distinct passes may impact scanning performance

### PR #859: Three-Pass Staged Processing

**Implementation Strategy:**
- **Pass 1:** Create placeholders for all type declarations
- **Pass 2:** Fill in type details with full symbol context
- **Pass 3:** Process remaining declarations (constants, variables, functions)

**Key Changes:**
- 70 additions, 35 deletions across 3 files
- Focused changes to `scanner.go` with minimal architectural disruption
- Introduces `fillTypeInfoFromSpec()` for detailed type processing
- Includes minor fix to `goscan.go` for absolute path resolution

**Technical Details:**
```go
// Pass 1: Create placeholders for all type declarations
for _, decl := range fileAst.Decls {
    if d, ok := decl.(*ast.GenDecl); ok && d.Tok == token.TYPE {
        // Create TypeInfo placeholder
        info.Types = append(info.Types, typeInfo)
    }
}

// Pass 2: Fill in the details for all collected types
for _, typeInfo := range info.Types {
    if ts, ok := typeInfo.Node.(*ast.TypeSpec); ok {
        s.fillTypeInfoFromSpec(ctx, typeInfo, ts, info, importLookup)
    }
}

// Pass 3: Process all other declarations
for _, decl := range fileAst.Decls {
    if d, ok := decl.(*ast.GenDecl); ok {
        if d.Tok != token.TYPE { // Types already detailed
            s.parseGenDecl(ctx, d, info, filePath, importLookup)
        }
    }
}
```

**Strengths:**
- ✅ **Balanced approach:** More thorough than #857 but less complex than #858
- ✅ **Staged processing:** Clear separation of concerns across three passes
- ✅ **Moderate changeset:** 70/35 lines is manageable while being comprehensive
- ✅ **Forward reference support:** Explicitly designed for forward reference resolution
- ✅ **Maintains compatibility:** Preserves existing function processing logic

**Weaknesses:**
- ❌ **Three-pass overhead:** Additional processing pass may impact performance
- ❌ **Function-type coordination:** Less sophisticated handling of method-receiver relationships
- ❌ **Limited function restructuring:** Doesn't modernize function processing architecture

## Performance Analysis

| Approach | Memory Footprint | Processing Passes | Architectural Impact |
|----------|------------------|-------------------|---------------------|
| **PR #857** | 6,709,552 B | 2 | Low |
| **PR #858** | 7,068,808 B | 3 | High |
| **PR #859** | 6,789,240 B | 3 | Medium |

**Memory Usage:** All approaches show similar memory consumption, with PR #858 having slightly higher usage due to additional data structures.

## Correctness Analysis

All three approaches should correctly resolve the forward reference issue, but differ in completeness:

| Feature | PR #857 | PR #858 | PR #859 |
|---------|---------|---------|---------|
| Forward type references | ✅ | ✅ | ✅ |
| Method-receiver association | ⚠️ | ✅ | ⚠️ |
| Local function types | ❌ | ✅ | ❌ |
| Interface method resolution | ✅ | ✅ | ✅ |
| Cross-file dependencies | ✅ | ✅ | ✅ |

## Recommendation

**Recommended Approach: PR #859 (Three-Pass Staged Processing)**

### Rationale:

1. **Optimal Balance:** PR #859 provides the best balance between solving the core problem and maintaining implementation simplicity.

2. **Sufficient Completeness:** While PR #858 is more comprehensive, the additional complexity may not be justified for the current problem scope. PR #859 addresses all known issues with forward references.

3. **Clear Architecture:** The three-pass approach provides clear separation of concerns:
   - Pass 1: Symbol discovery
   - Pass 2: Type resolution
   - Pass 3: Everything else

4. **Manageable Complexity:** With only 70 additions and 35 deletions, the changes are substantial enough to solve the problem but not so large as to create maintenance burden.

5. **Forward Compatibility:** The staged approach can be extended in the future if more sophisticated processing is needed.

### Implementation Considerations:

- **Performance:** While three passes seem like overhead, the clear separation makes each pass simpler and potentially faster than complex single-pass logic with backtracking.

- **Testing:** The staged approach makes it easier to test each phase independently.

- **Debugging:** Clear phase separation simplifies debugging type resolution issues.

### Alternative Scenarios:

- **Choose PR #857** if: Minimal risk is paramount and the current issue is the only concern
- **Choose PR #858** if: A comprehensive architecture overhaul is desired and future extensibility is critical

## Conclusion

PR #859 represents the optimal solution for the current order-dependency problem. It provides comprehensive coverage of forward reference scenarios while maintaining a clean, understandable architecture that can be maintained and extended as needed.

The three-pass approach—symbol discovery, type resolution, and remaining processing—mirrors the conceptual way developers think about type dependencies and provides a solid foundation for reliable type scanning in go-scan.