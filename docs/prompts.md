# Task List

This file tracks tasks related to interactive sessions.

## Enforce Strict Scan Policy in `symgo`

- [x] Investigate `symgo` evaluator's use of `WithoutPolicyCheck` methods.
- [x] Create a design document (`docs/plan-symgo-focus.md`) outlining the problem and a proposed solution.
- [x] Experimentally validate the impact of the proposed changes by running tests.
- [x] Implement the coordinated fix to enforce the scan policy and enable hooking of unresolved function calls.
- [x] Update the design document with the final impact analysis and resolution strategy.
- [x] Update `TODO.md` to reflect the new implementation tasks.
- [ ] Update failing tests to assert for `SymbolicPlaceholder` or `UnresolvedFunction` instead of concrete values.
- [ ] Update test utility functions (e.g., call tracers) to correctly handle `UnresolvedFunction` objects.
- [ ] Add a new test to verify that method calls on out-of-policy types are correctly handled as unresolved.
