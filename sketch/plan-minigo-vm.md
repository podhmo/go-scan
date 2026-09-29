# Plan: minigo Redesign — A Lazy, Stack-VM Go Interpreter

> **Status**: merged proposal. v1 of this document proposed the stack-VM +
> lazy-import redesign; this revision folds in a second proposal ("案2") that
> sharpened several points: strict phase separation, `TRAP` vs script `Panic`,
> the package lifecycle state machine, per-function lazy compilation,
> `go list -find` as a resolver oracle, stub-package host intrinsics, and a
> `PackageProvider` abstraction. Divergences and open questions are marked.

This document proposes a ground-up redesign of `minigo` — **as a separate
`minigo2` implementation, not an in-place rewrite** — that keeps the core
philosophy (lazy, `go/ast`-driven, no eager dependency expansion) while
addressing three regrets of the v1 design:

1. **No VM** — v1 is a monolithic AST-walking evaluator (`evaluator.go`
   ~5,300 lines) where Evaluator, package cache, symbol registry and scanner
   are tightly coupled. The redesign compiles AST to bytecode for a small
   stack machine.
2. **Stdlib bindings are per-package work** — v1 requires `gen-bindings` per
   package (`stdlib/<pkg>/install.go`). The redesign makes source
   interpretation the default and shrinks the native boundary from
   "per-package bindings" to "a fixed set of runtime intrinsics".
3. **Design/maintainability** — `Parse / Index / Resolve / Initialize /
   Compile / Execute` become fully separate phases in separate packages.

`go-scan` is **not** a dependency of `minigo2`; it is an optional adapter
behind the resolver interface.

## 1. Requirements

| Requirement | Consequence |
|---|---|
| Depend on `go/ast` only | Frontend is `go/parser` → `*ast.File`. No `go/types`, no `go/packages` in the core. |
| All code must be parseable; runtime panic allowed | The compiler is a *total function* over the AST: it never rejects a construct. Unsupported features compile to `TRAP` and panic only if executed. |
| Free entry point | Execution API is `Run(Entry{Package, Function, Args})`; an entry point is just a lazy package load + index lookup + call. |
| gopls works | The implementation is an ordinary Go module; scripts are ordinary `.go` files in real modules. No magic globals, no language extensions — host extensions are real (stub) packages. |
| Per-package lazy imports | `import` records an `ImportRef` only. A package is located, parsed, indexed and initialized the first time one of its symbols is fetched at runtime. |
| Go module system works | A `PackageResolver` resolves import path → package dir + file list without expanding the import graph. |
| Macro-like features | Special forms (v1's `RegisterSpecial`: call sites receiving unevaluated `[]ast.Expr`) are preserved via the constant pool. |

## 2. Pipeline

```text
                     ┌─────────────────────┐
                     │ ordinary Go source  │
                     │ .go / go.mod/go.work│
                     └──────────┬──────────┘
                                │ go/parser (only entry point)
                                ▼
                       ┌────────────────┐
                       │   *ast.File    │
                       └───────┬────────┘
                               │ Index declarations — nothing executes
                               ▼
                    ┌────────────────────┐
                    │ PackageIndex       │
                    │ func/type/var/const│
                    │ ImportRefs         │
                    └─────────┬──────────┘
                              │ entry point lookup
                              ▼
                 ┌─────────────────────────┐
                 │ Lazy bytecode compiler  │
                 │ ast.FuncDecl -> Chunk   │  (per function, on first CALL)
                 └────────────┬────────────┘
                              ▼
                       ┌─────────────┐
                       │  Stack VM   │
                       └──────┬──────┘
                pkg.X ────────┼────────────────┐
                              ▼                │
                        PKG_GET(pkg,X)         │
                              │                │
                              ▼                │
                  ┌────────────────────┐       │
                  │ Lazy PackageLoader │       │
                  └─────────┬──────────┘       │
                            │ Resolve → Parse → Index → Initialize
                            └──────────────────┘
```

There is **no import-graph traversal**: needed edges are walked at runtime,
one package at a time. This is the essential difference from a
`go/packages`-style architecture — `go/packages`/`go/types` load transitively
because type-checking a package needs its imports' type information, which is
exactly the eagerness we avoid.

## 3. The Central Invariant

```text
unsupported syntax
    ≠ compile error
    ≠ parse error

unsupported syntax
    → TRAP opcode emitted inline
```

- **The compiler never fails on an AST node.** `go`/`select`/`chan`/
  `unsafe`-dependent constructs emit `TRAP("unsupported ChanType")` where the
  construct occurs. A function that is never called traps never; a branch
  that is never taken traps never.
- Parse is `parser.ParseFile(fset, name, src, parser.ParseComments |
  parser.SkipObjectResolution | parser.AllErrors)` — `SkipObjectResolution`
  is the recommended mode (object resolution is deprecated and unused here);
  `AllErrors` surfaces all syntax errors instead of stopping early.
- The parse ceiling is whatever Go version the host toolchain's `go/parser`
  understands.

This invariant is what makes a VM the right structure: unsupported-ness
becomes *data* (an opcode), not a walker's failure path.

## 4. Dynamic Global Resolution

The "parse/compile everything, panic only on execution" property falls out of
one rule:

> **The compiler resolves only local variables and upvalues statically.
> Every other name (package-level decls, imported package members, builtins)
> is emitted as a symbolic reference resolved at runtime.**

- `x` local → `GET_LOCAL slot`; captured or address-taken → `*Cell` +
  `GET_UPVALUE`. Loop vars follow Go 1.22+ semantics (fresh cell per
  iteration when captured).
- `x` unresolved → `GET_GLOBAL "x"`: package env → import table → builtins.
- `fmt.Println` → `GET_GLOBAL "fmt"` yields `*ImportRef`, `PKG_GET` /
  `Select "Println"` triggers the lazy lifecycle on first access.
- The compiler cannot fail on an unresolved or mistyped name — it does not
  know what the name is. Typos trap at runtime, which requirements allow.
- Function bodies never trigger package loads — imports are only paid for
  when code actually fetches a member. This preserves v1's headline feature.

## 5. Package Lifecycle

```text
Unseen → Located → Parsed → Indexed → Initializing → Ready
                                                  ↘ Failed
```

Three lazily separated levels, so that *cheap* metadata is not conflated with
*expensive* materialization:

| Stage | Cost | What it yields |
|---|---|---|
| `ImportRef` | ~0 | alias → path mapping from the file's import decl |
| `Describe`/`Locate` | cheap | `PackageMeta{Dir, Name, GoFiles, Standard, Module}` — needed to answer "package name" when basename ≠ package name (e.g. `import "…/foo/v2"` → package `foo`) |
| `Materialize` (Parse+Index) | medium | ASTs + `PackageIndex`, no execution |
| `Initialize` | first `PKG_GET` | package-var initializers + `init()` run |

```go
type ImportRef struct {
    Path         string
    ExplicitName string        // alias, "_", ".", or ""
    metaOnce     sync.Once; meta *PackageMeta
    pkgOnce      sync.Once; pkg  *Package
}
```

### Init semantics — explicitly *not* Go semantics

Real Go runs an imported package's `init()` before `main`. Here, a package's
init runs on **first actual reference**. This divergence is made explicit:

```go
type InitMode int
const (
    LazyInit         InitMode = iota // default: init on first PKG_GET
    GoCompatibleInit                  // init all imports before entry
)
```

Per import-kind behavior:

- `import "foo"` — untouched until first `PKG_GET` (under `LazyInit`).
- `import . "foo"` — on first *unresolved* identifier, advance foo to
  `Parsed`/`Indexed` (needed to answer "is `Bar` local or `foo.Bar`?"),
  but **not** `Initialize`.
- `import _ "foo"` — its only meaning is side effects: `Initialize` eagerly
  when the importing package initializes.

Circular imports: a package marked `Initializing` returns its partially
initialized env; a member still missing errors at access time only.

## 6. Index, Don't Evaluate

v1's `EvalDeclarations` becomes `IndexDeclarations` — **nothing executes**:

```go
type PackageIndex struct {
    Funcs  map[string]*Function  // *ast.FuncDecl, Chunk nil until compiled
    Types  map[string]*TypeDecl  // TypeRef, unresolved
    Vars   map[string]*VarDecl   // init expr kept as AST
    Consts map[string]*ConstDecl // expr kept as AST
}
```

`var x = expensive()` indexes as `VarDecl{InitAST: CallExpr(expensive)}`;
it evaluates only at package `Initialize`.

## 7. Compiler

- **Per-function lazy compilation**, not per-package: `Function{Decl,
  compileOnce, Chunk}` — `first CALL → compile → cache → execute`. Parsing a
  package compiles nothing. This composes perfectly with lazy packages.
- **Total coverage**: `default:` case of every node-kind switch emits
  `TRAP`.
- **Constants & positions**: literals, names, and call-site `[]ast.Expr`
  (special forms) go into `Chunk.Consts`; every `Instruction` carries
  `token.Pos` for clean stack traces.
- **Generics = monomorphize-on-use**: `Foo[int](x)` → specialization cache
  `InstanceKey{FuncID, TypeArgs}` → compile `Foo[int]`. Start with explicit
  type args; add inference from runtime argument types next (v1's heuristic);
  unhandled patterns `TRAP`. Never a parse failure.
- **Migration path**: an `OP_EVAL_AST nodeID` opcode can delegate
  not-yet-ported constructs to the v1 evaluator as a slow path — compile the
  skeleton first, port constructs incrementally, then retire the opcode (or
  keep it for debugging).

## 8. Bytecode and the Stack VM

```go
type Instruction struct { Op Op; A, B uint32; Pos token.Pos }
type Chunk struct { Code []Instruction; Consts []Value }

type Frame struct { Func *Function; IP, Base int; Defers []Value }
type VM struct { Stack []Value; Frames []Frame; Runtime *Runtime }
```

Representative opcodes:

| Category | Ops |
|---|---|
| values | `CONST`, `NIL`, `ZERO` |
| locals | `GET_LOCAL`, `SET_LOCAL`, `GET_UPVALUE`, `SET_UPVALUE` |
| globals | `GET_GLOBAL`, `SET_GLOBAL` |
| packages | `PKG_GET` |
| ops | `ADD`, `SUB`, `EQ`, `LT`, `UNARY` |
| control | `JUMP`, `JUMP_IF_FALSE`, `RANGE_NEXT` |
| calls | `CALL`, `RETURN`, `MAKE_CLOSURE` |
| composites | `MAKE_STRUCT`, `MAKE_SLICE`, `MAKE_MAP` |
| access | `GET_FIELD`, `SET_FIELD`, `INDEX`, `SET_INDEX` |
| semantics | `CONVERT`, `TYPE_ASSERT` |
| failure | `DEFER`, `PANIC`, `RECOVER`, `TRAP` (+ `OP_EVAL_AST` while migrating) |

**Calls** leave one object on the stack; multi-value returns are a `*Tuple`
unpacked by multi-assign (v1's model — simplest correct; a fixed-arity
convention is a later optimization).

**defer/panic/recover map onto Go's own mechanisms**: script `panic(x)` runs
`panic(&ScriptPanic{v: x})`; a frame-level `defer` wrapper runs pending
script-defers while unwinding; `recover()` converts an in-flight
`*ScriptPanic` to a value. Two distinct unwinding types:

```go
type Panic struct { Value Value }              // catchable by recover()
type Trap  struct { Pos token.Pos; Reason string } // bypasses recover(),
                                                  // unwinds to the VM boundary
```

A `Trap` is a VM-level failure (`unsupported select statement`), never a
script panic — `recover()` must not swallow it. Stack traces render as
`file:line in Func` chains from per-instruction `Pos`.

## 9. Value Model

Start boxed, optimize later:

```go
type Value struct { Type *Type; Data any }   // v0 — tagged/u64 repr is a
                                            //      later optimization
```

- Primitives `int64`/`float64`/`string`/`bool`/`nil`; `*Cell` for mutable
  slots; `*Struct`, `*Slice`, `*Map`, `*Pointer`, `*Closure`, `*BoundMethod`,
  `*Tuple`, `*GenericFunc`.
- **`reflect.Value` is not the core representation** — it is confined to the
  FFI boundary (`*Native` box for host/native values).
- Type identity: `(packageID, typeName)` for named types; structural identity
  for anonymous types. Interfaces satisfied by **duck typing** against lazily
  computed method sets (incl. promoted methods via embedding); assertions and
  type switches are runtime method-set checks.
- **`TypeRef` — AST type expressions stay lazy**: `NamedTypeRef{Pkg, Name}`,
  `PointerTypeRef{Elem}`, `SliceTypeRef{Elem}`, `MapTypeRef{K,V}`… Indexing
  `type Foo struct { Client *http.Client }` does **not** load `net/http`;
  resolution happens when `Foo` is instantiated or its method set is needed.

## 10. Resolution — the module system without graph expansion

```go
type PackageResolver interface {
    Locate(ctx context.Context, fromDir, importPath string, cfg BuildConfig) (*PackageMeta, error)
}
type PackageMeta struct {
    ImportPath, Name, Dir string
    GoFiles, CgoFiles     []string
    Standard              bool
    Module                *ModuleMeta
}
```

**Recommended default oracle: `go list -e -json -find <pkg>`** — `-find`
identifies a package *without resolving dependencies* (documented behavior),
so it stays within our laziness rules while giving us `Dir`, `Name`,
`GoFiles`, `CgoFiles`, `Standard`, `Module`, **and correct
GOOS/GOARCH/build-tag file selection** for free — the part a hand-rolled
resolver gets wrong (`*_windows.go`, `//go:build`, cgo file lists).

> ⚠️ **Decision needed** — repo rules currently ban `go list`. The ban's
> rationale is avoiding eager dependency expansion, which `-find` does not
> do; still, adopting it needs an explicit OK. Trade-off: requires the `go`
> toolchain at runtime (fine for a dev-tooling interpreter; a consideration
> for embedded use) and one process spawn per new package (cacheable).

Backends behind the interface:

- `resolve/go_command.go` — `go list -find` oracle (recommended default)
- `resolve/gomod.go` — pure-Go fallback: `go.work` → go.mod module path +
  require/replace via `x/mod/modfile` → `vendor/modules.txt` → `GOROOT/src`
  → `GOMODCACHE/<mod>@<ver>`; portable but must approximate build-tag file
  selection
- `resolve/goscan_adapter.go` — optional `go-scan` `locator` adapter (the
  ideas/tests transfer; the dependency does not)

## 11. Packages Come From Providers, Not From the Loader

```go
type PackageProvider interface {
    Open(ctx context.Context, meta *PackageMeta) (PackageSource, bool, error)
}
```

Provider chain: `IntrinsicProvider` → `SourceProvider` →
`OptionalNativeProvider`.

- **Source (default)**: ordinary packages, incl. `$GOROOT` stdlib — parse,
  index, execute.
- **Intrinsic**: `unsafe`, `runtime`, `minigo.dev/host`, syscall-ish host IO —
  where source cannot reach. Not package-API bindings; a **fixed set of
  runtime primitives** (`memmove`-class ops, OS boundary, `unsafe.Sizeof`)
  collapses the old `fmt/strings/json/...`-per-package binding maintenance
  into a bounded intrinsic table.
- **OptionalNative**: a real Go function bound via reflect for hot paths
  (e.g. `encoding/json`) — opt-in, core VM is unaware.

### Host extension via stub packages — the gopls-friendly pattern

No magic `Globals` map for new code (kept as convenience for embedding):
host capabilities live behind a *real* package path:

```go
import "minigo.dev/host"
func Config() string { return host.Getenv("HOME") }
```

The repo ships `package host` with stub bodies (`panic("minigo intrinsic")`),
so gopls/gofmt/goimports/rename all work on scripts; the VM intercepts
`minigo.dev/host.*` as intrinsics. Special forms work the same way
(convert-define's `define` package is the existing precedent).

## 12. API — Engine + Session

```go
engine := minigo.New(minigo.Config{
    Resolver: minigo.NewGoResolver(),
    InitMode: minigo.LazyInit,
})
session := engine.NewSession()          // shared package/init state
result, err := session.Run(ctx, minigo.Entry{
    Package: "./config", Function: "Build",
    Args: []minigo.Value{minigo.String("prod")},
})
session.Run(ctx, minigo.Entry{Package: "./config", Function: "Preview"})
engine.NewSession()                      // fresh globals for isolation
```

- Entry points stay **outside** the source — `minigo run ./app --entry Build`
  — so scripts remain perfectly ordinary Go.
- `Session` naturally extends to a REPL; `engine.NewSession()` gives a clean
  global state.
- `Result.As(&dst)` reflect-unmarshal and `Register(pkg, symbols)`/`Globals`
  port over as convenience APIs.

## 13. Package Layout

```text
minigo2/
  cmd/minigo/            CLI: run --entry, repl, gen-intrinsics
  syntax/                parse.go — go/parser wrapper (syntax.File wraps *ast.File)
  resolve/               resolver.go, go_command.go, gomod.go, goscan.go
  loader/                package.go, import.go, lifecycle.go
  index/                 package_index.go, declarations.go
  types/                 type.go, typeref.go, methodset.go
  bytecode/              opcode.go, chunk.go, disasm.go
  compile/               compiler.go, expr.go, stmt.go, func.go
  vm/                    vm.go, frame.go, call.go, panic.go, defer.go
  runtime/               value.go, heap.go, slice.go, map.go, iface.go
  builtin/               builtin.go
  intrinsic/             provider.go, host.go, runtime.go
  ffi/                   reflect.go, generated.go
  debug/                 stacktrace.go, position.go
```

## 14. Boot Sequence

```text
minigo run ./app --entry BuildConfig
    Resolve("./app") → parse app → Index → find BuildConfig
    → initialize app → compile BuildConfig → CALL
        ... PKG_GET("foo","X") → first access
            → Resolve foo → Parse foo → Index foo → Initialize foo → X
```

Every step after the entry is demand-driven.

## 15. Trade-offs and Risks

- **Runtime name resolution cost** — map lookup per global/member access;
  mitigated by per-callsite member caches, later an `OpSelect` inline cache.
  Index-resolved locals still beat tree-walking.
- **No compile-time name errors** — misspelled globals trap at runtime;
  positional stack traces mitigate.
- **`go list -find` needs the go toolchain** at runtime + process spawns
  (cacheable). The pure-Go resolver avoids both but approximates build-tag
  file selection. Flagged for decision.
- **Generics inference stays heuristic** (no `go/types`) — explicit type args
  first, inference next, unhandled patterns `TRAP`.
- **Stdlib interpretation is partial by nature** — runtime/unsafe/assembly
  leaves can't be interpreted; intrinsics + optional natives cover the
  practical subset. The boundary is documented, not hidden.
- **`go`/`select`/channels** — `TRAP` for now; the frame/stack model leaves
  room for real goroutines later (per-goroutine VM state) — out of scope.
- **Init divergence is deliberate** — `LazyInit` changes observable init
  order vs Go; `GoCompatibleInit` is offered for compatibility-sensitive
  use.
- **Imported visibility** — source-interpreted packages expose only exported
  symbols to importers (filter at `PKG_GET`); internals stay in the package
  env.

## 16. Alternatives Considered

- **In-place refactor of the tree-walker** — keeps the coupled monolith and
  delivers no VM. Rejected.
- **Adopt `traefik/yaegi`** — ships generated stdlib symbol tables and
  interprets imports from source. Rejected as primary: heavy external dep,
  its own object system and package model, and the goal is a small
  controlled core. A spike candidate only if VM effort balloons.
- **Transpile to Go + plugin/compile** — `plugin` is Linux/FreeBSD/macOS-only
  and requires exact toolchain/build-tag match (runtime crash risk per Go
  docs); loses lazy embeddability anyway. Rejected.
- **`go/types`-assisted compilation** — reintroduces eager transitive loading.
  Rejected. Editor-level type correctness is delegated to gopls; the VM's
  runtime type system stays deliberately dumber (clean responsibility split).

## 17. Phased Implementation

1. **Skeleton**: resolver + parser + `PackageIndex` — parse/index GOROOT and
   large repos *without executing*; proves the lazy pipeline.
2. **VM core**: values, frames, expressions, calls, control flow →
   `minigo run --entry hello` works.
3. **Lazy packages end-to-end**: `PKG_GET` lifecycle + init ordering +
   `TypeRef`.
4. **Composites + methods + interfaces** (method sets, duck typing).
5. **defer/panic/recover** (Panic/Trap split), stack traces.
6. **Generics-lite** (monomorphize-on-use), iterators/range-over-func as
   yield-closure calls.
7. **Intrinsic boundary + stdlib via source**, `minigo.dev/host` stubs,
   special forms, CLI/REPL.
8. **Conformance harness**: differential tests — golden `.go` files under
   `go run` vs VM; port v1 `testdata`.

Rough estimate in Devin terms: a demonstrable core (1–3) ≈ 1 session; a
solid MVP (through 7) ≈ 2–3 sessions; conformance is ongoing.

## 18. Open Questions

1. **`go list -e -json -find` as default resolver?** Conflicts with the repo's
   `go list` ban (the ban's eager-loading rationale doesn't apply to `-find`,
   but it needs explicit approval). Alternative: pure-Go resolver default +
   go-command opt-in.
2. **Location**: `minigo2/` inside go-scan (side-by-side, then swap) vs a new
   standalone repo/module? 案2 recommends separate implementation regardless;
   in-repo `minigo2/` keeps CI/tooling and is recommended here.
3. **Reuse `locator`** as the pure-Go resolver backend via adapter, or write
   `resolve/gomod.go` fresh? (locator already does the job but has a
   hand-rolled go.mod parser; `x/mod/modfile` is cleaner.)
4. **GoCompatibleInit** — needed in v2.0 or defer until requested?
5. **`OP_EVAL_AST` migration bridge** — port constructs incrementally from
   the v1 evaluator (faster MVP) vs clean-slate VM (smaller final code)?
