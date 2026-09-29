# Plan: minigo Redesign — A Lazy, Stack-VM Go Interpreter

This document proposes a ground-up redesign of `minigo`. It keeps the core
philosophy of the current implementation (lazy, `go/ast`-driven, no
`go/packages`/`go/types`/`go list`) while addressing three regrets of the v1
design:

1. **No VM** — v1 is a monolithic AST-walking evaluator (`evaluator.go` ~5,300
   lines). The redesign compiles AST to bytecode for a small stack machine.
2. **Stdlib bindings are per-package work** — v1 requires running
   `gen-bindings` per package (`stdlib/<pkg>/install.go`). The redesign makes
   source interpretation the default and shrinks the native boundary to a
   handful of primitives.
3. **Design/maintainability** — the new code is split into small,
   single-purpose packages instead of one giant evaluator.

## 1. Requirements

| Requirement | Consequence |
|---|---|
| Depend on `go/ast` only | Frontend is `go/parser` → `*ast.File`. No `go/types`, no `go/packages`, no `go list`. |
| All code must be parseable; runtime panic allowed | The compiler is a *total function* over the AST: it never rejects a construct. Unsupported features compile to `OpUnsupported` and panic only if executed. |
| Free entry point | Execution API is `Call(pkgPath, funcName, args)`; an entry point is just a lazy package load + symbol lookup + call. |
| gopls works | The implementation is an ordinary Go module. Scripts are ordinary `.go` files inside real modules, so gopls/gofmt/goimports work on them unchanged. |
| Per-package lazy imports | `import` records an alias→path mapping only. A package is located, parsed and compiled the first time one of its symbols is selected at runtime. |
| Go module system works | Resolution walks `go.work` → main `go.mod` (module path + require + replace) → `vendor/` → `GOROOT` → `GOMODCACHE`, without shelling out to `go list`. |
| Macro-like features | Special forms (v1's `RegisterSpecial`: call sites that receive unevaluated `[]ast.Expr`) are preserved via the constant pool. |

## 2. The Key Decision: Dynamic Global Resolution

The property "parse/compile everything, panic only on execution" falls out of
one design choice:

> **The compiler resolves only local variables and upvalues statically.
> Every other name (package-level vars/funcs/types, imported package members,
> builtins) is emitted as a *symbolic* reference resolved at runtime.**

- `x` as a local → `OpLoadLocal(slot)`. A local escapes (is captured or
  addressed) → it becomes a cell: `OpLoadUpval(idx)` / heap `*Cell`.
- `x` unresolved by the function's own scope analysis → `OpLoadGlobal("x")`:
  looked up at runtime in package env → file import table → builtins.
- `fmt.Println` → `OpLoadGlobal("fmt")` yielding a `*LazyPackage` (from the
  file's import table), then `OpSelect("Println")`, which triggers the lazy
  load on first access.

Consequences:

- The compiler cannot fail on an unresolved or mistyped name — it does not
  know what the name is. Typos panic at runtime, which the requirements
  explicitly allow.
- Function bodies never trigger package loads; an import is only paid for
  when code actually selects a member. This preserves v1's headline feature.
- Per-file import semantics come free: each file contributes its own
  alias→path table, including `.` and `_` imports and aliases.

The compiler is therefore a small, total AST→bytecode translator with one
job per node kind — this is where most of v1's incidental complexity
disappears.

## 3. Architecture

```
source ──go/parser──> *ast.File ──compiler──> *Func{Code []Instr, Consts, NLocals, UpvalDescs}
                                               │
                     Loader ──> *LazyPackage ──select──> compile decls ──> pkg env
                                               │
                                    VM: operand stack + frames + defer stack
```

### 3.1 Package layout

```
minigo2/                  (new directory, side-by-side with minigo/ during migration)
  minigo.go               public API: Run, Interpreter, Call, Result.As
  compiler/               AST -> bytecode. compile_expr.go, compile_stmt.go,
                          scope.go (locals/upvalues analysis)
  vm/                     instr.go (opcodes), machine.go (dispatch loop,
                          frames, defer/panic machinery)
  value/                  value model, runtime type descriptors, method sets
  loader/                 lazy package objects, import tables, init ordering
  resolve/                import path -> directory (module resolution)
  gostd/                  stdlib shims written in plain .go source
  natives/                native function registry (the syscall boundary)
  cmd/minigo/             CLI: run -pkg -fn, repl, gen-natives
```

Everything is plain Go in the existing module; nothing obstructs gopls.

### 3.2 Compiler

- **Scope analysis per function**: declare/define locals into slots; a
  variable that is captured by an inner function or has its address taken is
  promoted to a cell. Loop variables follow Go 1.22+ semantics — a fresh cell
  per iteration when captured.
- **Total coverage**: every `ast.Node` kind emits code. `go`/`select`/`chan`/
  `unsafe`-dependent constructs emit `OpUnsupported(feature)` inline — a
  runtime panic exactly where the construct is reached, never a compile error.
- **Constants & positions**: literals, names, and call-site `[]ast.Expr`
  (for special forms) go into the function's `Consts`; a parallel line table
  maps ip → `token.Pos` for stack traces.
- **Package compilation**: on first touch of a package, all its files are
  parsed and all decls compiled. `var` initializers and `init()` run at that
  moment (lazy init — the package's own imports still resolve lazily when
  referenced). Circular imports: a package marked "loading" returns its
  partially-initialized env; a member that is still missing errors at access
  time only.

### 3.3 Bytecode and the stack machine

- `Instr` is a fixed-size struct `{Op, A, B int32}` (or packed `uint64`).
  `Func{Code []Instr, Consts []Value, NLocals int, Upvals []UpvalDesc}`.
- Frame: `{fn *Func, ip, bp int, defers []deferred}`; one operand stack
  shared by frames (single-threaded, as today).
- Dispatch is a `for { switch op }` loop. Representative op set:

  ```
  OpLoadLocal/OpStoreLocal, OpLoadUpval/OpStoreUpval, OpAllocCell
  OpLoadGlobal(nameIx), OpSelect(symIx), OpIndex, OpStoreIndex
  OpLoadConst, OpPop, OpDup
  OpBinary(op)/OpUnary(op)         // dynamic per-op dispatch
  OpJump/OpJumpIfFalse, OpRangeNext
  OpMakeSlice/OpMakeMap/OpMakeStruct/OpMakePointer
  OpCall(argc, astIx), OpCallMethod, OpReturn
  OpDefer, OpPanic, OpRecover
  OpUnsupported(featureIx)
  ```

- **Calls**: a call always leaves one result object on the stack; multi-value
  returns are a `*Tuple` unpacked by multi-assign (same model as v1, simplest
  correct option; a fixed-arity calling convention is a later optimization).
- **defer/panic/recover are mapped onto Go's**: `panic(x)` executes
  `panic(&ScriptPanic{v: x})`. Each frame's dispatch runs its registered
  script-defers in a Go `defer` wrapper while unwinding; `recover()` converts
  an in-flight `*ScriptPanic` back into a normal return value. This reuses
  Go's unwinding instead of hand-rolling longjmp — the cheapest correct
  implementation.
- **Special forms**: a call site stores its raw argument ASTs in `Consts`.
  `OpCall` checks the callee: if it is a `*SpecialForm`, it is invoked with
  the unevaluated `[]ast.Expr` instead of evaluated args. Identical semantics
  to v1's `RegisterSpecial`; convert-define-style macros keep working.

### 3.4 Value model

Start boxed (`type Value = any`), optimize later:

- primitives: `int64` (all ints), `float64`, `string`, `bool`, `nil`
- `*Cell` (mutable slot for pointers/addressed/captured vars)
- `*Struct{Def *TypeDef, Fields []Value}`, `*Slice{Elems []Value}`,
  `*Map{Pairs}`, `*Pointer{Cell *Cell}`
- `*Closure{Fn *Func, Upvals []*Cell}`, `*BoundMethod`, `*Tuple`
- `*TypeDef` — runtime type descriptors (name, kind, fields, lazy method set
  including promoted methods through embedding). Named types get identity;
  interfaces are satisfied by **duck typing**: a value satisfies an interface
  iff its (lazy) method set covers the interface's. Assertions and type
  switches are runtime checks on method sets — no static type world needed.
- `*GenericFunc{TypeParams, Fn}` — instantiation at call time: type args
  inferred from runtime argument types when not explicit (same heuristic as
  v1; documented limitation).
- `*Native{reflect.Value}` — the v1 `GoValue` equivalent for host-injected
  values and native-call results. Field/method access goes through reflect;
  conversions between `Value` and `reflect.Value` reuse the v1
  marshal/unmarshal logic.

### 3.5 Loader and resolver

- `Resolver` interface: `ResolveDir(importPath) (dir string, err error)`.
  Resolution order:
  1. `go.work` workspace modules
  2. main module: `module` path prefix → `rootDir/sub`
  3. `replace` directives (local and versioned), parsed with
     `golang.org/x/mod/modfile` (already a repo dependency) rather than the
     hand-rolled line parser in `locator`
  4. `vendor/modules.txt` when a vendor dir exists
  5. `GOROOT/src/<path>` for stdlib
  6. `GOMODCACHE/<mod>@<ver>/<sub>` from the main go.mod's require set
     (module-graph pruning means requires already list transitives)
  7. cache miss → error suggesting `go mod download` (opt-in auto-download
     flag; `go mod download` is not `go list`, but implicit shelling-out is
     still opt-in by default)
- Reuse of the existing `locator` package behind `Resolver` is the pragmatic
  path (it already does all of this without `go list`, using only `go env`);
  a fresh `resolve/` package is the independent path. Recommended: wrap
  `locator` first, swap later if desired — go-scan dependency stays optional
  either way.
- `*LazyPackage{Path}`: created from a file's import table; first `OpSelect`
  triggers locate→parse→compile→init→member lookup, cached per path.

### 3.6 Stdlib strategy — fixing the binding pain

Three tiers, tried in order per package:

1. **Source interpretation (default)**: load `$GOROOT/src/<pkg>` like any
   other package. Works for the pure-Go majority (`strings`, `bytes`, `sort`,
   `slices`, `errors`, `path`, `strconv` mostly…). An uninterpretable leaf
   (`unsafe`, assembly internals) panics only if actually called.
2. **Go-source shims**: `gostd/` ships ordinary `.go` files implementing
   stdlib-compatible APIs over a small native core — written in Go, readable
   by gopls, no codegen. Use where real stdlib source is too gnarly (e.g. a
   simplified `fmt`, `strings.Builder` internals).
3. **Natives**: `func(*VM, []Value) []Value` registered per symbol — the true
   syscall boundary (file I/O, `os.Getenv`, `time.Now`, printing). Optional
   `gen-natives` emits **one** generated file for a user-chosen package list,
   replacing v1's per-package `install.go`.

Net effect: bindings exist only at the boundary where interpretation cannot
reach; adding a stdlib package is usually "it just works" or "write a `.go`
shim", not "write a binding".

### 3.7 Interop surface (unchanged concepts, new internals)

- `Interpreter.Globals`/`Register` — reflect-wrapped host values/functions.
- `Result.As(&dst)` — reflect unmarshal (port v1's `unmarshal`).
- `RegisterSpecial` — macro-style special forms (see §3.3).
- Entry points: `interp.Call(ctx, "import/path", "Func", args...)`;
  `main.main` by default; methods reachable as `pkg.Type.Method` where
  unambiguous.
- CLI: `minigo run -pkg ./dir -fn Func`, `-eval`, `repl`, `gen-natives`.

### 3.8 Special forms and gopls

Scripts calling special forms stay gopls-clean the way convert-define already
does it: the DSL's import path resolves to a real Go package containing no-op
stub signatures, so gopls type-checks the script while the VM dispatches the
call to the registered special form.

## 4. Requirement → Mechanism Map

| Requirement | Mechanism |
|---|---|
| go/ast only | `go/parser` + `*ast.File`; bytecode, no type-checker |
| parse everything / panic at runtime | total compiler + `OpUnsupported`; dynamic global resolution |
| free entry point | lazy package load + `Call(path, name, args)` |
| gopls | plain module, `.go` scripts, stub packages for DSLs |
| lazy per-package import | import table → `*LazyPackage` → load on first `OpSelect` |
| module system | `Resolver` chain over `x/mod/modfile`; no `go list` |
| less binding work | source-interpret → `.go` shims → small native boundary |
| stack VM | compile AST → `[]Instr`; frames + operand stack |
| macros | call-site ASTs in `Consts`; `*SpecialForm` natives |

## 5. Trade-offs and Risks

- **Speed vs. laziness**: runtime name resolution costs a map lookup per
  global/member access. Mitigation: per-callsite member cache on packages,
  and later an inline cache on `OpSelect`. A stack VM with index-resolved
  locals still beats tree-walking.
- **No compile-time errors**: misspelled globals panic at runtime. Accepted
  per requirements; positional stack traces mitigate debugging cost.
- **Generics inference stays heuristic** (no `go/types`): explicit
  `[T](args)` works; inference from runtime arg types covers common cases;
  documented as a limit, same as v1.
- **Stdlib interpretation is partial by nature**: packages tight to
  runtime/unsafe/assembly can't be fully interpreted; the shim+native tiers
  cover the practical subset. The boundary is documented, not hidden.
- **`go`/`select`/channels**: compile to `OpUnsupported` — parseable, panics
  if reached. (If wanted later, real goroutines are feasible on this VM since
  frames/stacks are per-goroutine objects; out of scope for v2.)
- **Member visibility**: source-interpreted packages should expose only
  exported symbols to importers (filter at `OpSelect`), while internal decls
  stay visible inside the package's own env.

## 6. Alternatives Considered

- **Keep the tree-walker, refactor `evaluator.go`** — does not deliver the VM
  and keeps the monolith's resolution complexity. Rejected.
- **Adopt `traefik/yaegi`** — it already ships generated stdlib symbol tables
  and interprets imports from source. Rejected as primary: heavy external
  dependency, its own object system and eager-ish package model, and the
  goal here is a deliberately small core we control. Worth a spike only if
  the VM effort balloons.
- **Transpile to Go + compile/plugin** — needs a toolchain at runtime and
  loses the lazy, embeddable character. Rejected.
- **`go/types`-assisted compilation** — banned by repo rules (eager imports).

## 7. Phased Implementation

1. **Skeleton**: value model, compiler for expressions/functions/control
   flow, VM core, `OpLoadGlobal` + `*LazyPackage` + `Resolver` →
   `minigo run -fn hello file.go` works.
2. **Composites**: struct/slice/map/pointer, methods, field/index assign.
3. **Interfaces + generics-lite**: method sets, duck typing, instantiation.
4. **defer/panic/recover** via Go-panic mapping; stack traces.
5. **gostd shims + native boundary + `gen-natives`**.
6. **Special forms, CLI, REPL.**
7. **Conformance harness**: golden `.go` files run under both `go run` and
   the VM (differential tests); port v1 `testdata` cases.

Rough estimate in Devin terms: a demonstrable core (phases 1–2) ≈ 1 session;
a solid MVP (through 5) ≈ 2–3 sessions; conformance polish is ongoing.

## 8. Open Questions

1. Land as `minigo2/` side-by-side (recommended: compare during migration,
   then swap) or replace `minigo/` in place?
2. Reuse `locator` behind `Resolver` (recommended now) vs. a fresh
   standalone `resolve/` package?
3. Opt-in `go mod download` on cache miss — acceptable, or strict error only?
4. Range-over-func iterators (`for f := range seq`): keep desugaring to a
   yield-closure call as v1 does, or defer to phase 3+? (Recommend keeping —
   it compiles to an ordinary function call.)
