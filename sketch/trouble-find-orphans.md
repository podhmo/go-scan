# Troubleshooting: `find-orphans` Fails on `minigo/evaluator`

This document details the debugging process for a complex issue where the `find-orphans` tool failed to correctly analyze the `minigo/evaluator` package, incorrectly reporting many of its methods as orphans.

## 1. Initial Goal & Report

The user reported that `find-orphans` seemed to misidentify unexported methods as orphans, even when they were called by other exported methods within the same package. The specific example given was running the tool on `./minigo/evaluator`.

The primary goal was to fix this behavior so that intra-package calls are correctly recognized, thus preventing false positives in the orphan report.

## 2. Debugging Journey & Pivots

The path to the root cause was not linear and involved several incorrect hypotheses.

### Hypothesis 1: Exported Methods Are Not Entry Points

My initial analysis of `tools/find-orphans/main.go` revealed that the logic for identifying library entry points explicitly excluded methods (it contained a `fnInfo.Receiver == nil` check). This seemed to be the clear cause.

-   **Action**: I removed the `fnInfo.Receiver == nil` check and added a simple test case (`TestFindOrphans_intraPackageMethodCall`) with an exported method calling an unexported one.
-   **Result**: The new test case passed, as did all existing tests.
-   **Pivot**: I initially marked this as a success. However, when I ran the tool on the user's specific case (`./minigo/evaluator`), the problem persisted. This was a critical pivot point. **The initial fix was correct and necessary, but it was not sufficient.** The bug was deeper than just the entry point selection.

### Hypothesis 2: `symgo` Fails on `switch` Statements

The `minigo/evaluator.Eval` method is dominated by a large `switch` statement. My next hypothesis was that the `symgo` symbolic execution engine had a general bug in handling `switch` statements, causing it to terminate analysis prematurely.

-   **Action**: I wrote a new, targeted test (`TestFindOrphans_methodWithSwitch`) that specifically checked if a call inside a `case` block would be found.
-   **Result**: The test passed. This disproved the hypothesis.
-   **Pivot**: The problem was not a general bug with `switch` statements. It had to be something highly specific to the code within `minigo/evaluator.go`.

### Hypothesis 3: A Silent Panic or Error

At this point, direct evidence was exhausted, so I moved to indirect methods. The core mystery was why the analysis of `minigo.Eval` was stopping without any crash or error message. This suggested a silent failure.

-   **Action 1**: Add a log message (`SYMLOG`) to the very first line of `symgo`'s `Eval` method.
-   **Result 1**: When running on `./minigo/evaluator`, the log **was not printed**. This was a breakthrough. It meant the `symgo.Eval` function was never even entered for the body of `minigo.Eval`.

-   **Action 2**: Add logs to `symgo.applyFunction` to check if the function object being processed was malformed (e.g., `fn.Body == nil` or `fn.Decl == nil`).
-   **Result 2**: These logs **were not printed** either. This was confusing but meant that the code was not even reaching my checks, pointing to a panic happening *before* the checks.

-   **Action 3**: Add logs immediately before and after the call to `e.Eval` inside `symgo.applyFunction`.
-   **Result 3**: Both logs printed. This was the most confusing result. It meant `e.Eval` was called and returned without crashing, yet the first line inside it never ran. This seemed logically impossible, but strongly pointed to a `panic` being caught by an unseen `recover`.

### Hypothesis 4 (Current): A Silently Swallowed Error

The combination of the above results led to the current, most plausible hypothesis: an error is being returned during the analysis of one of the statements in `minigo.Eval`, but it is being ignored by the caller, leading to a silent termination of that analysis branch. I identified a potential error-swallowing bug in `symgo/evaluator/evaluator.go` in the `evalBlockStatement` function. I fixed it, but the user's case *still* failed.

This led to the final diagnostic step, which is the current state of the code: adding a log in the main `find-orphans` analysis loop to explicitly check the return value from `interp.Apply`. This is the most direct way to see the hidden error.

## 3. Retrospective & Key Learnings

-   **Foreseeable Obstacle**: The primary obstacle was the **silent failure** of the symbolic execution engine. An error was occurring, but it wasn't bubbling up, leading to many incorrect hypotheses. A robust analysis tool should not fail silently.

-   **What I Should Have Done Sooner**: Instead of forming complex hypotheses about *why* the analysis might be failing (e.g., `switch` statements), I should have focused earlier on proving *that* it was failing silently. **Adding the error-checking log to the main `interp.Apply` loop should have been one of the first steps** after the initial fix didn't work. It would have revealed the hidden error much earlier.

-   **Pivotal Moment**: Realizing that the log inside `symgo.Eval` was not printing, despite the logs around the call to it printing successfully. This contradictory evidence was the key that unlocked the "silent failure" line of investigation.

-   **Guidance for Future-Self**: When debugging a complex, recursive analysis tool that fails on a specific complex input but not on simple tests:
    1.  **Don't assume the bug is in the handling of a specific language feature.** First, verify the integrity of the core analysis loop.
    2.  **Instrument the top-level analysis call** (`interp.Apply` in this case) to ensure it is not returning errors silently. This is the most important step.
    3.  **Trace error propagation paths.** Ensure that errors returned from deep within the recursive evaluation are correctly bubbled up to the top. A single swallowed error can invalidate the entire analysis.
    4.  If a function call appears to happen but the code inside doesn't run, suspect a `panic`/`recover` mechanism and investigate accordingly.
