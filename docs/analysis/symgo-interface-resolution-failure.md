# Analysis of `symgo` Interface Resolution Test Failures

## Problem

The test suite for `symgo` has several failing tests related to interface resolution, specifically `TestInterfaceResolution`, `TestInterfaceResolutionWithPointerReceiver`, and `TestInterfaceResolutionWithValueReceiver`. These tests consistently fail with messages indicating that a concrete method was not called via interface resolution as expected.

For example: `expected (*Dog).Speak to be called via interface resolution, but it was not`.

## Initial Investigation & Hypothesis

The `TODO.md` file contained a hint suggesting the root cause was that the `Finalize` function could not discover the in-memory packages created by `scantest`. This led to the initial hypothesis:

*   **Hypothesis 1 (Disproven):** The `Finalize` function in `evaluator.go` looks for packages in the wrong cache (`e.scanner.AllSeenPackages()`) and fails to find the packages loaded into the evaluator's internal cache (`e.pkgCache`) during the test run.

To verify this, debug logging was added to the `Finalize` function to inspect both caches.

## Log Analysis & Revised Findings

The log output from the test run revealed that the initial hypothesis was **incorrect**. `e.scanner.AllSeenPackages()` *does* contain all the necessary packages.

Deeper analysis of the logs showed that the `Finalize` function correctly identifies the interface, the implementing struct, and the concrete method (`(*Dog).Speak`). It then calls the test's `defaultIntrinsic` with a corresponding `object.Function`.

*   **New Hypothesis (Root Cause):** The test fails because the `object.Function` passed to the intrinsic from `Finalize` is not constructed correctly. The test's intrinsic checks the `Receiver` field of the `object.Function` to verify which method was called, but this field appears to be `nil` or incorrect. The problem lies within `resolver.ResolveFunction` not properly setting the receiver information when creating the function object for the concrete method.

## Next Steps

The next step in the investigation should be to inspect the implementation of `evaluator/resolver.go`'s `ResolveFunction` method to confirm whether it correctly handles the creation of function objects for methods, including setting the receiver type information. If the hypothesis is correct, a fix would involve modifying `ResolveFunction` to properly populate the `Receiver` field. It is important to ensure that this fix does not introduce regressions in other tests that might rely on the current behavior.
