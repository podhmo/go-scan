# Performance Analysis of `find-orphans` Command

This document outlines the investigation into a performance degradation observed in the `find-orphans` command. After an initial fix proved insufficient, a data-driven approach using CPU profiling was employed to identify and resolve the true bottleneck.

## 1. The Problem

A significant performance slowdown was noticed in the `find-orphans` command, with execution time increasing from a few seconds to tens of seconds. The issue was correlated with a commit that introduced placeholder implementations for Go's standard built-in functions (`len`, `append`, `make`, etc.) into the `symgo` symbolic execution engine.

## 2. Investigation and Root Cause Analysis

### Initial Hypothesis (Incorrect)

The initial hypothesis was that the performance overhead was caused by a global hook (`defaultIntrinsic`) in `find-orphans` being *executed* for every built-in function call. A fix was implemented to skip the hook's logic for built-ins. However, user feedback confirmed this only yielded a minor (approx. 10%) improvement, indicating it was not the root cause.

### Profiling and Data-Driven Analysis (Correct)

To find the real bottleneck, the application was profiled using Go's `pprof` tool. The CPU profile revealed that a majority of the execution time was being spent in the Go runtime's memory management and garbage collection functions (`runtime.mallocgc`, `runtime.memclrNoHeapPointers`, etc.).

This pointed to a high rate of memory allocation churn. Further analysis of the profile's call graph traced these allocations back to a hot path in the symbolic execution engine: `symgo/evaluator.(*Evaluator).evalIdent`.

The true root cause was identified:
1.  The `evalIdent` function is called for every identifier encountered in the code being analyzed.
2.  When it encountered an identifier for a built-in function (e.g., `len`), it would look it up in the `universe` scope.
3.  Upon finding the built-in, it would **allocate a new `object.Intrinsic` struct on the heap** to wrap the function pointer.
4.  This allocation occurred for **every single call** to a built-in function, creating thousands of short-lived objects and overwhelming the garbage collector.

The initial fix was insufficient because it only prevented the *use* of the created object, not the expensive allocation itself.

## 3. The Solution

The correct, data-driven solution was to eliminate the heap allocations from the `evalIdent` hot path. This was achieved with a "flyweight" pattern, by pre-allocating and caching the `object.Intrinsic` wrappers at program startup.

1.  **Modified `symgo/evaluator/universe.go`**: The global `universe` scope was refactored. Instead of storing raw function pointers, it now pre-allocates and stores the complete `*object.Intrinsic` objects for all built-in functions in a single map upon initialization.

2.  **Modified `symgo/evaluator/evaluator.go`**: The `evalIdent` function was updated to fetch the pre-allocated object directly from the `universe`'s cache. This turns thousands of heap allocations into simple, fast map lookups.

This change successfully addressed the root cause of the performance issue. A timed run of the `find-orphans` command after the fix showed the execution time returning to the sub-second range, confirming the effectiveness of the solution.
