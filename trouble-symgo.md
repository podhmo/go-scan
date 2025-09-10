# Trouble: `symgo` causes infinite recursion in `minigo.Environment.Get`

This document describes an issue where the `symgo` symbolic execution engine can cause an infinite recursion, leading to a timeout.

## Symptoms

When running `symgo` on certain codebases, the process hangs and eventually times out. The logs are flooded with repeated warnings, such as:

```
level=WARN msg="expected multi-return value on RHS of assignment" \
  in_func=Get \
  in_func_pos=$HOME/ghq/github.com/podhmo/go-scan/minigo/object/object.go:1135:10 \
  exec_pos=$HOME/ghq/github.com/podhmo/go-scan/symgo/evaluator/evaluator.go:2189 \
  got_type=SYMBOLIC_PLACEHOLDER
```

The key indicators are the high volume of identical logs and the `in_func=Get` pointing to the `minigo/object.Environment.Get` method. This behavior is characteristic of an unterminated recursion during symbolic execution.

## Cause

The root cause is a lack of cycle detection in methods of `minigo/object.Environment` that traverse the chain of outer environments. The `symgo` evaluator, while analyzing Go code, can construct a graph of `Environment` objects where the `outer` field creates a circular reference (e.g., `envA.outer = envB` and `envB.outer = envA`).

The following methods are vulnerable to this issue:
- `Get(name string)`
- `GetAddress(name string)`
- `GetConstant(name string)`
- `Assign(name string, val Object)`

Each of these methods recursively calls itself on `e.outer` without tracking which environments have already been visited in the current call chain. When a cycle is present, this leads to an infinite loop, consuming CPU and eventually causing a timeout.

For example, the `Get` method's implementation was:

```go
func (e *Environment) Get(name string) (Object, bool) {
	// ... (local scope checks)
	if e.outer != nil {
		return e.outer.Get(name) // Recursive call without cycle detection
	}
	return nil, false
}
```

The fix involves adding a `visited` map to each of these traversal methods to track the environments encountered during a single lookup, thereby breaking the recursion when a cycle is detected.
