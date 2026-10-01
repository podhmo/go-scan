# symgo → symgo2 on the minigo2 VM: Feasibility and Migration Plan

This report evaluates whether `symgo` — the symbolic execution engine in this
repository — can be rewritten as a **`symgo2`** built on the `minigo2`
substrate, in the same sense that `minigo` was rewritten as `minigo2` (see
`sketch/plan-minigo-vm.md`): a successor engine on the new runtime rather than
an in-place refactor, coexisting with v1 until consumers cut over.

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

Recommended shape: **Option A — `symgo2` = minigo2 engine + trace-compile**
(~2–4 sessions for a working core, plus a conformance port of the symgo test
suite). The harder alternative, true fork/join symbolic execution inside the
VM (Option B), buys path merging but costs deep changes to `vm.exec` for
semantics symgo does not actually need.

## 1. What is actually being rewritten

A common confusion to clear up first: **symgo does not depend on minigo
today.** `grep` shows minigo referenced from symgo only in
`symgo/integration_test/minigo_analysis_test.go`, where minigo is *scan
input*, not a dependency. symgo has its own tree-walking evaluator
(`symgo/evaluator/`, ~10k lines) over `go/ast`, its own object model
(`symgo/object/object.go`, ~1.3k lines), and its own package/type resolution
against `go-scan`'s `scanner.PackageInfo` / `TypeInfo` / `FieldType`.

So "symgo2" means: **a new engine that reproduces symgo's semantics and public
surface on the minigo2 execution substrate** — its lazy resolver/index,
`runtime` value model, per-function bytecode compiler, and stack VM — while
keeping symgo's identity as a *tracer*, not an *interpreter*. Same playbook as
minigo→minigo2: new package, shared go-scan substrate, phased consumer cutover.

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

## 2. Current usage inventory — the contract symgo2 must serve

All concrete consumers live in `tools/` and `examples/`. This inventory is the
grounding lemma for the whole design: symgo2 is *done* when these compile
against it.

| Consumer | symgo API surface used |
|---|---|
| `tools/find-orphans` | `NewInterpreter`(`WithLogger`, `WithPrimaryAnalysisScope`, `WithMemoization`, `WithScanPolicy`); `RegisterDefaultIntrinsic` — marks every call site as used, *including* func-literal args (relies on `scanFunctionLiteral` semantics); `Eval(ctx, fileAst, pkg)` per file; `FindObjectInPackage`; `Apply(ctx, entryFn, symbolicArgs, pkg)`; `Finalize(ctx)`; reads `object.{Function, SymbolicPlaceholder, Error}`, `CallStack` |
| `tools/goinspect` | `NewInterpreter`(`WithLogger`, `WithScanPolicy`, `WithMemoization`); `RegisterDefaultIntrinsic`; `EvaluatorForTest()` (raw evaluator escape hatch); `Apply` |
| `examples/call-trace` | same as goinspect, plus `i.CallStack()` snapshots on every call for trace output; `Finalize` |
| `examples/docgen` | `WithPrimaryAnalysisScope` + `WithSymbolicDependencyScope` + `WithTracer`; `RegisterIntrinsic` with all three key shapes: `net/http.Fn`, `(pkg.T).M`, `(*pkg.T).M` — **including a type-conversion intrinsic** (`net/http.HandlerFunc`); `PushIntrinsics`/`PopIntrinsics` scoping; `Apply(handlerFn, symbolicArgs)` with args built from `handler.Decl.Type.Params`; the `patterns/` DSL handlers (`pattern.Apply(ctx, interp, analyzer, args)`) *call back into the interpreter* from inside an intrinsic |

Object-model surface consumers touch directly (must exist as `runtime.Value`
shapes in symgo2):

| symgo `object` type | minigo2 `runtime` counterpart |
|---|---|
| `SymbolicPlaceholder` (+`Reason`, `UnderlyingFunc`, `Receiver`, `PossibleConcreteTypes`) | `*runtime.Symbolic` (new — N1) |
| `Function` (+`Decl`, `.Package`, `TypeInfo()`) | `*runtime.Function` / `*runtime.Closure` |
| `Instance` (+`BaseObject.ResolvedTypeInfo`) | `*runtime.Struct` (+`TypeDef`) |
| `Variable` (+`Initializer`, `PossibleTypes`) | `*runtime.Cell` (+ `PossibleTypes` — N6) |
| `String`, `Pointer`, `Slice` | `string`/`*runtime.String`, `*runtime.Cell`, `*runtime.Slice` |
| `MultiReturn` | `*runtime.Tuple` |
| `Nil` | `runtime.NIL` / `*TypedNil` |
| `Error` | symgo errors are *returned values*, not Go panics — needs an error value or structured return, not `runtime.Trap` |
| `UnresolvedFunction` / synthetic `FunctionInfo` | symbolic-callee stub (N5) |
| `Environment`, `FileScope`, `CallFrame`, `Tracer`/`TraceEvent` | frame scopes/cells; VM frames; instruction-level `Pos` trace |

Intrinsic signature today: `func(ctx, *Interpreter, args []Object) Object` —
**reentrant** (docgen's `pattern.Apply` calls `interp.Apply` again). On
minigo2 this maps to `SpecialFunc`/`BuiltinFunc` over `SpecialContext`, which
already exposes `Eval`/`Call`/`Resolve`/`ResolveSymbol`/`ResolveType` — a good
fit, but symgo2 must add an `Apply(callee, args)`-level operation on the
context for the docgen pattern handlers.

## 3. Semantic inventory: what symgo does that a Go VM does not

From `docs/analysis-symgo-implementation.md` and the evaluator sources:

| # | symgo semantic | Detail |
|---|----------------|--------|
| S1 | Explore both branches | `if` evaluates then and else in enclosed envs; `switch` runs every case (each fallthrough chain independently); `select` runs every comm clause; `for`/`range` run the body **exactly once**; conditions are evaluated (for traced calls) but their results are ignored. |
| S2 | Symbolic placeholders instead of errors | Unknown/select-on-unknown/missing-method/embedded-failure → `SymbolicPlaceholder` or `AmbiguousSelector`; almost nothing is fatal. `ErrUnresolvedEmbedded` yields an assumed member, not an error. |
| S3 | Branch merge via env chain | `=` writes walk up to the defining scope (mutations merge across branches, last-wins); `:=` stays branch-local (`SetLocal`). `PossibleTypes` on a `Variable` accumulates concrete types assigned to interface-typed vars across branches. |
| S4 | Bounded recursion | Same function re-entered (method recursion keyed on receiver identity via `BoundCallStack`/call-stack `Pos`) → return signature-typed symbolic placeholders instead of recursing. |
| S5 | Call-boundary instrumentation | `defaultIntrinsic` fires on *every* call before dispatch (find-orphans' usage marking). Per-symbol intrinsics keyed by `"pkg.Path.Fn"`, `"(pkg.Type).Method"`, `"(*pkg.Type).Method"`. `PushIntrinsics`/`PopIntrinsics` provide scoped overrides. |
| S6 | Func-lit arg scanning | `scanFunctionLiteral` eagerly evaluates a func-literal argument's body with symbolic params (for side effects like `mux.HandleFunc("/x", handler)`). find-orphans' coverage depends on it. |
| S7 | Scan policy | Per-import-path gate: in-policy → eval body; out-of-policy → return typed symbolic result synthesized from the signature (`createSymbolicResultForFuncInfo`); symbolic-dependency scope → declarations only. docgen uses all three tiers. |
| S8 | Lazy package vars | `object.Variable{Initializer, IsEvaluated}` + `forceEval`; package envs populated lazily by kind (consts/types/vars/funcs). |
| S9 | Type switch per-case instances | Each case binds a **fresh symbolic instance of the case type**. |
| S10 | Interface method calls | Calling a method on an interface-typed var records `calledInterfaceMethods` + returns a signature-typed placeholder; `Finalize()` post-pass maps to implementers across scanned packages via `scanner.Implements` and marks them used (find-orphans' core output). |
| S11 | Branch returns are dropped | A `return` inside an if/switch arm does not abort the function — evaluation continues to the merge point (the `ReturnValue` is not propagated). |
| S12 | Tracer | Per-AST-node `TraceEvent`s (docgen takes `WithTracer`). |
| S13 | Cycle tolerance | `evaluating`/`evaluationInProgress`/`BoundCallStack` guards against package-load and initializer cycles. |

None of these exist in a normal interpreter; together they define "all-paths,
never-fail, trace-everything" evaluation.

## 4. Mapping onto the minigo2 substrate

### 4.1 Already provided

| symgo need | minigo2 counterpart |
|------------|---------------------|
| Lazy package load + per-file imports | `resolve` locator → `index` decl table → `Materialize` hook; `p.Scopes[file]` import refs |
| Lazy vars / signatures-only scan | `LazyInit` init mode; `Globals` materialize lazily |
| Type/method/embedded questions | `Hooks`: `TypeMethods`, `FindMethod` (promoted methods = the embedded DFS), `FieldTypes`, `IfaceReqs`, `Underlying`, `ElemOf`, `AliasOf` |
| Signature-typed results | `Function.Decl.Type` AST + `typeDefOf`/file-scope type resolution |
| Bounded *execution* | `frame.boundLo/boundHi` (re-entrant bounded run for range-over-func yield) — precedent for "run a body region once" |
| Intrinsic dispatch by symbol | `Special` registry + `Builtin` hook; member-select funnel (`v.selectMember`) can serve `(T).M` / `(*T).M` keyed intrinsics |
| Scan policy | Materialize-time `Compile` choice per `Decl` — in-policy gets bytecode, out-of-policy gets a symbolic callee (see §5) |
| AST-fragment bridge | `OpEvalAST` + `CompileExpr` hook (the existing migration escape hatch) |
| Implements check for `Finalize` | `TypeMethods` + `IfaceReqs` compose into a method-set coverage test |
| Host/stdlib overrides | `Bind()` + `hostPolicy` (bound packages beat source) |

### 4.2 Requires new machinery

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

**N2 — Branch semantics.** Two designs (see §5). Either a trace-compile mode
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
the VM analog is a field on `runtime.Cell` (accumulate on `assignCell`
when the cell's declared tag is an interface typedef) — also the
"fresh symbolic instance per type-switch case" needs this.

**N7 — Func-literal `Decl`.** `scanFunctionLiteral` needs the literal's AST
body; minigo2 currently drops `Decl`/`Lit` on eagerly-compiled func-literal
protos (`bindCompiles` sets `Compile` directly). Retain a `Lit *ast.FuncLit`
(or `Decl`-equivalent) on `Function` for literals — small change.

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

**N11 — Error-as-value.** symgo `object.Error` is a return value (consumers
type-switch on it); `runtime.Trap` is a panic. symgo2 needs an `Error` value
type (or a Go `error` wrapped as a value) and "returned, not raised" plumbing
in the witness/call path.

## 5. Options

### Option A — `symgo2` = minigo2 + trace-compile (recommended)

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
deps) falls out naturally, and matches how `WithPrimaryAnalysisScope` /
`WithSymbolicDependencyScope` tier consumers today.

Packaging mirrors the minigo→minigo2 precedent: `symgo2/` (or
`symgo/symgo2/`) as a new package presenting the §2 API over an embedded
minigo2 `Engine`; `symgo/` stays until every consumer is ported.

VM deltas: N1 (Symbolic tolerance, flag-gated), N4 (CallWitness hook), N6
(Cell.PossibleTypes), N7 (Lit retention), N11 (error value). No control-flow
changes.

### Option B — VM forking

At `OpJumpFalse`/`OpJumpTrue` under a symbolic flag, clone the frame (stack +
locals slice; `Cell`s shared so `=` writes merge identically), run both
continuations to the join, then continue merged. Handles *dynamic* conditions
and keeps a single compiler — but requires defining "run until ip==join"
inside `v.exec`/`v.call` recursion, `OpReturn`-inside-arm capture, defer/label
edge cases, and meaningful work whenever the condition is concrete (normal
execution must not fork). Substantially more machinery for semantics symgo
doesn't need — it never joins on a *concrete* condition; it always wants both
sides. Only worth it if a future symgo2 wants KLEE-style path merging.

### Option C — keep the tree-walker, adopt only the substrate

Keep `symgo/evaluator`'s AST-walking shape but re-base it on minigo2's
resolve/index/runtime: `object.Object` → `runtime.Value`,
`getOrLoadPackage`/`ensurePackageEnvPopulated` → `Materialize`/`LazyInit`,
`scanner.Implements` → `TypeMethods`/`IfaceReqs`, `FieldType` →
`TypeDef`/type-expr resolution. Lowest risk, but preserves a second engine and
doesn't unify the intrinsic/special-form surfaces. A reasonable **Phase 0** of
Option A: it de-risks the type-model port (the real unknown) before touching
control flow.

### Option D — no rewrite

symgo stays a standalone engine; consumers that need DSLs use minigo2
specials alongside. Zero cost, keeps dual maintenance forever — and leaves
symgo pinned to the `scanner.TypeInfo` model while the rest of the ecosystem
moves to `TypeDef`.

## 6. Proposed implementation (Option A → symgo2)

Phase 0 — **substrate spike** (Option C, timeboxed): map `object`/`scanner`
concepts onto `runtime`/`index`; prove `Implements`-equivalence and
signature→typed-symbolic derivation on a few fixtures. Output: decision to
proceed.

Phase 1 — **symgo2 core**:
1. `runtime.Symbolic` + flag-gated tolerance in the ~15–20 value-op arms.
2. `compile.Trace` for `if`/`switch`/`select`/`for`/`range`/`type-switch`/
   `return`-in-arm (Option A control flow). Start with expression/statement
   forms that already compile; `OpTrap` is acceptable parity for
   never-supported forms (goto across arms, concrete `select` semantics).
3. `Hooks.CallWitness` (default-intrinsic) + intrinsic registry incl.
   receiver-keyed entries; `PushIntrinsics` scoping via a layered registry on
   the engine/session.
4. Symbolic-callee dispatch + bounded recursion (frame scan on `Decl.Pos` +
   receiver cell identity) + func-lit arg scan (N7).
5. `Cell.PossibleTypes` accumulation for interface-tagged cells; type-switch
   case instances.
6. `Finalize()` on the engine + an `Error` value (N11).

Phase 2 — **conformance**: port `symgo/evaluator/*_test.go` against symgo2
(the tests *are* the semantic spec — the analysis doc notes they verify
tracer, not interpreter, semantics).

Phase 3 — **consumer cutover**, in order of API simplicity:
`tools/goinspect` → `examples/call-trace` → `tools/find-orphans` (Finalize +
CallStack) → `examples/docgen` (deepest intrinsic/pattern usage). Deprecate
the tree-walker only after all four pass their existing tests on symgo2.

## 7. Risks and open questions

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
  `podhmo/minigo` ("code may lag behind the standalone repository"). symgo2
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

## 8. Effort estimate

Measured in sessions (not human-time): Phase 0 ~0.5–1 session; Phase 1 ~2–3
sessions; Phase 2 ~1–2 sessions; Phase 3 ~0.5–1 session + consumer validation.
Total **~4–6 sessions**, dominated by the type-model port and test-suite
conformance. Option B roughly doubles Phase 1–2. Option C alone is ~1–2
sessions but leaves two engines to maintain.
