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

## 21. Round-2 notes: what the first review pass exposed

These came out of fixing the skeleton's first review findings; they are the
parts the design text had left implicit.

- **The `__init__` chunk mixes decls from several files, so "the current
  file scope" cannot come from `Function.File`.** Recover the file per
  instruction from `Pos` (`fset.PositionFor(pos).Filename` →
  `Package.FileByName`). Every instruction already carries a decl position,
  so imports in package-level initializer expressions resolve against the
  correct file's import table with no extra machinery.
- **Package init order is dependency order, not file order.** `var B = A+1`
  before `var A = 1` must initialize `A` first. Phase-0 topologically sorts
  specs by free identifiers in their value expressions; transitive deps
  through function bodies (`var x = f()` where `f` reads `var y`) are a
  documented gap — a full dep analysis needs func-body reachability.
- **Imported vars are Cells; member access must unwrap them.** `pkg.X+1`
  must not operate on the `*Cell` itself. The same unwrap rule applies to
  `OpSetGlobal`/`OpGlobalRef` targets.
- **`for i := range s` (single var) yields the key/index** — not the
  element. Two-var form yields key+elem. Getting this backwards silently
  changes semantics.
- **`:=` inside the same block assigns rather than redeclaring.** `x, y :=`
  where `x` exists in the innermost block must reuse `x`'s cell (closures
  capturing `x` see the update); only names absent from the current block
  get fresh slots.
- **Struct values copy on every store** (param bind, `OpNewLocal`,
  `OpSetLocal`, `OpNewGlobal`, `OpSetGlobal`). `b := a` without a copy
  leaks mutations back through `a`. Slices/maps/pointers share as in Go.
- **Pointer receivers need an addressable cell.** Method selection wraps a
  non-cell receiver in a fresh `Cell` for `PtrRecv` methods; value
  receivers get a struct copy. This distinction lives on
  `runtime.Function.PtrRecv`, set from `*ast.StarExpr` in the receiver.
- **Visibility enforcement point**: `selectMember` (the `x.y` path) checks
  `token.IsExported` for `ImportRef`/`Package` bases — engine entry-point
  calls intentionally bypass it (`main.main` is unexported).
- **A directory `Run`/`Package` ref can read and execute any reachable Go
  tree** — by design for a local interpreter, but it is a limitation for
  embedding/hosted use. Candidate knob: `resolve` option
  `AllowedRoots []string` checked in `LocateDir`. Left to the maintainer
  (tracked in TODO.md).

## 22. Round-3 notes: defer/recover, the concurrency approximation, and the second review pass

What implementing this phase's TODO items revealed that the design text
left implicit.

### Panic/defer model

- **The recoverable channel is `v.inflight`, not a frame field.** Unwind
  sets it for the duration of a frame's defer drain and restores the outer
  value afterwards, so a nested call's recover() cannot see an outer
  frame's panic — matching Go's "innermost deferred function" rule.
- **Named results are gathered after defers whenever the function declares
  them** — including on panic unwind, where no `OpReturn` ever ran: a
  recovering defer can still assign them. Unnamed results remain the
  snapshot taken at return-expression evaluation.
- **Deferred callees are not limited to compiled functions.** `defer
  close(ch)` and `defer recover()` are legal Go: a callee without a
  bytecode frame (BuiltinFunc, TypeDef conversion, Cell-wrapped) runs
  directly at teardown under a sentinel frame marked `deferred`, pushed
  purely so `recover()` still sees the call as deferred.
- **Script panics inside a deferred call supersede the panic being
  unwound but do not skip the remaining defers**; a Trap or host panic
  aborts the drain. `Panic`/`Trap` accumulate `Frames` (`name at
  file:line`) during unwind — a best-effort stack trace, added after the
  design text and deliberately not spec-level fidelity.

### Concurrency approximation (documented, single-threaded)

- **Channels are unbounded queues.** `go f()` runs f synchronously and
  propagates its panic immediately. Ops that would block forever trap
  instead of deadlocking: `recv` on an empty open channel, `select` with
  no ready case and no default.
- **`case <-ch` still consumes.** A non-binding receive pops the value —
  the review caught it leaving the value queued.
- **Select operand evaluation is spec order, which forces a two-pass
  compile.** Every case's channel operand (and a send case's value) is
  evaluated exactly once in source order on entry — stash into `$selN`
  temp slots first, then dispatch first-ready-wins. Compiling operands
  inline under the readiness jump lets a winning earlier case skip later
  operands' side effects.

### FFI / intrinsics

- **`Value` is `any` — `case runtime.Value` in a type switch matches
  everything.** Marshalling helpers must enumerate the concrete runtime
  types first; anything else (errors, host structs, `time.Time`) boxes as
  `*runtime.GoValue`, and `selectMember` dispatches methods on it
  reflectively so `err.Error()` works.
- **`goNative` marshals by copy — mutating intrinsics cannot use it.**
  `sort.Ints`/`slices.Sort` must sort `*runtime.Slice.Elems` in place; the
  `h.fn` boundary is read-only by construction.
- **`bindCompiles` must skip eagerly-compiled protos.** Literal
  `*Function` consts arrive with `Chunk` set and `Decl` nil; attaching
  `Compile` hooks them into `compile.Func`'s nil-Decl deref on first
  call. Hook only where `Decl != nil && Chunk == nil`.

### Lifecycle & embedding

- **A failed initializer must not be masked by partial globals.** `Member`
  short-circuits on `State == Failed` before serving `Globals` — earlier
  versions returned the half-populated value.
- **`AllowedRoots` implies a smaller host surface.** With roots set, the
  resolver rejects dirs outside (symlink-resolved both ways — a root
  containing a link to the outside otherwise bypasses the check) and the
  `os` intrinsic drops `Getenv`/`Args`; `os.Exit` always traps in every
  mode, since an interpreted program must never terminate the host.
- **Init-order analysis through function bodies** is a memoized
  transitive closure over `Index.Funcs` + method decls (depth-capped),
  folded into the existing spec topo sort — `var x = f()` waits on every
  package-level name `f` transitively reads.

## 23. Round-4 notes: references, interfaces, generics-lite, special forms

What implementing the remaining TODO items revealed that the design text
left implicit.

### References generalize the cell model

- **"Pointer" is a protocol, not a type.** `runtime.Deref`/`SetRef` work
  over `*Cell`, `*FieldRef` (`&s.f`), and `*IndexRef` (`&s[i]`); every
  member/index/deref path in the VM (`OpDeref`, `OpSetInd`,
  `selectMember`, `setField`, `setIndex`, `slice`, `call`,
  `invokeDeferred`) unwraps through them. `&s.f` therefore needs no new
  storage — a `FieldRef` is a cell view resolved at access time, and
  pointer receivers bind any Deref-able ref (else wrap in a fresh cell).
- **Compound assign on non-idents is three stack shapes**, not a rewrite
  to load+store temps: `s.f op= v` is Dup/Select/Binary/SetField, `x[i]
  op= v` needs `OpDup2` (keep base+idx for the store), `*p op= v` is
  Dup/Deref/Binary/SetInd. IncDec shares the same shapes with a const-1.

### `OpInstantiate` doubles as indexing

- `a[i]` and `F[T]` share one encoding: the compiler emits
  `OpInstantiate(n)` for `IndexExpr`/`IndexListExpr`, and the VM falls
  back to `v.index` when the base is not a generic `Function`/`TypeDef`.
  Single-index expressions keep value semantics for the index; only the
  type-level spelling (`typeExpr`) feeds typedefs. IndexListExpr is
  always instantiation.

### Interfaces are duck-typed; there is no static type identity

- **Satisfaction = method-set containment.** `TypeDef` carries `MReqs`
  (declared method names) + `IEmbeds`/`Embeds` for embedded interface
  elements; the `IfaceReqs` hook unions them transitively and
  `satisfiesIface` checks `reqs ⊆ MethodsOf(v)`. `any` and `error` are
  builtin typedefs.
- **Constraint elements (`~T`, unions, `comparable`) are collected but
  treated as satisfied** — minigo2 has no static type algebra to check
  them against.
- **Asserted-type matching is by name/kind, not identity.** `x.(T)`
  compares the typedef name + package (or kind for slices/maps/chans);
  `int` asserts on the whole int family (`int64` is the storage). `x`
  holding a `*Struct` asserting an interface typedef checks method-set
  satisfaction — including interface-embedded struct fields, where
  `FindMethod` returns `(nil, fieldValue, ok)` so the VM dispatches on
  the stored concrete value.
- **A failed single-form assert is a script `Panic`, not a `Trap`** —
  `defer`/`recover` catches it. Comma-ok form pushes `Tuple{v, ok}`.

### Generics are monomorphize-on-use, and erasure shows through

- **`Function.TParams` + `Binds map[string]Value`** is the whole
  mechanism: `F[int]` clones the function with the binding, and the
  compiler consults `c.binds` after locals/upvals in `getRef`, so `T(x)`
  inside a generic body resolves to the bound typedef and behaves like a
  conversion. `TypeDef` specialization (`specializeType`) re-binds method
  receivers the same way.
- **The gaps are a consequence of erased types**: `var z T` stores `nil`
  (declared types are never tracked — there is no type to materialize a
  zero value from), and `Id(40)` cannot infer `T` because binding only
  happens at `F[T]` sites. Both are documented TODO items rather than
  bugs to fix now.

### Special forms are caller-scoped quoted calls

- **`OpSpecialCall` resolves statically, at compile time, through the
  file's import scope**: `dsl.Twice(...)` compiles to the op only when
  `dsl` resolves via `Scopes[file]` to an `ImportRef` whose
  `SymbolID{PackagePath, Name}` has a registered `SpecialFunc`. Anything
  else stays a normal call — specials never shadow real members.
- **`QuotedCall` snapshots the caller's local/upval maps** (names → slot
  indices); `SpecialContext.Eval` compiles the arg AST with
  `compile.ExprScoped` overlaying those slots, then runs a frame sharing
  the caller's cells — so `dsl.Twice(x+1)` sees the caller's `x` and a
  handler that never Evals produces true laziness (`boom()` unrun).
- **This is quoted-Go at the *expression* level**, not the whole-program
  `QUOTE`/`UNQUOTE` sketched earlier: partial-argument evaluation falls
  out of `Eval` being per-arg.

### Control flow: labels, fallthrough

- **Labels are claimed only by directly-wrapping control constructs**
  (`for`/`range`/`switch`/`select`/`type switch`) via `pendingLabels`;
  everything else just registers a jump target. `break L`/`continue L`
  scan the ctrl stack for a ctx carrying the label — `break L` on a
  switch exits the switch, not a loop.
- **Forward `goto` resolves at function end** (`pendingGotos`); an
  unresolved name patches to a run-time `OpTrap`. Go's scoping
  restrictions (no jumping into a block, over declarations) are not
  enforced.
- **`fallthrough` patches to the next clause's body start** (or the
  default body, or a trailing trap) — collected per body in `c.falls`,
  so `fallthrough` mid-clause just jumps; `case` tests are never
  re-entered.

### Misc

- **`WithHostPolicy(func(path, sym string) bool)`** filters `Bind`
  symbols uniformly — stdlib intrinsics included — so a restricted engine
  drops `os.Getenv` by dropping the symbol rather than by special-casing
  `os`. Behavioral surfaces (output destination, future I/O) are still
  unscoped.
- **`errors.As` is approximated**: scripts cannot spell the target type
  the way Go's `As(&target)` relies on, so it binds the first non-nil
  cause through `SetRef`.
- **`errors.Is`/`Unwrap`** chain through the `Unwrap` intrinsic method
  convention and `*runtime.GoValue` boxing.

### Post-review fixes (e2e differential run vs real Go)

- **Nil is iterable**: `for range` over `runtime.Nil` yields zero
  iterations (nil slice/map semantics; a nil channel that would block
  forever folds into the same approximation). `f(nil...)` spreads to
  zero args, `len(nil)` is 0, `append(nil, ...)` creates the slice.
- **Failed comma-ok asserts bind the zero value** (`zeroOf`), not
  `runtime.Nil` — `v, ok := x.(int)` leaves `v` usable as `0`.
- **Elided literal element types compile to `OpElemType` chains**:
  `{{1,2}}` inside `[][]int` emits `typeExpr(parent)` + depth×peel of
  the enclosing typedef, resolved at run time through `TypeDef.Anon`
  (or `Spec.Type`) by the `ElemOf` hook — so named containers
  (`type Matrix [][]int`) work too, as long as the underlying AST
  names a resolvable element type.
- **`x.(any)` on a nil interface fails** (nil has no dynamic type);
  `case nil` in a type switch is `BinEql`, never `OpAssertOK`.

## 24. Round-5 notes: declared types, typed nils, goto legality, constraint checks

Round 5 pushed the value model one step closer to Go's: declared types
now reach the VM for bindings (`var x T`), typed nils keep their
identity through interface slots, and the compiler diagnoses `goto`
scope violations and generic constraint failures — still as run-time
traps, never compile errors, so the total-function invariant holds.

### Declared types at run time (`OpCoerce`)

- **The compiler emits a typedef + coerce op wherever Go would apply a
  declared type**: `var x T` locals (`OpCoerce`), package-level vars
  (`OpCoerceGlobal`), call params and named results (a prologue of
  `OpCoerce` per declared param — also what makes `var z T` inside a
  generic body see the *bound* typedef, since `binds` rewrites the type
  ident to the targ's TypeDef), and explicit returns (`OpCoerceTop`
  against the declared result type). `coerce` itself is tiny: `NIL` →
  the declared type's zero; `*TypedNil` into an interface kind →
  `*IfaceNil`; everything else passes through — the VM stays
  dynamically typed, the annotation only manufactures zeros and boxes.
- **`runtime.Zero(td)` materializes Go zero values**: interface kinds →
  `NIL` (an interface zero is nil), nilable kinds
  (`*T`/`slice`/`map`/`chan`/`func`) → `*TypedNil{Typ}`, structs →
  `*Struct` with `Fields` slots, named basics → the underlying literal
  (`int64(0)`), aliases via the `Underlying` hook.
- **Struct zeros need field types the runtime cannot see** — `Def` only
  carries names. A new `Hooks.FieldTypes` resolves each field's AST in
  parallel with `td.Fields` lazily (embedded fields count once,
  `td.Binds` answers `T`-typed fields on an instantiated generic). The
  VM's `zeroValue` recurses through it with an ancestor `seen` set so
  `type T struct{ X T }` bottoms out instead of diverging; unresolvable
  field types stay `NIL`. The same fill applies to composite literals —
  `Sq{}` zeros unmentioned fields too.
- **Typed nil is a real value**: `*TypedNil{Typ}` for `(*int)(nil)` and
  friends (`== nil` is true), `*IfaceNil{Typ}` for a typed nil boxed
  into an interface slot (`== nil` is false — matching Go's non-nil
  interface). Every VM path that learns "this is nil" learned both:
  eql/truthy/index/deref/iterate/member-select/method-dispatch/
  assert/convert/popArgs. `OpDeref`/`OpSetInd` on a TypedNil pointer
  raise a recoverable `*Panic` (Go's nil-pointer panic), other nilable
  kinds keep the trap.
- **`*T` became a first-class typedef (`KindPointer`)**: `x.(*int)`
  asserts pointer identity (a `*Cell` whose element typedef matches),
  `[]*Sq{{...}}` elides `&` via `ElemOf` peeling, and a paren-wrapped
  callee — `(*int)(nil)` — is parsed as a conversion, not a deref
  (`isTypeForm` now peels `ParenExpr`; the earlier shape compiled a
  `*int` deref of the `int` typedef and trapped).

### Map reads return the element zero

- `*runtime.Map` grew `Typ *TypeDef`; literals and `make(map[K]V)` set
  it. A missing key — or any read on a nil map — yields `mapZero`:
  `ElemOf` → `zeroValue`, so `m["k"]` on `map[string]int` is `0`, not
  `NIL` (scripts doing `v != 0` now behave). Maps built by intrinsics
  may carry `Typ == nil` and fall back to `NIL` — a known seam.

### Function-local `type` declarations

- `DeclStmt TYPE` inside a body binds a `*TypeDef` const as a local —
  `type S struct{...}` in a function resolves identically to a
  package-level one (literals, asserts, conversions, methods all go
  through the same `getRef`/`TypeDef` paths, so nothing else had to
  learn about locality). The typedef's `Methods` map lazily fills via
  the receiver scan like top-level types.

### `goto` scoping diagnostics

- Labels record the set of block IDs they're declared in plus a
  snapshot of visible variable names; each `goto` records its own two
  sets at resolve time. `gotoViolation` checks label-blocks ⊆
  goto-blocks (else "jumps into a block") then label-vars ⊆ goto-vars
  (else "jumps over declaration of x") — an approximation of the spec
  rule expressed over the compiler's own block/var bookkeeping. A
  violation replaces the `OpJump` with an `OpTrap` carrying Go's
  message shape; the compile still never fails.

### Constraint checking at instantiation

- `TypeSpec.TypeParams` constraints are collected into
  `TypeDef.TConstraints`/`Function.TConstraints` at materialize time;
  `checkTArgs` runs at `OpInstantiate` before binds are applied. A
  subtlety: `T ~int | ~string` arrives as a top-level `BinaryExpr`, not
  wrapped in `InterfaceType` — element-shaped constraint expressions
  route through `satisfiesTypeElem` (`~T` = underlying-kind match, `|`
  = union member check, `comparable` and plain method-less interfaces
  accept). A wrong arg traps at the instantiation site, which is the
  closest a run-time system gets to Go's type-check rejection.
- Call-site inference (`inferBinds`): when a generic function has
  `TParams` but no `Binds`, the VM binds each type param from the
  dynamic type of the corresponding arg (`Id(40)` → `T=int`). It is
  argument-driven only — no result/context inference — and uses the
  arg's *runtime* type, so a `T` constrained to a named basic may
  under-infer.

### Intrinsics & host surface

- The fmt `Print*` family and the builtin `print`/`println` route
  through `Engine.out` (`WithOutput(io.Writer)`), closing the
  output-destination gap the host-policy note flagged.
- Coverage: `errors.Join`; strings `ContainsAny`/`Compare`/`Replace`/
  `Cut`/`CutPrefix`/`CutSuffix`; strconv `Quote`/`Unquote`/`ParseUint`/
  `FormatFloat`/`FormatBool`; sort `SliceIsSorted`; slices `IsSorted`/
  `SortFunc`/`EqualFunc`/`IndexFunc`/`Max`/`Min`/`Reverse`/`Insert`/
  `Delete`; maps `Copy`/`Equal`; time `Parse`/`Unix` — mostly
  script-predicate bridges over `VMCaller.Call`.

### Surprises found while implementing

- **Indexed slice literals sized by element count**, not max index:
  `[]int{1: 7, 3: 9}` allocated a 2-elem slice and panicked. Now sizes
  by `max(index)+1` with `NIL` gaps (approximating Go's zero gaps).
- **`F[[]int]` never reached `typeExpr`** — the single-index expr path
  always compiled the index as a value expression, so only idents
  could be type args. Type-form args now route to `typeExpr`.
- **`Binds` had to move onto `TypeDef`** (it only lived on `Function`):
  `specializeType` records the targs so `FieldTypes` can answer what a
  `T`-typed field resolves to on an instantiated struct.
- **`errors.As`'s approximation stays**: the script side still cannot
  spell `*target` the way Go requires; the current binding-of-first-
  cause is documented rather than re-engineered.

## 25. Round-6 notes: assignability checks and `*runtime.Named`

This round closed the four typed-zero follow-ups: maps now stamp their
declared typedef on binds, `var x T = v` (and `x = v`) enforces
assignability, declared named-basic values keep their declared identity
through `*runtime.Named`, and unresolvable struct field types yield
typed-nil holes instead of bare `NIL`.

### `Cell.Typ` — the declared type of a slot

- `runtime.Cell` grew `Typ *TypeDef`, stamped wherever a slot is
  declared (`OpCoerce`/`OpCoerceGlobal` on locals/globals, `new(T)`).
  `assignCell` — used by `OpSetLocal`/`OpSetUpval`/`OpSetGlobal` and
  the `*p = v` path (`OpSetInd`) — re-coerces on every store, so
  `x = v` on a `var x T` enforces the same contract as the
  declaration. This is the piece that makes plain assignment
  type-checked, not just `var` initializers.
- `FieldRef`/`IndexRef` writes coerce against the struct's declared
  field type / the map or slice's declared element type — so
  `s.f = v` and `m[k] = v` are checked too, not just variable stores.

### Assignability at run time (`coerce` → `coerceConcrete`)

- Non-interface targets go through `coerceConcrete`: peel the target
  through `Alias`/`NamedBasic` (`peelNamed`, capped), check the
  unboxed value against the peeled shape (`shapeOK` — basic families,
  `*Struct` by typedef identity, `Slice`/`Map`/`Chan`/`Func`/`Pointer`
  kind checks), then re-tag. Interfaces still route through
  `satisfiesIface` and keep boxing semantics.
- `TypedNil` re-tags only through `sameTypeDef`/`tdShapeEq`, so
  `(*Sq)(nil)` cannot bind a `*int` slot. `IfaceNil` traps on
  re-assignment to a concrete type (a boxed nil keeps its dynamic
  type), matching Go.
- Two named map types refuse to re-bind (`var o M3 = m` where `m` is
  `M2` traps); an anonymous-shaped map may bind into a declared map
  type only when the element shapes match (`tdShapeEq`), and a map
  with no typedef yet gets stamped so missing-key reads resolve the
  declared element zero.

### `*runtime.Named` — declared identity on basic values

- `Named{Typ, V}` wraps a value with its *declared* typedef. Only
  typedefs that came from a `type` spec (`Spec`/`Pkg` set — builtins
  and anonymous shapes have neither) produce `Named`, so
  `Convert[int](40)`/`T(x)` on builtins stay bare and all the
  untyped-constant semantics keep working.
- Arith on a single declared tag re-tags the result (`var c Celsius;
  c + 1` is `Celsius`); mixing two different declared tags traps
  "mismatched types", which is the closest a run-time check gets to
  Go's compile error. An untyped operand adopts the named operand's
  tag. `x.(T)` on a Named checks `Typ` identity — `x.(float64)` on a
  `Celsius` correctly fails.
- `type A B` shares B's storage, not B's methods: `namedMember`
  resolves methods only from `n.Typ.Methods`, and `methodsOfValue`
  checks `*Named` before deref (a Named over a pointer-like
  underlying must not dereference through its tag). Fields still read
  through `n.V` when it is a `*Struct`.
- `runtime.Unwrap(v)` peels `Named→V` and is used at ~30 consumption
  sites (truthy/binaryOp/eqlValue/index/lenOf/asChan/convert/
  intrinsics/`assignReflect`…), so the tag is invisible everywhere a
  value flows outward and visible only where type identity matters.

### Field-type fallback

- `fieldTypes` (dispatch.go) now synthesizes a `NamedBasic` typedef
  (`Anon` = the field's unresolved type expr) when `elemTypeRef`
  fails, and `runtime.Zero` maps an unresolvable `NamedBasic` to
  `TypedNil{td}` — so `type W struct{ R io.Reader }` with `io`
  unresolvable fills `W.R` with a typed-nil hole: `h.R == nil` is
  true, and `var x any = h.R` boxes non-nil, like any typed nil.

### Approximations taken (unplanned, documented)

- **Bare values act as untyped constants.** Once a `:=` value erases
  its type, `var y MyInt = intVar` wraps instead of rejecting —
  `int`-vs-`MyInt` named-to-named rejection needs static type
  information the runtime does not have. Consequences: numeric
  families widen loosely (int names take int64, float names take
  int64|float64).
- **`x = v` inherits declaration semantics silently.** `var x int;
  x = nil` zero-fills rather than trapping — coerce is shared.
- **Boxed pointers are unchecked.** `OpBox` cells (from `&literal`)
  carry no `Typ`; stores through them can't re-coerce.
- **Consts are never coerced** — `const k MyInt = 5` stores a bare
  `int64`, so a const read cannot assert back to `MyInt`.
- **Named-pointer/map/chan/func typedefs can only tag nil.** A
  `*Cell`/`*Map` has nowhere to carry a declared tag short of
  wrapping every pointer-like value; `type P *T` therefore stays
  shape-approximate on values (though its `TypedNil` retags).
- **`type I2 I` peels to the underlying interface** — `x.(I2)`
  behaves as `x.(I)`; the I2 name is lost in matching.
- Only int64/float64/string results re-tag in `binaryOp` — bool
  results stay bare (correct: they re-coerce on the next bind).

### Review fixes (post-PR pass)

- **Alias vs defined peeling.** The Named-bind check originally compared
  `n.Typ` against `peelNamed(td)` — a `type A B` chain whose peel stalls
  on B (unresolvable underlying) let a `Named{B}` bind an `A` slot and
  keep the wrong tag. A Named value may bind only its identical declared
  type or an *alias* of it, so the check now peels through the new
  one-hop `Hooks.AliasOf` hook (`peelAlias`) — `Underlying` is
  transitive across `type A B` by design and could not be reused.
  `coerce`'s alias branch peels one hop at a time for the same reason
  (`type A = Str` must hit the `Str` typedef, not jump to `string`).
- **Typed nils check interface satisfaction too** — `(*int)(nil)` boxed
  into a method-requiring interface without consulting the method set;
  the boxing path now runs `satisfiesIface` first.
- **Method sets see Named through a pointer deref** — `methodsOfValue`
  checked `*Named` only before its deref loop, so `&c` on a named value
  derefed past the tag and reported no methods; the check now lives
  inside the loop.
- **Unary ops keep the declared tag** — `-x`/`+x`/`^x`/`!x` on a Named
  operand returned the bare underlying value; results re-tag like
  `binaryOp`.

## 26. Round-7 notes: file-level entries and the first special-form consumer

This round surveyed and executed the `convert-define` migration
(`sketch/plan-minigo2-convert-define.md`), which made minigo2's §12 claims
pay off against a real tool.

### File-level entry points (out-of-plan addition)

- The plan's entry model is directory packages (`minigo run ./app`), and
  `resolve.ReadPackageFiles` filters with `go/build` match rules — a DSL
  file guarded by `//go:build codegen` is *invisible* to `Run` unless its
  tag is mirrored into `BuildConfig.Tags`, which is brittle when DSL and
  non-DSL files share a directory. `Engine.LoadFile`/`Engine.RunFile`
  therefore make the **named file** the whole package — a direct port of
  v1's `LoadFile`+`Eval` semantics. The package gets a synthetic path
  (`"<file>"+abs`) and its own cache (`e.files`) so sibling files are not
  leaked into a later `Package(dir)` — `byDir` stays directory-pure.
- `resolve.BuildConfig.CheckDir` is exported so `LoadFile` keeps honoring
  `AllowedRoots` (an entry point must still live inside the roots).

### What the migration actually needed (and did not need)

- **The `goscan.Scanner` stays host-side.** convert-define resolves
  `pkg.Type`/`pkg.Func` into `scanner.TypeInfo`/`scanner.FunctionInfo` to
  build `model.ParsedInfo`; minigo2's resolver is locator-level
  (`PackageMeta`) by design and never produces `scanner` types. The plan's
  `ctx.ResolveType` is *not* required — the handler maps the file-local
  alias to a path via `ctx.Package().Scopes[ctx.File()]` and asks its own
  scanner.
- **Registration order is free.** `trySpecial` consults `p.Specials`, the
  shared `e.specials` map reference — specials registered after a package
  is parsed still dispatch correctly.
- **Interior calls are data.** `c.Map`/`c.Convert`/`c.Compute` inside the
  quoted `*ast.FuncLit` are never compiled — the whole literal stays AST —
  so `(*Config).Map` needs no method special form. §12.7's recommendation
  holds in practice.
- **Laziness is load-bearing.** A spy resolver wrapped around the engine
  proves zero `Locate`/`LocateDir` calls for a define-style run: `define`
  (special target) and `convutil`/`source`/`destination` (quoted args) are
  never materialized — not even to `Indexed`. The same run also covers the
  DSL-package-never-parsed claim even when the DSL file is the only
  package on disk.
- **`Resolve`/`ResolveType` on `SpecialContext` remain unimplemented** —
  convert-define is their first real consumer shape; `ResolveSymbol`
  landed in §27 and now backs convert-define's alias lookup (§28).

## 27. Round-8 notes: host stub package, ResolveSymbol, REPL, unsafe/runtime intrinsics

### `minigo.dev/host` resolves through the same intrinsic table as the in-repo stub

§11's gopls-friendly pattern lands as `minigo2/host`: a stub package
whose bodies are `panic("minigo intrinsic")`, so real Go tooling can
type-check scripts while the interpreter never runs them.
`installStdlib` binds one host table under both `minigo.dev/host` and
the in-repo import path — the `pkgs` check in `loadPath` makes bound
paths win before the resolver is consulted, so the stub's panic bodies
are unreachable in either spelling. `host.Exit` always errors (an
interpreted program cannot terminate its host); the env/argv/wd helpers
bind only when the engine is unrestricted, on the same condition as
`os.Getenv`/`os.Args`.

### `SpecialContext.ResolveSymbol` — index-level laziness for quoters

The §12.5 interface sketched `Resolve`/`ResolveType`/`ResolveSymbol`;
only `ResolveSymbol` landed because it is the one needing no evaluation:
`pkg.Sym` maps through the caller file's import table straight to
`SymbolID{path, name}` (no `Materialize` call — quoting
`huge.ConvertFoo` does not initialize `huge`), a bare identifier maps to
a member of the caller's package, and locals/upvals error out.
`Resolve`/`ResolveType` remain unimplemented: `Eval`/`Call` cover the
evaluated cases and no consumer is driving type-level queries yet.

### REPL: persistent globals by hoisting, not by replay

`engine.NewREPL()` keeps a scratch `*runtime.Package` (`<repl>`) on a
session engine. Each line classifies as declarations (imports and
func/type decls accumulate; var/const names are *hoisted* into
`pkg.Globals` as cells and their initializers run as a step) or
statements (a generated `func __stepN() any`). `reload()` re-parses the
accumulated source and swaps Files/Index/Scopes/Imports while keeping
`Globals` and `State` — the `__init__` once is already consumed, so
re-indexing is free and values persist. Divergences worth noting:

- `x := e` inside a line rewrites to `=` against the hoisted global —
  re-declaration updates rather than shadows, matching Python-REPL
  intuition, not Go scoping.
- `var x T` without a value lowers to `x = *new(T)`; `var`/`const` in
  statement position hoist the same way, so block scope does not exist
  at the prompt.
- The step must always end in an explicit `return`: declaring `any`
  makes the implicit `OpReturn` pop a result, which underflows on
  statement-only input — `return nil` is appended when missing.
- Blank imports added mid-session need an explicit `EnsureReady` — the
  synthetic `__init__` ran once, before the import existed.
- Input is line-oriented only (no brace continuation yet).

`cmd/minigo` grew `run --entry F` and `repl` subcommands; the bare
`minigo <ref> [func]` shorthand is unchanged. `--entry` is extracted
manually because `flag` stops parsing at the first positional argument.

### `unsafe`/`runtime` intrinsics are host approximations by design

The §11 intrinsic table gained `unsafe` (`Sizeof`/`Alignof` over the
boxed 64-bit representation — `Offsetof` errors since selector results
are not values) and `runtime` (`GOOS`/`GOARCH`/`Version`/`NumCPU`/
`GOMAXPROCS` pass through, `NumGoroutine` pins to 1 under the
single-threaded model, `GC` no-ops). `sort.Search`/`SliceStable` and
`slices.BinarySearch`/`BinarySearchFunc`/`SortStableFunc` close out the
ordering surface; `(index, found)` returns as a `*runtime.Tuple`.

## 28. Round-9 notes: verifying convert-define against the plan

This round turned the §12 claims about convert-define into executable
acceptance tests on the tool itself
(`examples/convert-define/internal/plan_test.go` —
`TestConvertDefineSatisfiesPlan`, plus the module-wide
`migration_guard_test.go` — `TestNoMinigoV1Dependency`). The assertions
that now pass:

- **Zero resolver traffic end-to-end.** A spying `resolve.Resolver`
  installed on the real `Runner` records **no** `Locate`/`LocateDir`
  calls for a full define run — `define` (special target),
  `convutil`/`source`/`destination` (quoted args), and `bogus`
  (dead-branch arg) are never materialized, not even to `Indexed`.
- **Alias-agnostic dispatch.** `import d ".../define"` still compiles
  `d.Rule`/`d.Convert` to `SPECIAL_CALL` — canonicalization happens
  per-file from the import table, so the local name is irrelevant.
- **Reachability, not existence.** `if false { define.Rule(bogus.Nope) }`
  never fires: the compiler emits `SPECIAL_CALL` behind a conditional
  jump (compile stays total; no dead-code analysis), and `bogus` is
  touched by neither the interpreter nor the host scanner.
- **Source-level dependency guard.** v1 `minigo` and `minigo2` share the
  `github.com/podhmo/go-scan` module, so "no v1 dependency" cannot be
  expressed in `go.mod` — `TestNoMinigoV1Dependency` scans every `.go`
  file's imports instead (and asserts `minigo2` is actually imported, so
  the check can't pass on a tree that uses neither).

### Out-of-plan observations

- **Host-side observability needs a seam.** §12.5 narrows what a special
  handler sees (`SpecialContext`), but nothing addresses the reverse —
  how a *host* observes its own engine. A tool that builds the engine
  internally (`Runner.Run`) cannot attach a spy from outside;
  convert-define now keeps an unexported `resolver` field as the test
  hook rather than widening `NewRunner`'s public API. Expect other
  consumers to need the same pattern (or an `Option`-style engine seam).
- **`LoadFile` is lazier than the plan promised.** §12.2 only claims the
  *special's* package is never located; in fact the *entry* package
  isn't either — `LoadFile` parses the named file and checks
  `BuildConfig.CheckDir` directly, so a fully-quoted DSL run issues zero
  resolver calls total, not just zero for the quoted packages.
- **Quoted args can name packages the host never reads.** The `bogus`
  fixture is real on disk and the DSL file stays statically valid Go
  (§12.1's property), yet neither engine nor scanner touches it —
  laziness extends past "not parsed by the runtime" to "never read by
  anyone". This also means the *dead-branch* form is a valid idiom for
  host-only annotation calls.
- **Specials registered for canonical paths ignore local names — a
  feature to test per consumer.** The plan assumed alias-tolerance; the
  first real consumer confirms it, but each new special-form host should
  keep an aliased-import case in its own acceptance suite, since the
  dispatch table is populated per tool, not per engine.
- **`ResolveSymbol` beats a raw `Scopes` lookup for exactly the reason
  the plan gave the interface the method.** convert-define now calls
  `ctx.ResolveSymbol(expr)` for both quoted `pkg.Type` and `pkg.Func`
  args: it still never materializes, and it additionally rejects a local
  or captured variable shadowing an import name — silent
  misresolution the direct `Scopes[file][name]` read could not see.

## 29. Round-10 notes: declared tags beyond basics

(Originally drafted as "round-7b" — it branched after round-7 but landed
after rounds 8–9, so it is renumbered to keep section order monotonic.)

The round-6 residuals turned out mostly tractable: every runtime value
that *can* carry a declared type now does — `Struct.Def` (already did),
`Map.Typ`, and new `Typ` fields on `Slice` and `Chan` (stamped by
composite literals, `make`, and any declared-type bind via a generalized
container-stamp in `coerceConcrete`).

- `declaredTag(x)` reads the tag; `tagIsNamed(td)` defines "named" as
  `Spec != nil || Name != ""` — anonymous structural typedefs carry a
  `Pkg` for resolution but are NOT named (that distinction matters:
  `var m M = map[string]int{...}` must bind, `var m M2 = m1` must not).
- The named-to-named trap only fires when the TARGET is also named:
  `var m map[string]int = om` stays a shape check (V named, T unnamed —
  Go allows it), while `var a A = sq` traps.
- Named pointers (`type P *T`) check the pointee's declared tag against
  `ElemOf` — `var p P = &other` traps; `OpBox` propagates the pointee
  tag into `Cell.Typ`, so `*p = v` through `&T{...}` coerces like a var
  store. `s[i] = v` and `ch <- v` coerce via `ElemOf(container.Typ)`.
- Typed consts coerce: package-level via `OpCoerceTop` before `bind`
  (consts are plain globals, not cells — `OpCoerceGlobal` can't reach
  them), locals via the existing slot coerce. Inherited const specs
  (`const (a T = 1; b)`) stay bare — the inherited type isn't carried by
  the index.
- Unnamed struct literals bind named struct types on a field-name
  match (`structFieldsEq`) — unnamed→named is assignable in Go when the
  underlying matches; field tags and element types are not compared.
- Still open: bare basic values remain "untyped constants" (`var y
  MyInt = intVar` wraps — distinguishing `:=`-erased `int` vars from
  literals would mean tagging every value, which is a much deeper
  change); `type F func()` / `type I2 I` keep only their underlying
  shape (no tag field on `Function`, asserts peel interfaces);
  `&s.f`/`&s[i]` FieldRef/IndexRef stores are still unchecked.

## 30. Round-11 notes: REPL continuation and reconciling round-10

Things learned outside the plan while reconciling the round-10 merge and
adding multi-line input to the REPL:

- **Fragment completeness is a token property, not a parse result.**
  `EvalLine` already accepted embedded newlines; the only real gap was
  the interactive loop. `IncompleteInput` runs `go/scanner` over the
  pending buffer: an open `()`/`[]`/`{}` group (depth > 0) or a final
  token where Go would not insert a semicolon (trailing operator,
  comma, dot, `:=`) means "keep reading"; a final inserted `;` means
  "evaluate". No speculative re-parsing, and the rule matches what
  users know from `gofmt`-formatted code.
- **`} else {` must share a line**, exactly as in Go source — `}` at
  end-of-fragment inserts a semicolon, so an `else` typed on the next
  line can never re-attach. The REPL inherits this rule for free by
  evaluating the buffer as soon as it reads complete.
- **Two "scan errors" are continuations, not errors**: an unterminated
  raw string (`` ` ``) and an unterminated `/*` comment are the only
  constructs Go legitimately continues across lines, so
  `IncompleteInput` reads them as incomplete; every other degenerate
  fragment (comment-only input, unterminated `"` or rune literals,
  negative depth) reads as complete so its error surfaces through
  `EvalLine` instead of waiting forever. The cost is that a genuinely
  mistyped line like `x +` also waits for continuation — there is no
  abort-fragment escape yet (noted in TODO.md).
- **EOF mid-fragment surfaces the parse error** rather than silently
  dropping the buffer — piped input can't keep the REPL waiting on a
  half-typed decl.
- **Range-over-func is a real gap, and a v1 regression**: `newIterator`
  covers slice/map/chan/int/string but traps `range over %T` on function
  values, so `for x := range f` (Go 1.23 `iter.Seq`) is unsupported —
  v1 minigo passes `minigo_range_func_test.go`. Recorded in TODO.md —
  needs a yield-callback bridge plus early-`break` plumbing (`yield`
  must return false).
- Renumbering note: sections 26–28 were written in merge order; the
  round-10 work predates them in branch order, which is why its notes
  were originally titled "round-7b".

## (end)
