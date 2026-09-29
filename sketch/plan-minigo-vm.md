# Plan: minigo Redesign — A Lazy, Stack-VM Go Interpreter

> **Status**: merged proposal. v1 of this document proposed the stack-VM +
> lazy-import redesign; this revision folds in two external proposals ("案2")
> that sharpened several points: strict phase separation, `TRAP` vs script
> `Panic`, the package lifecycle state machine, per-function lazy
> compilation, `go list -find` as a resolver oracle, stub-package host
> intrinsics, a `PackageProvider` abstraction — and, for special forms,
> canonical-symbol dispatch, a `SPECIAL_CALL` convention with a quote table,
> the `SpecialContext` abstraction, and partial argument evaluation.
> Divergences and open questions are marked.

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

`go-scan` supplies the **default resolver** — the user has decided to reuse
the existing lazy package locator rather than shell out to the go command.
The `PackageResolver` interface still keeps it swappable, so `minigo2` can
run without `go-scan` if a different backend is ever wanted.

The redesign can be framed as three pillars:

```text
1. Lazy Go        — never read a package/symbol until it's needed
2. Executable Go  — a stack VM executing a Go subset
3. Quoted Go      — special forms: ordinary Go syntax used as a DSL
```

## 1. Requirements

| Requirement | Consequence |
|---|---|
| Depend on `go/ast` only | Frontend is `go/parser` → `*ast.File`. No `go/types`, no `go/packages` in the core. |
| All code must be parseable; runtime panic allowed | The compiler is a *total function* over the AST: it never rejects a construct. Unsupported features compile to `TRAP` and panic only if executed. |
| Free entry point | Execution API is `Run(Entry{Package, Function, Args})`; an entry point is just a lazy package load + index lookup + call. |
| gopls works | The implementation is an ordinary Go module; scripts are ordinary `.go` files in real modules. Two invariants: *every minigo program stays a valid Go program*; *interpreter extensions are expressed as valid Go stub APIs*. |
| Per-package lazy imports | `import` records an `ImportRef` only. A package is located, parsed, indexed and initialized the first time one of its symbols is fetched at runtime. |
| Go module system works | A `PackageResolver` resolves import path → package dir + file list without expanding the import graph. |
| Macro-like features | Special forms (v1's `RegisterSpecial`) become a first-class VM call convention (`SPECIAL_CALL` + quote table) — runtime calls over quoted args, **not** AST→AST macros. |

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
- **Constants & positions**: literals, names, and quote-table entries
  (special-form call sites) go into the chunk; every `Instruction` carries
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
type Chunk struct {
    Code   []Instruction
    Consts []Value
    Quotes []QuotedCall   // special-form call sites
}

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
| calls | `CALL`, `RETURN`, `MAKE_CLOSURE`, `SPECIAL_CALL` |
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

**Decided: the default backend is go-scan.** The user wants `minigo2` to
reuse the existing lazy machinery rather than shell out to the go command.
Two complementary levels are available from `go-scan`:

- **`locator`** — import path → directory, already implementing the whole
  chain without `go list`: `go.work` → main `go.mod` (module path +
  require/replace) → `GOROOT` → `GOMODCACHE/<mod>@<ver>` (module-cache
  layout). It fills `PackageMeta{ImportPath, Name, Dir, Standard, Module}`;
  file selection needs a build-tag filter (`//go:build`, `_GOOS`/`_GOARCH`
  suffixes) added on top — the one thing the go command would do for free.
- **`goscan.Scanner`** — symbol-targeted scanning
  (`FindSymbolInPackage`): an even *lazier* option where `Materialize`
  parses only the files needed to find a requested symbol instead of every
  file in the package. Package-granular parsing is the baseline; the
  scanner path is the upgrade for very large packages.

Alternative backends behind the same interface:

- `resolve/go_command.go` — `go list -e -json -find <pkg>` oracle
  (identifies a package *without resolving dependencies*; the most accurate
  build-tag file selection, but repo rules ban `go list` and it needs the
  go toolchain at runtime — keep as opt-in only)
- `resolve/gomod.go` — a from-scratch pure-Go fallback using
  `x/mod/modfile`; only needed if `minigo2` ever leaves the go-scan repo

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

Two invariants make this concrete:

> **Every minigo program SHOULD remain a valid ordinary Go program.**
> **Interpreter extensions SHOULD be represented by valid Go stub APIs.**

Host capabilities live behind a *real* package path:

```go
import "minigo.dev/host"
func Config() string { return host.Getenv("HOME") }
```

The repo ships `package host` with stub bodies (`panic("minigo intrinsic")`),
so gopls/gofmt/goimports/rename all work on scripts; the VM intercepts
`minigo.dev/host.*` as intrinsics. No magic `Globals` map for new code (kept
as convenience for embedding). A `//go:build codegen`-style tag on DSL files
remains the *user's* choice — useful to keep DSL files out of normal builds,
never required by minigo itself.

## 12. Special Forms — "Quoted Go"

Special forms are not a side feature; they are a third pillar. The design
rule: **a special form is an execution-semantics overlay on an ordinary Go
symbol, dispatched by canonical symbol identity — not a special global
function, and not a macro.**

### 12.1 Why it matters

`convert-define` is the proof: its `define` package contains ordinary Go
stubs (`Convert(any)`, `Rule(any)`, `(*Config).Map(any,any)`), so scripts are
statically valid Go — gopls resolves `dst *destination.DstUser`, renames
work, imports are real. Meanwhile the interpreter treats
`github.com/.../define.Convert` as a *quoted call*: the `*ast.FuncLit` and
its interior (`c.Map`, `c.Compute`, `convutil.TimeToString`) are received as
syntax, never executed as Go. Typed Go syntax + source identity + AST
quotation = a type-aware DSL.

### 12.2 Dispatch by canonical identity, before materialization

```go
import d "github.com/foo/define"
d.Convert(...)
```

The compiler canonicalizes `SelectorExpr{Ident("d"), "Convert"}` through the
file's import table → `github.com/foo/define.Convert` → looks up
`SymbolID{PackagePath, Name}` in the special registry. On hit it emits
`SPECIAL_CALL`; **the `define` package itself is never loaded or even parsed
by the runtime** — the import spec alone yields the path. On miss, it falls
through to normal lazy-package dispatch:

```text
CanonicalSymbol
     ├─ special?   → SPECIAL_CALL (quoted args)
     ├─ intrinsic? → native primitive
     ├─ native?    → evaluated args + FFI
     └─ otherwise  → lazy source package (PKG_GET)
```

```go
type SymbolID struct { PackagePath string; Name string }
engine.RegisterSpecial(SymbolID{definePath, "Convert"}, handleConvert)
engine.RegisterSpecial(SymbolID{definePath, "Rule"},    handleRule)
// convenience: engine.SpecialPackage(path).Register("Convert", h).Register("Rule", h)
```

### 12.3 `SPECIAL_CALL` is runtime, not compile-time

```go
if enabled { define.Rule(foo.Convert) }
```

compiles to

```text
EVAL enabled
JUMP_IF_FALSE L1
SPECIAL_CALL special=#3 quote=#42
L1:
```

A special handler fires only when the VM *reaches* the call — never when the
compiler *sees* it. `quote=#42` indexes the chunk's quote table:

```go
type QuotedCall struct {
    Call  *ast.CallExpr
    Args  []QuotedExpr
    File  *SourceFile
    Scope ScopeID        // lexical context, incl. a handle to the caller's env
}
```

### 12.4 `QuotedExpr` carries lexical context

Bare `[]ast.Expr` (v1's API) forces handlers to re-walk `FileScope.Aliases`
and the scanner. A quote keeps the context:

```go
type QuotedExpr struct {
    Expr    ast.Expr
    FileID  FileID
    ScopeID ScopeID   // resolves locals/imports/types in the caller's scope
}
```

so that `ctx.Eval(call.Args[0])` inside `special.Do(x)` returns the value of
local `x` — the scope is a handle, not a raw `*Frame` (lifetime-safe).

### 12.5 `SpecialContext` hides the interpreter guts

v1 handlers receive `(*evaluator.Evaluator, *object.FileScope, pos, []ast.Expr)`
— convert-define reaches into `fscope.Aliases` and the scanner. New surface:

```go
type SpecialContext interface {
    Context() context.Context
    Position(ast.Node) token.Position

    File() *SourceFile
    Package() *Package

    Resolve(ast.Expr) (Ref, error)
    ResolveType(ast.Expr) (TypeRef, error)
    ResolveSymbol(ast.Expr) (SymbolRef, error)

    Eval(QuotedExpr) (Value, error)   // evaluate in the caller's scope
    Format(ast.Node) string
    Errorf(ast.Node, string, ...any) error
}
```

convert-define's alias+scanner dance collapses to
`ctx.ResolveType(param.Type)` / `ctx.ResolveSymbol(expr)`. Crucially,
`ResolveSymbol` needs only the *package index* level of laziness — a special
form quoting `huge.ConvertFoo` resolves its `SymbolRef{PackagePath, Name}`
without initializing `huge`, and without evaluating the selector (which
would materialize the package). Special forms don't break laziness; they
exploit it.

### 12.6 Partial evaluation — the actual superpower

```go
func when(ctx *SpecialContext, call *QuotedCall) (Value, error) {
    cond, _ := ctx.Eval(call.Args[0])     // evaluate arg 0
    if cond.Bool() { return ctx.Eval(call.Args[1]) } // arg 1 lazy
    return Nil, nil
}
```

Impossible in an ordinary Go call — this is what makes the mechanism a DSL
extension API and not just an FFI variant.

### 12.7 Boundaries — what it is *not*

| Kind | Fires | Example |
|---|---|---|
| `SpecialForm` | at runtime, quoted args | `define.Convert` |
| `CompilerIntrinsic` | compiled specially | `len`, `make`, `new` |
| `Macro` (AST→AST) | — **not built** | source positions, hygiene, gopls divergence — explicitly out of scope |

Same goes for `SemanticOverlay` as a model: a canonical symbol may resolve to
`Source` / `Native` / `Special` / `Intrinsic` binding kinds; the registries
stay separate (`engine.SpecialForms`, `.NativeBindings`, `.Intrinsics`),
sharing only `SymbolID` identity.

Method special forms (`MethodSymbolID{PackagePath, Receiver, Name}`, e.g.
`(*define.Config).Map`) are supported by the registry shape but **not
recommended** for convert-define-style usage: there the enclosing
`FuncLit` is data to be walked wholesale by the outer `define.Convert`
handler, not calls to dispatch individually.

## 13. API — Engine + Session

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
- `Result.As(&dst)` reflect-unmarshal, `Register(pkg, symbols)`/`Globals`,
  and `RegisterSpecial(SymbolID, handler)` port over as APIs.

## 14. Package Layout

```text
minigo2/
  cmd/minigo/            CLI: run --entry, repl, gen-intrinsics
  syntax/                parse.go — go/parser wrapper (syntax.File wraps *ast.File)
  resolve/               resolver.go, goscan.go (default), go_command.go (opt-in)
  loader/                package.go, import.go, lifecycle.go
  index/                 package_index.go, declarations.go
  types/                 type.go, typeref.go, methodset.go
  bytecode/              opcode.go, chunk.go, disasm.go
  compile/               compiler.go, expr.go, stmt.go, func.go
  vm/                    vm.go, frame.go, call.go, panic.go, defer.go
  runtime/               value.go, heap.go, slice.go, map.go, iface.go
  builtin/               builtin.go
  intrinsic/             provider.go, host.go, runtime.go
  special/               registry.go, quote.go, context.go
  ffi/                   reflect.go, generated.go
  debug/                 stacktrace.go, position.go
```

## 15. Boot Sequence

```text
minigo run ./app --entry BuildConfig
    Resolve("./app") → parse app → Index → find BuildConfig
    → initialize app → compile BuildConfig → CALL
        ... PKG_GET("foo","X") → first access
            → Resolve foo → Parse foo → Index foo → Initialize foo → X
        ... SPECIAL_CALL define.Rule → handler fires, define never loaded
```

Every step after the entry is demand-driven.

## 16. Trade-offs and Risks

- **Runtime name resolution cost** — map lookup per global/member access;
  mitigated by per-callsite member caches, later an `OpSelect` inline cache.
  Index-resolved locals still beat tree-walking.
- **No compile-time name errors** — misspelled globals trap at runtime;
  positional stack traces mitigate.
- **The go-scan resolver approximates build-tag file selection** —
  `//go:build` constraints and `_GOOS`/`_GOARCH` suffixes must be filtered
  in `resolve/` (using `go/build/constraint`, pure Go, no `go list`). The
  `go list -find` backend remains as an opt-in for environments where
  exactness matters more than the toolchain dependency.
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
- **Special-form silent divergence** — a `d.Convert` that is *meant* to be
  special but isn't registered compiles to an ordinary `PKG_GET` call into
  the stub package, whose body is `panic("minigo intrinsic")` — the failure
  is loud but late; a `minigo vet`-style checker listing unregistered stub
  calls is a cheap safety net.

## 17. Alternatives Considered

- **In-place refactor of the tree-walker** — keeps the coupled monolith and
  delivers no VM. Rejected.
- **Adopt `traefik/yaegi`** — ships generated stdlib symbol tables and
  interprets imports from source. Rejected as primary: heavy external dep,
  its own object system and package model, and the goal is a small
  controlled core. A spike candidate only if VM effort balloons.
- **Transpile to Go + plugin/compile** — `plugin` is Linux/FreeBSD/macOS-only
  and requires exact toolchain/build-tag match (runtime crash risk per Go
  docs); loses lazy embeddability anyway. Rejected.
- **AST→AST macros** — source positions, hygiene, debugging, gopls
  divergence. Rejected; `Quoted Go` covers the use cases.
- **`go/types`-assisted compilation** — reintroduces eager transitive loading.
  Rejected. Editor-level type correctness is delegated to gopls; the VM's
  runtime type system stays deliberately dumber (clean responsibility split).

## 18. Phased Implementation

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
   `SPECIAL_CALL` + `SpecialContext`, CLI/REPL.
8. **Conformance harness**: differential tests — golden `.go` files under
   `go run` vs VM; port v1 `testdata`.

Rough estimate in Devin terms: a demonstrable core (1–3) ≈ 1 session; a
solid MVP (through 7) ≈ 2–3 sessions; conformance is ongoing.

## 19. Open Questions

1. ~~**Resolver backend**~~ — **decided**: go-scan (`locator`, optionally
   `Scanner` for symbol-level laziness). `go list -find` is demoted to an
   opt-in backend; the `go list` ban stands.
2. **Location**: `minigo2/` inside go-scan (side-by-side, then swap) vs a new
   standalone repo/module? Using go-scan as default nudges toward in-repo
   `minigo2/` — recommended.
3. ~~**locator vs fresh `resolve/gomod.go`**~~ — **decided**: reuse
   `locator`/`Scanner` behind `PackageResolver`.
4. **GoCompatibleInit** — needed in v2.0 or defer until requested?
5. **`OP_EVAL_AST` migration bridge** — port constructs incrementally from
   the v1 evaluator (faster MVP) vs clean-slate VM (smaller final code)?
6. **`SpecialContext` surface** — the full interface above, or start with
   `Eval`/`Resolve`/`Format` only and grow on demand?

## 20. Phase-0 Implementation Notes (minigo2 skeleton)

Deltas and gaps discovered while building the skeleton (`minigo2/` tree).
These refine — not invalidate — the design above.

### Representations chosen

- **Pointers are cells.** Every declared variable is a `*Cell`; `&x` is the
  cell itself (`OpLocalRef`/`OpGlobalRef`). Struct literals produce `*Struct`;
  `&T{...}` wraps in a cell (`OpBox`). `*p = v` is `OpSetInd` on the cell.
- **Receiver is param slot 0**, pre-bound by the VM; `BoundMethod{Recv, Fn}`
  is created by `OpSelect` when a method name hits a `Struct`'s method set.
  Method expressions (`T.M`) return the raw `*Function`.
- **Multi-return is `*Tuple`** + `OpPack`/`OpUnpack` at call/assign sites.
- **`iota` is a hidden local** in the synthetic `__init__` chunk, reset per
  `GenDecl` spec index; const specs with no values reuse the previous spec's
  values (`Decl.Inherited`, populated at index time).
- **Bare `return` reads `Chunk.NamedSlots`** (local slots of named results).
- **Conversions are calls on `*TypeDef`**: `int(x)` is `OpGlobal "int"` →
  builtin typedef → `OpCall` → host-side `convert`. `make`/`new`/call
  position route syntactic type forms (`[]T`, `map[K]V`, `struct{...}`,
  `func(...)` — `isTypeForm`) through `typeExpr`; `chan`/`interface` forms
  trap.
- **LHS store order**: `OpSetField`/`OpSetIndex`/`OpSetInd` pop value-then-
  base(-key); since RHS is evaluated before LHS bases, `OpSwap`/`OpRot3`
  reorder the operand stack (Go leaves LHS-vs-RHS eval order unspecified).

### Deviations from the design text

- **Function literals compile eagerly** with the parent chunk — not
  compile-on-first-call. `compileOnce` laziness currently applies only to
  top-level functions. Funclits are embedded as `*Function` constants and
  become `Closure` values via `OpMakeClosure` + `UpvalDesc` (parent local
  or parent upval).
- **`defer`, `go`, `select`, channel ops, type assertions, spread calls,
  fallthrough, labels/goto** are `OpTrap` in phase-0. `defer` is common
  enough in real code that it should move up the phase list (defer→Go
  panic mapping still stands as the approach).
- **`nil` is the zero value for typed vars** (`var x T` → `nil`, not
  type-directed zero). Named-type identity is not preserved on values
  (`type MyInt int` converts pass-through).
- **Map literal keys that are identifiers are a known ambiguity**: in kv
  position an `Ident` key compiles to its name string (struct field case);
  map keys requiring identifier evaluation need typedef info and trap.
- **`parser.ParseFile(fset, name, nil, …)` footgun**: a typed-nil `[]byte`
  passed as `src` reads as an empty file — pass `any(nil)` explicitly.
- **Entry resolution**: `Engine.Package(ctx, ref)` accepts either an import
  path or a directory (`resolve.LooksLikeDir`); dir-located packages get
  synthetic paths (`<dir>` + abs) when `locator.PathToImport` fails
  (outside-module trees).
- **`&s.f`, `x[i]++`, compound-assign on non-ident targets** trap —
  they need base/key dup patterns not yet emitted.
- **`import "x/vN"` local name** falls back to the parent path element
  when basename is `vN`; basename≠package-name is otherwise still
  unresolved (cheap `PackageClauseOnly` metadata pass is the fix).

### Lifecycle note

`InitMode.LazyInit` is not implemented: `Package.Member` → `EnsureReady` →
`Bootstrap` (compiled `__init__`: var/const specs in file order, then
`init()` calls). Function bodies still compile lazily on first `CALL`,
so an imported package pays parse+index only until a member is touched —
the expensive part is avoided, per the `lazyboom` panic test.

## (end)
