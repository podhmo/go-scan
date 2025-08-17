# Trouble Report: Investigation into `json.Unmarshal` Error Propagation

This document details the investigation into a bug where `json.Unmarshal` errors were not correctly propagated through the `minigo` FFI, resulting in a `nil` value within the script.

## 1. Summary of Actions

The initial task was to fix a bug noted in `TODO.md`: "*Fix `json.Unmarshal` error propagation: The FFI fails to correctly propagate `*json.UnmarshalTypeError` from `json.Unmarshal`, returning a `nil` value instead.*"

My process was as follows:
1.  **Reproduce the Bug:** I created a new test case (`TestStdlib_json_unmarshalTypeError`) that specifically triggers a `*json.UnmarshalTypeError` by attempting to unmarshal a JSON string into an `int` field.
2.  **Confirm Failure:** I ran the test and confirmed that the `err` variable in the `minigo` script was `object.NIL`, successfully reproducing the bug.
3.  **Attempt Fixes:** I formulated hypotheses about the cause and attempted several fixes within the FFI logic in `minigo/evaluator/evaluator.go`.
4.  **Investigate Failures:** When the fixes failed, I attempted several methods of debugging and investigation to understand the root cause.
5.  **Stalemate:** After multiple failed attempts, I concluded that I was missing a fundamental piece of information and could not solve the bug with my current understanding. All attempted changes have been reverted.

## 2. Investigation Log & Findings

My investigation focused on the `nativeToValue` function in `minigo/evaluator/evaluator.go`, which is responsible for converting Go values returned from FFI calls into `minigo` objects.

### Initial Hypothesis
The `error` value returned by `json.Unmarshal` (an interface) was being incorrectly processed by the generic `reflect.Interface` handling logic in `nativeToValue`. Specifically, I believed the `val.IsNil()` check was the source of the problem.

### Investigation 1: The `case error:` Fix
-   **Method:** I modified `nativeToValue` to add a specific `case error:` to the type switch. This case would check if the error value `v` was `nil` (`if v == nil`) and, if not, wrap it in an `*object.GoValue`. This seemed logically sound.
-   **Result:** The test still failed with the exact same error: the `err` object in the script was `nil`.
-   **Conclusion:** This was my first major surprise. The most direct and logical fix had no effect, indicating my initial hypothesis was either wrong or incomplete.

### Investigation 2: Flawed Debugging with `fmt.Printf`
-   **Method:** To see what was happening inside `nativeToValue`, I added a `fmt.Printf` statement to log the properties of the `reflect.Value` being processed, including a call to `.IsNil()`.
-   **Result:** This caused widespread panics and test failures across the entire test suite. The panic message was `reflect: call of reflect.Value.IsNil on string Value`.
-   **Conclusion:** I learned that `nativeToValue` is a hot path called for *all* types of Go values, not just interfaces. My debugging code was flawed because it unconditionally called `.IsNil()` on `reflect.Value`s (like `string`) that do not support the method. This taught me that any debugging in this function must be highly specific.

### Investigation 3: The Targeted `panic`
-   **Method:** To gather information without breaking other tests, I added a `panic` inside the FFI wrapper (`WrapGoFunction`) that would *only* trigger if the function being called was `json.Unmarshal` and it was about to incorrectly return a `nil` object for a non-nil Go error. The condition was `if resultObjects[0] == object.NIL && !results[0].IsNil()`.
-   **Result:** The test failed, but **the panic did not trigger**.
-   **Conclusion:** This was the most critical and baffling finding. For the panic's condition to be false, given that we know `resultObjects[0]` is `object.NIL` (from the test failure), the *only possibility* is that `!results[0].IsNil()` is `false`. This means `results[0].IsNil()` must be **true**. `results[0]` is the raw `reflect.Value` returned from the `json.Unmarshal` call. This result implies that the Go FFI call itself is returning a `reflect.Value` that reports itself as nil, even though the test case is designed to force a non-nil `*json.UnmarshalTypeError`. This contradicts the expected behavior of `json.Unmarshal` and the `reflect` package.

## 3. Encountered Accidents & Miscalculations

-   **Accident:** My attempt to add generic logging to `nativeToValue` caused dozens of unrelated tests to fail due to the `reflect` panic. This was a significant diversion that cost time and highlighted the danger of broad debugging statements in type-switch functions.
-   **Miscalculation:** My core miscalculation was assuming the problem was a simple logic error in the `if/else` or `switch` statements within the FFI conversion. All evidence points to the `reflect.Value` itself having an unexpected state (`IsNil() == true`) when the bug occurs. I spent too much time trying to fix the logic based on a flawed premise, rather than questioning the premise itself.

## 4. Current Status & Remaining Task

I have exhausted my hypotheses for why a non-nil error value would result in a `reflect.Value` where `IsNil()` is true. The FFI layer appears to be the source of the problem, but the exact mechanism is unclear.

Per your instructions, I have ceased all work on fixing the bug. All my changes have been reverted.

The only remaining task is the creation and submission of this document.
