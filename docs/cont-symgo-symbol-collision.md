### Initial Prompt

Read one item from `TODO.md` and implement it. If necessary, break it down into sub-tasks. After breaking it down, you may write it in `TODO.md`.

Then, please proceed with the work. Please also add test code. Continue to modify the code until the tests succeed. After finishing the work, be sure to update `TODO.md` at the end.

Please work on a `symgo` task. Looking at part of the command when running `make -C examples/find-orphans e2e`, it seems there is confusion between `main` and `run`. The following is the stack trace when the ERROR occurred, and it seems that after analyzing the `run()` function of `deps-walk`, it ended up analyzing the code of `find-orphans` (this should probably have been called from `find-orphans.run()`).

```
time=2025-09-11T01:24:32.001+09:00 level=INFO msg="found main entry point(s), running in application mode" count=9
time=2025-09-11T01:24:32.008+09:00 level=WARN msg="could not scan package, treating as external" in_func=discoverModules in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:168:21 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:1423 package=golang.org/x/mod/modfile error="ScanPackageByImport: could not find a module responsible for import path \"golang.org/x/mod/modfile\""
time=2025-09-11T01:24:32.008+09:00 level=ERROR msg="identifier not found: keys" in_func=analyze in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:272:9 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2636 pos=815254
time=2025-09-11T01:24:32.008+09:00 level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/deps-walk.main error="symgo runtime error: identifier not found: keys
	$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:438:20:
		patternsToWalk := keys(a.scanPackages)
	$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:272:9:	in analyze
		return a.analyze(ctx, asJSON)
	$HOME/ghq/github.com/podhmo/go-scan/examples/deps-walk/main.go:94:12:	in run
		if err := run(context.Background(), startPkgs, hops, ignore, hide, output, format, granularity, full, short, direction, aggressive, test, dryRun, inspect, logger); err != nil {
	:0:0:	in main
```

Please investigate the cause. And output the result in English in `docs/trouble-symgo.md`.

Points of concern:
- The stack trace included in the log displayed by `find-orphans/main.go` is strange (symbolic execution failed for entry point)
- The identifier `keys` is not found (the analysis target is `find-orphans/main.go:438`)

Even if you cannot complete the task, please add it to `TODO.md`.

### Goal

The main goal is to fix a bug in the `symgo` symbolic execution engine that causes a crash when running `make -C examples/find-orphans e2e`. The crash is due to a symbol collision between different `main` packages being analyzed.

### Initial Implementation Attempt

My initial approach was to isolate the bug by creating a new test case that specifically reproduced the symbol collision. I hypothesized that the `symgo` evaluator was using a single, global environment for all packages, causing symbols to overwrite each other. My first implementation attempt involved creating a new test file, `symgo/integration_test/cross_main_package_test.go`, to prove this hypothesis.

### Roadblocks & Key Discoveries

The primary roadblock was correctly setting up the test harness (`scantest`) for a multi-module test workspace. This led to several failed test runs due to scanner initialization errors and incorrect import paths. The key discovery was that the `evalFile` function in `symgo/evaluator/evaluator.go` was indeed falling back to a global environment instead of using a package-specific one. This confirmed my initial hypothesis and explained why the `run` function from one `main` package was overwriting the `run` function from another.

### Major Refactoring Effort

Based on the discovery, I performed a major refactoring of the `symgo/evaluator/evaluator.go` file. The core change was to introduce a `pkgCache` map within the `Evaluator` struct. This cache stores a separate, isolated environment for each package being analyzed. I modified the `evalFile` function to always use the package-specific environment from this cache, ensuring that symbols from different packages would no longer collide.

### Current Status

The core refactoring in `evaluator.go` is complete. However, this change caused a large number of existing tests in the `symgo/evaluator` and `symgo` packages to fail, as they were implicitly relying on the old, incorrect behavior of a shared global environment. I have been methodically fixing these test failures by updating them to look up symbols in the correct package-specific environment. The vast majority of tests in `symgo/evaluator` and `symgo` are now passing. The next technical hurdle is to fix the remaining test failures and then run the full test suite.

### References

- `docs/dev-symgo.md`
- `docs/dev-scantest.md`

### TODO / Next Steps

1.  Fix the remaining test failures in `symgo/evaluator` and `symgo` packages.
2.  Run the full test suite for the project, including `go test ./...` and `make -C examples/find-orphans e2e`, to ensure no regressions were introduced.
3.  Once all tests pass, finalize the changes for submission.
