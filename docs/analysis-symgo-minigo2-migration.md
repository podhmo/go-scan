# symgo on the minigo2 VM: Feasibility and Migration Plan

This report evaluates whether `symgo` — the symbolic execution engine in this
repository — can be rebuilt on top of `minigo2` (the bytecode-VM redesign of
`minigo`, described in `sketch/plan-minigo-vm.md`), and if so, how.

Status: **experiment / analysis only**. No code changes are proposed for merge.

## TL;DR

**Feasible, and the cleanest path is not "fork the VM at every branch" but a
second compilation mode.** symgo's special semantics — both-branches `if`,
once-per-case `switch`/`select`, unroll-once loops, bounded recursion,
symbolic placeholders instead of errors — can be expressed as a *trace-compile*
mode (`compile.Trace`) that emits straight-line bytecode instead of conditional
jumps, running on the existing stack VM plus two additions: a `Symbolic` value
that degrades gracefully through the value ops, and a call-boundary hook that
replaces `defaultIntrinsic` and the scan-policy dispatch.

Recommended shape: **Option A — compile-to-trace** (~2–4 sessions for a working
core, plus a conformance port of the symgo test suite). The harder alternative,
true fork/join symbolic execution inside the VM (Option B), buys path merging
but costs deep changes to `vm.exec` for semantics symgo does not actually need.

## 1. What is actually being converted

A common confusion to clear up first: **symgo does not depend on minigo today.**
`grep` shows minigo referenced from symgo only in
`symgo/integration_test/minigo_analysis_test.go`, where minigo is *scan input*,
not a dependency. symgo has its own tree-walking evaluator
(`symgo/evaluator/`, ~10k lines) over `go/ast`, its own object model
(`symgo/object/object.go`, ~1.3k lines), and its own package/type resolution
against `go-scan`'s `scanner.PackageInfo` / `TypeInfo` / `FieldType`.

So "convert symgo to minigo2" really means: **reimplement symgo's evaluator on
the minigo2 execution substrate** — its lazy resolver/index, `runtime` value
model, per-function bytecode compiler, and stack VM — while preserving the
semantics that make symgo a *tracer* rather than an *interpreter*.

What makes this worth doing at all:

- minigo2's lazy pipeline (`resolve → parse → index → materialize on first
  access`, per-file import scopes) is a strict superset of symgo's
  `getOrLoadPackage` + `ensurePackageEnvPopulated` + lazy `object.Variable`.
- `LazyInit` mode already provides "declarations and signatures materialize,
  var/const/init side effects stay pending" — exactly symgo's
  *symbolic-dependency* scope.
- The VM's `Hooks` table already delegates every type/lazy question back to
  the engine (`Materialize`, `MethodsOf`, `FindMethod`, `FieldTypes`,
  `IfaceReqs`, `TypeMethods`, `Underlying`, `ElemOf`, `AliasOf`,
  `CompileExpr`), so the type-system duality is handled at one seam.
- `Special` forms (quoted calls dispatched by `SymbolID`) cover the
  "inspect call args as AST" use cases that symgo intrinsics currently serve
  for DSLs like docgen's patterns.

## 2. Semantic inventory: what symgo does that a Go VM does not

From `docs/analysis-symgo-implementation.md` and the evaluator sources:

| # | symgo semantic | Detail |
|---|----------------|--------|
| S1 | Explore both branches | `if` evaluates then and else in enclosed envs; `switch` runs every case (each fallthrough chain independently); `select` runs every comm clause; `for`/`range` run the body **exactly once**; conditions are evaluated (for traced calls) but their results are ignored. |
| S2 | Symbolic placeholders instead of errors | Unknown/select-on-unknown/missing-method/embedded-failure → `SymbolicPlaceholder` or `AmbiguousSelector`; almost nothing is fatal. `ErrUnresolvedEmbedded` yields an assumed member, not an error. |
| S3 | Branch merge via env chain | `=` writes walk up to the defining scope (mutations merge across branches, last-wins); `:=` stays branch-local (`SetLocal`). `PossibleTypes` on a `Variable` accumulates concrete types assigned to interface-typed vars across branches. |
| S4 | Bounded recursion | Same function re-entered (method recursion keyed on receiver identity via `BoundCallStack`/call-stack `Pos`) → return signature-typed symbolic placeholders instead of recursing. |
| S5 | Call-boundary instrumentation | `defaultIntrinsic` fires on *every* call before dispatch (find-orphans' usage marking). Per-symbol intrinsics keyed by `"pkg.Path.Fn"`, `"(pkg.Type).Method"`, `"(*pkg.Type).Method"`. `PushIntrinsics`/`PopIntrinsics` provide scoped overrides. |
| S6 | Func-lit arg scanning | `scanFunctionLiteral` eagerly evaluates a func-literal argument's body with symbolic params (for side effects like `mux.HandleFunc("/x", handler)`). |
| S7 | Scan policy | Per-import-path gate: in-policy → eval body; out-of-policy → return typed symbolic result synthesized from the signature (`createSymbolicResultForFuncInfo`); symbolic-dependency scope → declarations only. |
| S8 | Lazy package vars | `object.Variable{Initializer, IsEvaluated}` + `forceEval`; package envs populated lazily by kind (consts/types/vars/funcs). |
| S9 | Type switch per-case instances | Each case binds a **fresh symbolic instance of the case type**. |
| S10 | Interface method calls | Calling a method on an interface-typed var records `calledInterfaceMethods` + returns a signature-typed placeholder; `Finalize()` post-pass maps to implementers across scanned packages via `scanner.Implements` and marks them used. |
| S11 | Branch returns are dropped | A `return` inside an if/switch arm does not abort the function — evaluation continues to the merge point (the `ReturnValue` is not propagated). |
| S12 | Tracer | Per-AST-node `TraceEvent`s for call-graph/tools output. |
| S13 | Cycle tolerance | `evaluating`/`evaluationInProgress`/`BoundCallStack` guards against package-load and initializer cycles. |

None of these exist in a normal interpreter; together they define "all-paths,
never-fail, trace-everything" evaluation.

## 3. Mapping onto minigo2

### 3.1 Already provided

| symgo need | minigo2 counterpart |
|------------|---------------------|
| Lazy package load + per-file imports | `resolve` locator → `index` decl table → `Materialize` hook; `p.Scopes[file]` import refs |
| Lazy vars / signatures-only scan | `LazyInit` init mode; `Globals` materialize lazily |
| Type/method/embedded questions | `Hooks`: `TypeMethods`, `FindMethod` (promoted methods = the embedded DFS), `FieldTypes`, `IfaceReqs`, `Underlying`, `ElemOf`, `AliasOf` |
| Signature-typed results | `Function.Decl.Type` AST + `typeDefOf`/file-scope type resolution |
| Bounded *execution* | `frame.boundLo/boundHi` (re-entrant bounded run for range-over-func yield) — precedent for "run a body region once" |
| Intrinsic dispatch by symbol | `Special` registry + `Builtin` hook; member-select funnel (`v.selectMember`) can serve `(T).M` / `(*T).M` keyed intrinsics |
| Scan policy | Materialize-time `Compile` choice per `Decl` — in-policy gets bytecode, out-of-policy gets a symbolic callee (see §4) |
| AST-fragment bridge | `OpEvalAST` + `CompileExpr` hook (the existing migration escape hatch) |
| Implements check for `Finalize` | `TypeMethods` + `IfaceReqs` compose into a method-set coverage test |
| Host/stdlib overrides | `Bind()` + `hostPolicy` (bound packages beat source) |

### 3.2 Requires new machinery

**N1 — A `Symbolic` value and tolerance in the value ops.**
`runtime.Value` is `any`; adding `*runtime.Symbolic{Reason, Typ *TypeDef,
Receiver, UnderlyingFunc, PossibleTypes}` is easy. The cost is *tolerance*:
today every op traps on an operand it can't handle (`f.trap`). Under symgo
semantics the same sites must *degrade to Symbolic* instead — `OpBinary`,
`OpUnary`, `OpSelect`, `OpIndex`, `OpDeref`, `OpSlice`, `OpCall` on a symbolic
callee, `OpMakeComposite` on an unresolved type, etc. Roughly 15–20 op arms
need a `if s, ok := x.(*Symbolic); ok { return derivedSymbolic(s) }` fast path.
This is the single most invasive VM change, and it inverts minigo2's
"explicit `OpTrap` instead of silent wrongness" philosophy — so it should be
gated on a per-frame/per-VM `Symbolic bool` flag, never active in normal runs.

**N2 — Branch semantics.** Two designs (see §4). Either a trace-compile mode
that emits arms sequentially (Option A), or real frame forking at
`OpJumpFalse`/`OpJumpTrue` (Option B).

**N3 — Loop-once.** Trivial in trace-compile (emit init+cond-eval+one body
pass). For range, bind symbolic k/v. On Option B, reuse `boundLo/boundHi`.

**N4 — Call-boundary hook.** symgo's `defaultIntrinsic` + intrinsic-key
dispatch + policy dispatch + recursion bound + func-lit scanning all live at
`applyFunction`. On the VM there is one funnel — `v.call`/`prepFrame` — where a
`Hooks.CallWitness(fn, args)` + the symbolic-callee dispatch (N5) reproduce all
of it.

**N5 — Symbolic callee.** Out-of-policy/unresolved functions become a
`*runtime.Function` variant (or a `BuiltinFunc`) whose invocation returns
signature-typed `Symbolic`s from `Decl.Type.Results` — replacing
`UnresolvedFunction`/`createSymbolicResultForFuncInfo`. Type-switch per-case
instances are `Symbolic{Typ: caseTypeDef}` bound into the case scope.

**N6 — `PossibleTypes` union.** symgo's merge map lives on `object.Variable`;
the VM analog is a field on `runtime.Cell` (set-or-accumulate on `assignCell`
when the cell's declared tag is an interface typedef) — also the
"fresh symbolic instance per type-switch case" needs this.

**N7 — Func-literal `Decl`.** `scanFunctionLiteral` needs the literal's AST
body; minigo2 currently drops `Decl`/`Lit` on eagerly-compiled func-literal
protos (`bindCompiles` sets `Compile` directly). Retain a `Lit *ast.FuncLit`
(or keep `Decl`-equivalent) on `Function` for literals — small change.

**N8 — Finalize.** Post-pass over scanned packages: `calledInterfaceMethods`
(recorded during calls on interface-typed receivers) → implementers via
`TypeMethods`/`IfaceReqs` → mark used through the witness hook. Engine-level,
not VM-level.

**N9 — Intrinsic key surface.** Today's keys include `(pkg.T).M` /
`(*pkg.T).M` receiver forms — beyond `SymbolID{PackagePath, Name}`. Either
extend `Special` lookup keys to include a receiver form, or keep a separate
layered registry consulted at `selectMember`/`v.call`. The standalone
`podhmo/minigo` already ships `TypeExpr.CanonicalName`, which produces exactly
these key shapes.

**N10 — Possible-return semantics (S11).** In trace-compile, `return` inside a
branch arm must not `OpReturn` — emit a "record possible return + jump to arm
merge" sequence instead (or just `OpPop`, matching symgo's drop-the-value
behavior, with a trace event).

## 4. Options

### Option A — compile-to-trace (recommended)

Add a second compiler entry `compile.Trace(fn) (*bytecode.Chunk, error)` that
emits the *same opcodes* but different control flow:

- `if` → `[init][cond eval, discard][then-arm][else-arm][merge]`
- `switch`/`select`/`type-switch` → `[tag/comm eval][each arm sequentially]`; a
  type-switch case first binds `Symbolic{Typ: caseT}` to the guard var
- `for` → `[init][cond eval for tracing][post skipped — symgo skips it][body once]`
- `range` → `[X eval][bind symbolic k/v][body once]`
- `return` inside an arm → trace event + jump to merge (or drop)
- `break`/`continue` inside an arm → jump to that arm's merge point

This exploits exactly what the redesign bought: **semantics live in the
compiler, not the VM.** Both arms' `:=` decls get distinct slots (each decl is
a distinct slot already); `=` writes hit shared outer cells — reproducing
symgo's merge semantics *for free*. No VM forking, no continuation cloning.

Choice of mode is per-function at materialize time: the engine's `Compile`
closure already decides `compile.Func` vs host/foreign stubs; a policy check
(`ScanPolicy(pkgPath)` — the same function symgo uses) selects
`compile.Trace` for in-policy source and a symbolic-callee stub for
out-of-policy. Mixed granularity (trace the entry package, symbolic-stub the
deps) falls out naturally.

VM deltas: N1 (Symbolic tolerance, flag-gated), N4 (CallWitness hook), N6
(Cell.PossibleTypes), N7 (Lit retention). No control-flow changes.

### Option B — VM forking

At `OpJumpFalse`/`OpJumpTrue` under a symbolic flag, clone the frame (stack +
locals slice; `Cell`s shared so `=` writes merge identically), run both
continuations to the join, then continue merged. Handles *dynamic* conditions
and keeps a single compiler — but requires defining "run until ip==join"
inside `v.exec`/`v.call` recursion, `OpReturn`-inside-arm capture, defer/label
edge cases, and meaningful work whenever the condition is concrete (normal
execution must not fork). Substantially more machinery for semantics symgo
doesn't need — it never joins on a *concrete* condition; it always wants both
sides. Only worth it if a future symgo wants KLEE-style path merging.

### Option C — keep the tree-walker, adopt only the substrate

Keep `symgo/evaluator` as an AST walker but re-base it on minigo2's
resolve/index/runtime: `object.Object` → `runtime.Value`,
`getOrLoadPackage`/`ensurePackageEnvPopulated` → `Materialize`/`LazyInit`,
`scanner.Implements` → `TypeMethods`/`IfaceReqs`, `FieldType` →
`TypeDef`/type-expr resolution. Lowest risk, but preserves a second engine and
doesn't unify the intrinsic/special-form surfaces. A reasonable **Phase 0** of
Option A: it de-risks the type-model port (the real unknown) before touching
control flow.

### Option D — no conversion

symgo stays a standalone engine; consumers that need DSLs use minigo2
specials alongside. Zero cost, keeps dual maintenance forever.

## 5. Proposed implementation (Option A)

Phase 0 — **substrate spike** (Option C, timeboxed): map `object`/`scanner`
concepts onto `runtime`/`index`; prove `Implements`-equivalence and
signature→typed-symbolic derivation on a few fixtures. Output: decision to
proceed.

Phase 1 — **core**:
1. `runtime.Symbolic` + flag-gated tolerance in the ~15–20 value-op arms.
2. `compile.Trace` for `if`/`switch`/`select`/`for`/`range`/`type-switch`/
   `return`-in-arm (Option A control flow). Start with expression/statements
   that already compile; `OpTrap` is acceptable parity for never-supported
   forms (goto across arms, `select` concrete semantics).
3. `Hooks.CallWitness` (default-intrinsic) + intrinsic registry incl.
   receiver-keyed entries; `PushIntrinsics` scoping via layered registry on
   the engine/session.
4. Symbolic-callee dispatch + bounded recursion (frame scan on `Decl.Pos` +
   receiver cell identity) + func-lit arg scan (N7).
5. `Cell.PossibleTypes` accumulation for interface-tagged cells; type-switch
   case instances.
6. `Finalize()` on the engine.

Phase 2 — **conformance**: port `symgo/evaluator/*_test.go` to the new engine
(the tests *are* the semantic spec — the analysis doc notes they verify
tracer, not interpreter, semantics), then switch `tools/find-orphans` and
`examples/docgen` behind a flag.

Phase 3 — **consumer cutover** and, only then, deprecation of the tree-walker.

## 6. Risks and open questions

- **Type-model duality** (biggest unknown): `scanner.TypeInfo`/`FieldType` vs
  `runtime.TypeDef`/AST type exprs. `PossibleTypes` keys, intrinsic keys,
  `Implements`, comma-ok assert handling all depend on it. Phase 0 exists
  precisely to de-risk this; `podhmo/minigo`'s new inspect API
  (`TypeExpr.CanonicalName`, `Sig` accessors, `SourceOf`) looks like the right
  surface.
- **Philosophy inversion**: `OpTrap`-on-unknown is a *feature* of the Go
  interpreter; symbolic tolerance must stay flag-gated per run so the two
  modes never contaminate each other.
- **Fallback parity**: symgo synthesizes `FunctionInfo` for interface methods
  missing from scan data and assumed members for unresolved embeds — the hooks
  must be able to return "unknown-but-typed" rather than erroring.
- **Target repo**: in-repo `minigo2` is being rebooted as the standalone
  `podhmo/minigo` ("code may lag behind the standalone repository"). A symgo2
  should build against the standalone module — the APIs above
  (Hooks/OpEvalAST/Special/bounded runs/inspect) all exist there.
- **Coverage floor**: symgo tolerates far more Go than it executes
  (generics instantiation, real `select`, goto); trace-compile inherits
  compile.Func's supported set, which is *narrower than the walker's* for some
  nodes (e.g. symgo's `evalGenDecl` accepts things the compiler traps on).
  Symbolic-degradation at compile time (emit `Symbolic` push instead of
  trapping) closes most of this — but it's a per-node grind.
- **Test suite**: the symgo suite is large and semantics-specific; porting it
  is the bulk of the work and the real acceptance gate.

## 7. Effort estimate

Measured in sessions (not human-time): Phase 0 ~0.5–1 session; Phase 1 ~2–3
sessions; Phase 2 ~1–2 sessions; Phase 3 ~0.5 session + consumer validation.
Total **~4–6 sessions**, dominated by the type-model port and test-suite
conformance. Option B roughly doubles Phase 1–2. Option C alone is ~1–2
sessions but leaves two engines to maintain.

## Appendix: entry points used today (consumer contract)

- `tools/find-orphans`: `RegisterDefaultIntrinsic` (usage marking on every
  call incl. func args), `Eval(file)`, `FindObjectInPackage`, `Apply(entry,
  symbolicArgs, pkg)`, `Finalize(ctx)`.
- `examples/docgen`: intrinsics keyed `net/http.*` /
  `(*net/http.ServeMux).HandleFunc`, `PushIntrinsics` scoping, `Apply(handler,
  args)` with handler args built from `handler.Decl.Type.Params`.
- `tools/goinspect`, `examples/call-trace`: `Eval`/`Apply` + tracer events.

Any minigo2-based symgo must reproduce this surface (or a thin compat shim) —
`Apply`, intrinsic registration, `Finalize`, and the tracer — which Phase 1
items 3–6 cover.
