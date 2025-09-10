# `symgo`: Incorrect Handling of Multi-Return Assignments from Unscannable Packages

This document details an issue where the `symgo` symbolic execution engine fails to correctly analyze code that assigns the results of a multi-return function call when that function belongs to a package outside the current scanning policy.

## 1. Problem Description

When `symgo` encounters a function call from a package that is not part of the primary analysis scope (i.e., an "unscannable" or "out-of-policy" package), it correctly treats the call as a symbolic boundary. The `applyFunction` in `evaluator.go` returns a `*object.SymbolicPlaceholder` to represent the unknown result of this external call.

However, a problem arises when this external function returns multiple values and is used in a multi-value assignment statement (e.g., `val, err := ext.Func()`).

The `evalAssignStmt` function, which handles assignments, receives the single `SymbolicPlaceholder` from the right-hand side. It expects a `*object.MultiReturn` object in this context. When it doesn't find one, it incorrectly assumes the assignment is invalid and logs a warning: `expected multi-return value on RHS of assignment`. It then assigns a generic placeholder to each of the left-hand-side variables, losing any potential type information and failing to model the assignment correctly.

## 2. Example Log Output

The following log snippet, generated while analyzing the `find-orphans` tool, demonstrates the issue. The code being analyzed is `wf, err := modfile.ParseWork(...)`, where `golang.org/x/mod/modfile` is an unscannable external package.

```
time=2025-09-10T23:58:10.435+09:00 level=DEBUG source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:173 msg="evaluating node" type=*ast.Ident pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:90:52 source=nil

time=2025-09-10T23:58:10.435+09:00 level=DEBUG source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2618 msg="could not scan potential package for ident" in_func=discoverModules in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:168:21 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2545 ident=nil path=golang.org/x/mod/modfile error=<nil>
time=2025-09-10T23:58:10.435+09:00 level=DEBUG source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2618 msg=applyFunction in_func="<Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>" in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:90:14 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2925 in_func="<Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>" in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:90:14 exec_pos=804003 type=SYMBOLIC_PLACEHOLDER value="<Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>" args="<Symbolic: result of external call to Join>, <Symbolic: result of external call to ReadFile (result 0)>, nil"
time=2025-09-10T23:58:10.435+09:00 level=WARN source=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2618 msg="expected multi-return value on RHS of assignment" in_func=discoverModules in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/examples/find-orphans/main.go:168:21 exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2193 got_type=SYMBOLIC_PLACEHOLDER
(*object.SymbolicPlaceholder)(0x14002b15180)({
 BaseObject: (object.BaseObject) {
  ResolvedTypeInfo: (*scanner.TypeInfo)(<nil>),
  ResolvedFieldType: (*scanner.FieldType)(<nil>)
 },
 Reason: (string) (len=109) "result of calling <Symbolic: unresolved identifier ParseWork in unscannable package golang.org/x/mod/modfile>",
 UnderlyingFunc: (*scanner.FunctionInfo)(<nil>),
 Package: (*scanner.PackageInfo)(<nil>),
 Receiver: (object.Object) <nil>,
 PossibleConcreteTypes: ([]*scanner.FieldType) <nil>,
 inspectCache: (string) "",
 cacheValid: (bool) false
})
```

- `applyFunction` returns a single `SYMBOLIC_PLACEHOLDER` for the call to `ParseWork`.
- `evalAssignStmt` receives this placeholder, logs the `WARN` message, and fails to model the assignment correctly.

## 3. Root Cause and Proposed Solution

The root cause is that the symbolic execution engine does not propagate the context of the assignment (i.e., the number of expected return values) down to the function evaluation logic. The `applyFunction` returns a single placeholder because it has no information to suggest it should do otherwise.

The proposed solution is to modify `evalAssignStmt`. When it evaluates the RHS of a multi-value assignment and receives a single `*object.SymbolicPlaceholder`, it will infer the number of required return values from the number of variables on the LHS. It will then dynamically create a `*object.MultiReturn` object containing the appropriate number of new placeholders, effectively "expanding" the single placeholder to fit the assignment.

This approach is localized, safe, and correctly models the programmer's intent as expressed by the multi-value assignment syntax.
