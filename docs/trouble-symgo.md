# Symgo Troubleshooting Guide

This document lists common errors encountered when running `make -C examples/find-orphans` and provides guidance on how to interpret and address them.

## Error Groups

The errors have been grouped into the following categories:

1.  [Undefined Method on Symbolic Placeholder](#group-1-undefined-method-on-symbolic-placeholder)
2.  [Identifier Not Found](#group-2-identifier-not-found)
3.  [Unsupported Unary Operator on Symbolic Placeholder](#group-3-unsupported-unary-operator-on-symbolic-placeholder)
4.  [Undefined Method on minigo.Result](#group-4-undefined-method-on-minigoresult)
5.  [Selector on Unresolved Type](#group-5-selector-on-unresolved-type)
6.  [Invalid Indirect of Unresolved Type](#group-6-invalid-indirect-of-unresolved-type)

---

### Group 1: Undefined Method on Symbolic Placeholder

**Error Messages:**
```
time=2025-09-14T00:48:50.995Z level=ERROR msg="undefined method or field: Set for pointer type SYMBOLIC_PLACEHOLDER" in_func=run in_func_pos=/app/examples/find-orphans/main.go:60:12 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=808006
time=2025-09-14T00:48:50.995Z level=ERROR msg="undefined method or field: Set for pointer type SYMBOLIC_PLACEHOLDER" in_func=run in_func_pos=/app/examples/find-orphans/main.go:60:12 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=808048
time=2025-09-14T00:48:50.996Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/find-orphans.main error="symgo runtime error: undefined method or field: Set for pointer type SYMBOLIC_PLACEHOLDER\n\t/app/examples/find-orphans/main.go:142:3:\n\t\tlogLevel.Set(slog.LevelDebug)\n\t/app/examples/find-orphans/main.go:60:12:\tin run\n\t\tif err := run(ctx, *all, *includeTests, *workspace, *verbose, *asJSON, *mode, startPatterns, excludeDirs, nil, primaryAnalysisScope); err != nil {\n\t:0:0:\tin main\n"
```

**Analysis:**
This error occurs during the symbolic execution of `examples/find-orphans/main.go`. The symgo engine encounters a method call (`logLevel.Set(slog.LevelDebug)`) on a variable (`logLevel`) that is a symbolic placeholder. A symbolic placeholder represents a value that is not known at the time of analysis, and it does not have methods like `Set`.

**Recommended Action:**
The `symgo` engine does not know how to handle the `Set` method on the `slog.LevelVar` type. To resolve this, you can register an "intrinsic" function for `slog.LevelVar.Set`. An intrinsic is a custom Go function that tells the symbolic engine how to behave when it encounters a specific function call. In this case, the intrinsic can be a no-op (a function that does nothing), which will cause `symgo` to effectively ignore the call to `logLevel.Set`.

Example of registering a no-op intrinsic:
```go
interpreter.RegisterIntrinsic("log/slog.LevelVar.Set", func(ctx context.Context, i *symgo.Interpreter, args []symgo.Object) symgo.Object {
    // Do nothing, just return nil
    return nil
})
```

---

### Group 2: Identifier Not Found

**Error Messages:**
```
time=2025-09-14T00:48:51.255Z level=ERROR msg="identifier not found: varDecls" in_func=registerDecls in_func_pos=/app/minigo/evaluator/evaluator.go:2623:26 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=1194224
time=2025-09-14T00:48:51.255Z level=ERROR msg="identifier not found: constDecls" in_func=registerDecls in_func_pos=/app/minigo/evaluator/evaluator.go:2623:26 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=1194285
time=2025-09-14T00:48:53.467Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/convert-define.main error="symgo runtime error: identifier not found: varDecls\n\t/app/minigo/evaluator/evaluator.go:2658:9:\n\t\treturn varDecls, constDecls\n\t/app/minigo/evaluator/evaluator.go:2623:26:\tin registerDecls\n\t\tvarDecls, constDecls := e.registerDecls(decls, env)\n\t/app/minigo/minigo.go:560:12:\tin EvalToplevel\n\t\tresult := i.eval.EvalToplevel(allDecls, i.globalEnv)\n\t/app/minigo/minigo.go:570:12:\tin EvalDeclarations\n\t\tif err := i.EvalDeclarations(ctx); err != nil {\n\t/app/examples/convert-define/internal/interpreter.go:87:15:\tin Eval\n\t\tif _, err := r.interp.Eval(ctx); err != nil {\n\t/app/examples/convert-define/main.go:104:12:\tin Run\n\t\tif err := runner.Run(ctx, defineFile); err != nil {\n\t/app/examples/convert-define/main.go:74:12:\tin run\n\t\tif err := run(ctx, *defineFile, *output, *dryRun, *buildTags); err != nil {\n\t:0:0:\tin main\n"
```

**Analysis:**
These errors (`identifier not found: varDecls`, `identifier not found: constDecls`) originate from the `minigo` interpreter, which is used by `symgo`. The error occurs within the `registerDecls` function, suggesting a problem with how declarations are being processed. The stack trace points to this being triggered from `examples/convert-define/main.go`. This appears to be an internal bug within the `minigo` interpreter, where it is trying to access variables that are not in scope.

**Recommended Action:**
This error likely requires a fix within the `minigo` interpreter itself. The `registerDecls` function in `minigo/evaluator/evaluator.go` seems to have a scoping issue. A developer familiar with the `minigo` codebase would need to investigate why `varDecls` and `constDecls` are not available when `registerDecls` is called.

---

### Group 3: Unsupported Unary Operator on Symbolic Placeholder

**Error Messages:**
```
time=2025-09-14T00:48:51.616Z level=ERROR msg="unary operator - not supported for type SYMBOLIC_PLACEHOLDER" in_func=evalMinusPrefixOperatorExpression in_func_pos=/app/minigo/evaluator/evaluator.go:761:10 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=0
```

**Analysis:**
This is another error from the `minigo` evaluator. It indicates that the unary minus operator (`-`) was applied to a symbolic placeholder. Symbolic placeholders do not have defined behavior for arithmetic operations.

**Recommended Action:**
This error is a consequence of performing symbolic execution on a code path that contains operations not supported for symbolic values. The best approach is to limit the scope of symbolic execution to avoid such paths. Use `symgo.WithPrimaryAnalysisScope` to specify which packages should be deeply analyzed. For other packages, `symgo` will treat function calls as black boxes and will not attempt to execute their bodies, thus avoiding this error.

---

### Group 4: Undefined Method on minigo.Result

**Error Messages:**
```
time=2025-09-14T00:48:51.744Z level=ERROR msg="undefined method: Value on github.com/podhmo/go-scan/minigo.Result" in_func=As in_func_pos=/app/examples/docgen/loader.go:53:12 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=923217
time=2025-09-14T00:48:51.744Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/docgen.main error="symgo runtime error: undefined method: Value on github.com/podhmo/go-scan/minigo.Result\n\t/app/minigo/minigo.go:351:19:\n\t\treturn unmarshal(r.Value, dstVal.Elem())\n\t/app/examples/docgen/loader.go:53:12:\tin As\n\t\tif err := result.As(&configs); err != nil {\n\t/app/examples/docgen/loader.go:25:9:\tin LoadPatternsFromSource\n\t\treturn LoadPatternsFromSource(configSource, logger, scanner)\n\t/app/examples/docgen/main.go:111:9:\tin LoadPatternsFromConfig\n\t\treturn LoadPatternsFromConfig(filePath, logger, scanner)\n\t/app/examples/docgen/main.go:74:25:\tin loadCustomPatterns\n\t\tcustomPatterns, err := loadCustomPatterns(patternsFile, logger, s)\n\t/app/examples/docgen/main.go:49:12:\tin run\n\t\tif err := run(logger, format, patternsFile, entrypoint, extraPkgs); err != nil {\n\t:0:0:\tin main\n"
```

**Analysis:**
This error occurs in `examples/docgen/main.go`. The code attempts to call the `Value` method on an object of type `minigo.Result`. The error message indicates that the `minigo.Result` struct does not have a `Value` field or method. This is likely due to a breaking change in the `minigo` API between versions.

**Recommended Action:**
The code in `examples/docgen/loader.go` and `minigo/minigo.go` needs to be updated to conform to the new API of `minigo.Result`. A developer should inspect the current definition of the `minigo.Result` struct and update the calling code accordingly. This is a required code modification.

---

### Group 5: Selector on Unresolved Type

**Error Messages:**
```
time=2025-09-14T00:48:51.823Z level=ERROR msg="expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE" in_func=run in_func_pos=/app/examples/convert/main.go:91:12 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=239074
time=2025-09-14T00:48:51.823Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/convert.main error="symgo runtime error: expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE\n\t/app/examples/convert/main.go:166:3:\n\t\tos.Stdout.Write(formatted)\n\t/app/examples/convert/main.go:91:12:\tin run\n\t\tif err := run(ctx, *pkgpath, *workdir, *output, *pkgname, *outputPkgPath, *dryRun, *inspect, logger, *buildTags); err != nil {\n\t:0:0:\tin main\n"
time=2025-09-14T00:48:51.839Z level=ERROR msg="expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE" in_func=run in_func_pos=/app/examples/deps-walk/main.go:94:12 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=460798
time=2025-09-14T00:48:51.839Z level=ERROR msg="symbolic execution failed for entry point" function=github.com/podhmo/go-scan/examples/deps-walk.main error="symgo runtime error: expected a package, instance, or pointer on the left side of selector, but got UNRESOLVED_TYPE\n\t/app/examples/deps-walk/main.go:269:12:\n\t\t_, err = os.Stdout.Write(finalOutput.Bytes())\n\t/app/examples/deps-walk/main.go:94:12:\tin run\n\t\tif err := run(context.Background(), startPkgs, hops, ignore, hide, output, format, granularity, full, short, direction, aggressive, test, dryRun, inspect, logger); err != nil {\n\t:0:0:\tin main\n"
```

**Analysis:**
This error occurs in both `examples/convert/main.go` and `examples/deps-walk/main.go`. It indicates that the symbolic execution engine is trying to access a field or method on a type that it could not resolve. This often happens when a type is defined in a package that was not properly scanned or is otherwise unavailable. For example, the error in `convert/main.go` happens when calling `os.Stdout.Write`. This suggests that the `os` package, or specifically `os.Stdout`, has not been correctly resolved.

**Recommended Action:**
Ensure that all necessary dependencies are correctly configured in the `symgo` scanner. The `symgo.WithSymbolicDependencyScope` or `goscan.WithDeclarationsOnlyPackages` options should be used to tell the scanner to parse the declarations of external packages (like the `os` package). This will make the type information available to the symbolic execution engine and allow it to resolve the types correctly.

---

### Group 6: Invalid Indirect of Unresolved Type

**Error Messages:**
```
time=2025-09-14T00:48:52.062Z level=ERROR msg="invalid indirect of <Unresolved Type: strings.Builder> (type *object.UnresolvedType)" in_func=Install in_func_pos=/app/examples/minigo/main.go:31:2 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=1440638
time=2025-09-14T00:48:52.062Z level=ERROR msg="invalid indirect of <Unresolved Type: strings.Reader> (type *object.UnresolvedType)" in_func=Install in_func_pos=/app/examples/minigo/main.go:31:2 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=1440706
time=2025-09-14T00:48:52.062Z level=ERROR msg="invalid indirect of <Unresolved Type: strings.Replacer> (type *object.UnresolvedType)" in_func=Install in_func_pos=/app/examples/minigo/main.go:31:2 exec_pos=/app/symgo/evaluator/evaluator.go:2741 pos=1440773
... and many more for types in fmt, encoding/json, strconv ...
```

**Analysis:**
This large group of errors originates from `examples/minigo/main.go`. The `Install` function seems to be attempting to perform an indirection (e.g., dereferencing a pointer) on a number of standard library types that are unresolved. This is a type resolution problem within the symbolic execution engine.

**Recommended Action:**
This is a more severe version of the type resolution problem seen in Group 5. The symbolic execution of `examples/minigo/main.go` requires a large number of standard library packages. The `symgo` scanner must be configured to include all of these packages (e.g., `strings`, `fmt`, `encoding/json`, `strconv`) in its scope. Using `goscan.WithDeclarationsOnlyPackages` for these standard library packages is the recommended approach to make their type information available without incurring the high cost of a full symbolic execution of the standard library.
