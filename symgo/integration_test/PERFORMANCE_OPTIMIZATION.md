# Performance Optimization Report for symgo/integration_test

## Executive Summary

The integration tests are experiencing significant performance issues, taking ~7 seconds to complete with extreme memory usage (75GB+). This document outlines the identified bottlenecks and proposed optimizations.

## Performance Analysis

### Test Execution Time
- **Total Duration**: ~7 seconds
- **Memory Allocation**: 75.3GB
- **CPU Profile**: 134% CPU utilization (9.16s CPU time in 6.84s wall time)

## Major Bottlenecks

### 1. Memory Allocation Issues (Critical)

#### Problem
- `fmt.Sprintf` consumes **45.07%** of total memory (33.9GB)
- `SymbolicPlaceholder.Inspect` method triggers massive string allocations
- Total memory usage exceeds 75GB for integration tests

#### Root Cause
```go
// Current implementation uses fmt.Sprintf heavily
fmt.Sprintf("SymbolicPlaceholder{%s}", details)
```

#### Impact
- Excessive GC pressure (23% CPU time in `runtime.gcDrain`)
- System memory exhaustion risk
- Slow test execution

### 2. Infinite Recursion Warnings

#### Problem
Multiple infinite recursion detections in `TestAnalyzeMinigoPackage`:
- `EvalToplevel` function
- `Eval` function
- Cross-function circular dependencies

#### Impact
- Test reliability issues
- Potential stack overflow
- Unnecessary computation cycles

### 3. System Call Overhead

#### Problem
- `syscall.syscall`: 27.84% of CPU time
- `runtime.madvise`: 24.89% of CPU time

#### Impact
- High kernel-space CPU usage
- Memory management overhead

## Optimization Recommendations

### Priority 1: Optimize String Generation

#### Solution 1.1: Replace fmt.Sprintf with strings.Builder
```go
// Before
func (s *SymbolicPlaceholder) Inspect() string {
    return fmt.Sprintf("SymbolicPlaceholder{...}")
}

// After
func (s *SymbolicPlaceholder) Inspect() string {
    var builder strings.Builder
    builder.WriteString("SymbolicPlaceholder{")
    // ... append details
    builder.WriteString("}")
    return builder.String()
}
```

#### Solution 1.2: Implement Lazy Evaluation
```go
type SymbolicPlaceholder struct {
    // ... existing fields
    inspectCache *string // Cache the inspection string
}

func (s *SymbolicPlaceholder) Inspect() string {
    if s.inspectCache != nil {
        return *s.inspectCache
    }
    result := s.buildInspectString()
    s.inspectCache = &result
    return result
}
```

### Priority 2: Fix Infinite Recursion

#### Solution 2.1: Improve Recursion Detection
```go
type RecursionTracker struct {
    visited map[string]int
    maxDepth int
}

func (rt *RecursionTracker) Enter(funcName string) error {
    rt.visited[funcName]++
    if rt.visited[funcName] > rt.maxDepth {
        return fmt.Errorf("max recursion depth exceeded for %s", funcName)
    }
    return nil
}

func (rt *RecursionTracker) Exit(funcName string) {
    rt.visited[funcName]--
}
```

#### Solution 2.2: Skip or Mock Problematic Tests
```go
func TestAnalyzeMinigoPackage(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping minigo analysis in short mode")
    }
    // ... existing test
}
```

### Priority 3: Reduce Memory Allocations

#### Solution 3.1: Object Pooling
```go
var symbolicPlaceholderPool = sync.Pool{
    New: func() interface{} {
        return &SymbolicPlaceholder{}
    },
}

func NewSymbolicPlaceholder() *SymbolicPlaceholder {
    sp := symbolicPlaceholderPool.Get().(*SymbolicPlaceholder)
    // Reset fields
    return sp
}

func (sp *SymbolicPlaceholder) Release() {
    // Clear references
    symbolicPlaceholderPool.Put(sp)
}
```

#### Solution 3.2: Optimize ResolveSymbolicField
```go
func (r *Resolver) ResolveSymbolicField(field string) (*ResolvedField, error) {
    // Add caching layer
    if cached, ok := r.fieldCache[field]; ok {
        return cached, nil
    }
    
    // ... existing resolution logic
    
    r.fieldCache[field] = result
    return result, nil
}
```

### Priority 4: Logging and Debug Overhead

#### Critical Issue: Expensive Operations in Debug Logs
The current implementation calls expensive methods like `Inspect()` and `runtime.Caller()` even when debug logging is disabled:

```go
// PROBLEM: Inspect() is always called, even when debug is disabled
e.logger.Debug("evalIdent: found in env", "name", n.Name, "type", val.Type(), "val", val.Inspect())
```

#### Solution 4.1: Implement Lazy Evaluation for Debug Logs
```go
// Use slog's built-in lazy evaluation with LogValuer interface
type lazyInspector struct {
    obj object.Object
}

func (l lazyInspector) LogValue() slog.Value {
    if l.obj == nil {
        return slog.StringValue("<nil>")
    }
    return slog.StringValue(l.obj.Inspect())
}

// Usage:
e.logger.Debug("evalIdent: found in env", 
    "name", n.Name, 
    "type", val.Type(), 
    "val", lazyInspector{val}) // Inspect() only called if debug is enabled
```

#### Solution 4.2: Conditional runtime.Caller() Execution
```go
func (e *Evaluator) logWithPosition(ctx context.Context, level slog.Level, msg string, args ...any) {
    if !e.logger.Enabled(ctx, level) {
        return // Exit early before calling runtime.Caller()
    }
    
    // Only call runtime.Caller() if logging is enabled
    _, file, line, ok := runtime.Caller(1)
    if ok {
        args = append([]any{slog.String("exec_pos", fmt.Sprintf("%s:%d", file, line))}, args...)
    }
    // ... rest of logging
}
```

#### Solution 4.3: Remove Inspect() from Production Logs
```go
// Instead of logging full object inspection in production
if e.logger.Enabled(ctx, slog.LevelDebug) {
    e.logger.DebugContext(ctx, "evalVariable: already evaluated", 
        "var", v.Name, 
        "value_type", v.Value.Type(), 
        "value", lazyInspector{v.Value})
} else {
    // Log only essential info for non-debug levels
    e.logger.InfoContext(ctx, "evalVariable: already evaluated", 
        "var", v.Name, 
        "value_type", v.Value.Type())
}
```

#### Solution 4.4: Parallel Test Execution
```go
func TestIntegration(t *testing.T) {
    t.Parallel() // Enable parallel execution
    // ... test logic
}
```

## Implementation Plan

### Phase 1: Quick Wins (1-2 days)
1. Replace `fmt.Sprintf` with `strings.Builder` in hot paths
2. Add caching to `SymbolicPlaceholder.Inspect`
3. Implement conditional logging

### Phase 2: Core Optimizations (3-5 days)
1. Implement object pooling for frequently allocated objects
2. Add field resolution caching
3. Fix infinite recursion issues

### Phase 3: Long-term Improvements (1 week)
1. Refactor test architecture to avoid circular dependencies
2. Implement comprehensive caching strategy
3. Add performance regression tests

## Expected Improvements

### Conservative Estimates
- **Memory Usage**: 60-70% reduction (from 75GB to ~20GB)
- **Execution Time**: 40-50% reduction (from 7s to ~3.5s)
- **GC Pressure**: 50% reduction in GC cycles
- **Debug Overhead**: 30-40% reduction in CPU usage when debug logging is disabled

### Optimistic Estimates
- **Memory Usage**: 80-90% reduction (to ~7.5GB)
- **Execution Time**: 70% reduction (to ~2s)
- **GC Pressure**: 80% reduction
- **Debug Overhead**: 90% reduction (near-zero overhead when disabled)

## Monitoring and Validation

### Metrics to Track
1. Total test execution time
2. Peak memory usage
3. GC pause times and frequency
4. CPU utilization profile

### Validation Steps
```bash
# Before optimization
go test ./symgo/integration_test -cpuprofile=before.prof -memprofile=before_mem.prof -bench=.

# After each optimization
go test ./symgo/integration_test -cpuprofile=after.prof -memprofile=after_mem.prof -bench=.

# Compare profiles
go tool pprof -diff_base=before.prof after.prof
```

## Risk Assessment

### Low Risk
- String builder optimizations
- Caching implementations
- Conditional logging

### Medium Risk
- Object pooling (requires careful cleanup)
- Recursion detection changes

### High Risk
- Test architecture refactoring
- Core evaluator modifications

## Conclusion

The primary issue is excessive memory allocation from string formatting operations, consuming over 75GB of memory. By implementing the proposed optimizations in priority order, we can achieve significant performance improvements while maintaining test coverage and reliability.

The most impactful change will be optimizing the `SymbolicPlaceholder.Inspect` method and implementing proper caching strategies. These changes alone should reduce memory usage by at least 60% and improve execution time by 40-50%.