### Initial Prompt

"symgo: Improve Robustness and Reduce Configuration"

### Goal

To fix the `TestAnalyzeMinigoPackage` test by resolving underlying bugs in the `symgo` symbolic execution engine, making it robust enough to analyze the `minigo` interpreter without panicking. The ultimate goal is for the test to pass on a successful, error-free analysis.

### Initial Implementation Attempt

The initial focus was on fixing a series of `identifier not found` errors (`opts`, `NewInterpreter`, `varDecls`) and `panic: nil` errors (`invalid indirect`, `undefined method`, `!nil`) that were causing the analysis to fail early. This involved patching `symgo/evaluator/evaluator.go` to handle these specific cases gracefully, often by returning symbolic placeholders to allow the analysis to continue.

### Roadblocks & Key Discoveries

The initial patches fixed the surface-level errors, but this revealed a deeper issue: an infinite recursion loop causing the test to timeout. The key discovery was that `symgo` was analyzing `minigo`, another interpreter, leading to self-referential analysis. The recursion was happening as `symgo` tried to evaluate `minigo`'s own evaluation logic. A recursion-limiting check was added to `applyFunction` as a temporary measure, which successfully contained the loop and prevented timeouts. This allowed the analysis to proceed further, uncovering the next layer of errors.

### Major Refactoring Effort

The patches applied were targeted fixes to specific functions in `symgo/evaluator/evaluator.go`, including `extendFunctionEnv`, `applyFunction`, `evalReturnStmt`, `evalStarExpr`, `evalSelectorExpr`, and `evalBangOperatorExpression`. These changes made the evaluator more resilient to incomplete or unexpected states during symbolic execution.

### Current Status

The code now passes the `TestAnalyzeMinigoPackage` test, but only because the test is designed to catch a `panic`, which is still occurring (though at a different point than originally). The logs are now filled with "bounded recursion depth exceeded" warnings, confirming the recursion limiter is working but the underlying loop is not solved. The analysis is now failing with a new error: `identifier not found: e`. This error appears to originate from within the `minigo/evaluator/evaluator.go` code that `symgo` is analyzing, specifically during the evaluation of a `type switch` statement.

### References

- `docs/trouble-symgo.md`
- `docs/analysis-symgo-implementation.md`

### TODO / Next Steps

1.  Investigate the `identifier not found: e` error that occurs when `symgo` evaluates a `type switch` statement within the `minigo` evaluator source code.
2.  The investigation should focus on how `symgo`'s `evalTypeSwitchStmt` handles variable scoping, especially for closures defined within `case` blocks.
3.  Fix the scoping issue to resolve the `identifier not found: e` error.
4.  With the identifier error fixed, re-evaluate the infinite recursion loop. The `panic: nil` is likely a symptom of this loop, which needs to be broken.
5.  Once all panics and loops are resolved, update the `TestAnalyzeMinigoPackage` to expect a successful, error-free analysis instead of a panic.
