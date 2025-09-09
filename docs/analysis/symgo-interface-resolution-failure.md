# Analysis of `symgo` Interface Resolution Test Failures

## Problem

The test suite for `symgo` has several failing tests related to interface resolution, specifically `TestInterfaceResolution`, `TestInterfaceResolutionWithPointerReceiver`, and `TestInterfaceResolutionWithValueReceiver`. These tests consistently fail with messages indicating that a concrete method was not called via interface resolution as expected.

For example: `expected (*Dog).Speak to be called via interface resolution, but it was not`.

## Initial Investigation & Hypothesis

The `TODO.md` file contained a hint suggesting the root cause was that the `Finalize` function could not discover the in-memory packages created by `scantest`. This led to the initial hypothesis:

*   **Hypothesis 1 (Disproven):** The `Finalize` function in `evaluator.go` looks for packages in the wrong cache (`e.scanner.AllSeenPackages()`) and fails to find the packages loaded into the evaluator's internal cache (`e.pkgCache`) during the test run.

To verify this, debug logging was added to the `Finalize` function to print the keys from both the evaluator's `pkgCache` and the result of `scanner.AllSeenPackages()`.

## Log Analysis & Revised Findings

The log output from the test run revealed the following:

```
level=DEBUG msg="Finalize: e.pkgCache (evaluator) keys" keys=[example.com/me]
level=DEBUG msg="Finalize: e.scanner.AllSeenPackages() keys" keys="[example.com/me example.com/me/impl example.com/me/def]"
```

This result **disproved the initial hypothesis**. The `scanner`'s cache, which `Finalize` uses, correctly contained all necessary packages (`def`, `impl`, and `main`).

Further analysis of the debug logs showed that the `Finalize` function was, in fact, working almost perfectly:
1.  It correctly identified all required packages.
2.  It collected all struct and interface types.
3.  It correctly determined that `impl.Dog` implements the `def.Speaker` interface.
4.  It identified that the `Speaker.Speak` method had been called symbolically.
5.  It found the concrete implementation `impl.Dog.Speak`.
6.  It proceeded to "mark the concrete method as used" by calling the test's `defaultIntrinsic` with a function object representing `(*Dog).Speak`.

## Root Cause & New Hypothesis

The test still fails because the check inside the `defaultIntrinsic` hook never passes. The hook receives an `object.Function` and inspects its `Receiver` field to build a key like `(*example.com/me/impl.Dog).Speak`. The failure implies that the `object.Function` passed to it from `Finalize` is malformed.

*   **Hypothesis 2 (Current):** The root cause is a bug in `resolver.ResolveFunction`. When `Finalize` calls this function to create a callable `object.Function` for a concrete method implementation (e.g., `(*Dog).Speak`), it fails to correctly populate the `Receiver` field on the resulting `object.Function`. Without the receiver information, the test's check cannot validate that the correct concrete method was identified, leading to the test failure.

## Next Steps

The next step in the investigation is to inspect the implementation of `evaluator/resolver.go`'s `ResolveFunction` method to confirm whether it correctly handles the creation of function objects for methods, including setting the receiver type information. If the hypothesis is correct, a fix would involve modifying `ResolveFunction` to properly populate the `Receiver` field.
