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

## 9. Option A deep dive — concrete challenges and design decisions

### 9.1 The real spec: symgo's per-construct control-signal matrix

symgo's evaluators do **not** share one "evaluate the arm" rule — each
construct treats control signals differently, and the differences are
asymmetric. Reading `evaluator_eval_{if,for,range,switch,type_switch,select}_stmt.go`:

| Construct | Tag/cond evaluated? | Arms | `return` in arm | `Error` in arm | unlabeled `break`/`continue` |
|---|---|---|---|---|---|
| `if` | yes, result discarded | then + else, sequential | **dropped** (never propagates) | propagates (aborts fn) | propagate outward |
| `for` | init + cond (errors abort) | body once | dropped | propagates | absorbed (labeled → propagate) |
| `range` | `X` only | body once, fresh symbolic k/v | dropped | propagates | absorbed (labeled → propagate) |
| `switch` | tag + **each case's list exprs, re-evaluated per fallthrough chain** | per-case-start paths | **propagates** (aborts the whole switch *and* the function) | propagates | `break` ends this chain only (next case still runs); `continue` propagates |
| `type-switch` | guard | every case | dropped | **swallowed** (warn + continue) | n/a |
| `select` | every comm expr | every clause | dropped | **swallowed** | n/a |

Plus: `defer f()`/`go f()` evaluate the call immediately (no deferral);
`ch <- v` evaluates both operands but performs no send; `a && b`/`a || b`
evaluate **both** operands always (no short-circuit — `evalBinaryExpr` evals
`Y` unconditionally).

This table is the actual contract `compile.Trace` must implement. Notably it
contains real quirks — `return` inside `if` is dropped but inside `switch`
aborts exploration — that v1 symgo2 should **preserve verbatim** (the test
suite likely depends on them); normalize later only if consumers ask.

### 9.2 Emission spec for `compile.Trace`

Two sub-decisions:

**(a) Arm representation — recommend arm chunks.** Emit each arm body as a
synthetic zero-arg `runtime.Function` proto invoked via `OpCall`:

- env semantics map 1:1 onto the existing closure machinery — vars declared
  before the branch and written inside the arm become **upval cells** (shared,
  so `=` merges exactly like symgo's `Set` walk-up), `:=` inside the arm is a
  local slot (isolated). No new scope machinery needed.
- per-arm **panic/trap containment** comes free: the arm runs inside a
  `v.call`, and symbolic-mode `v.call` recovers `Panic`/`Trap` into a signal
  value — which is precisely what `type-switch`/`select`'s swallow-errors
  semantics need.
- `return` inside an arm can't use `OpReturn` (it would end the enclosing
  function), so arms return **signal values** instead.

Flat-inline emission (arm code spliced into the parent chunk, merge = shared
slots) is cheaper per arm but gives no arm-level error containment — wrong for
swallow-semantics constructs and asymmetric anyway. Uniform arm-chunks win.

**(b) Signal encoding — mirror symgo's signal objects as runtime values.**
`runtime.Signal{Kind, Value, Label}` with kinds Return/Error/Break/Continue/
Fallthrough. Two new opcodes suffice for the whole matrix:

- `OpSignal` — pop TOS, return `Signal{kind, v, label}` from the current frame
  (used by trace-mode `return`/`break`/`continue`/`fallthrough` inside arms).
- `OpArmDispatch{consumeSet}` — after each arm's `OpCall`: result not a signal
  → pop; signal in `consumeSet` → pop (absorbed at this boundary — labeled
  Break/Continue only match when their label names this construct); signal not
  consumed → propagate by returning it from the enclosing frame. Signals thus
  bubble up through nested arm frames exactly like symgo's signal objects
  bubble through nested `Eval` calls — the dispatch chain *is* symgo's
  per-construct `switch result.(type)`.

One boundary rule completes it: when an `OpArmDispatch` propagates a
`Return`/`Error` signal out of the *outermost* arm of the function, it unwraps
to a real `OpReturn` value (or, for Error, the call's error result) instead of
leaking the Signal to the caller.

The per-construct `consumeSet`s encode §9.1 directly — *including* symgo's
label-handling quirks: `if` = {Return} (everything else propagates), `switch`
= {Break} (symgo absorbs `break` in a case arm regardless of label — a `break
L` to an outer loop inside a case is wrongly swallowed today; preserve or fix,
deliberately), `type-switch`/`select` = {all kinds} (swallow), `for`/`range` =
{unlabeled Break, unlabeled Continue} (labeled ones propagate). `fallthrough`
never materializes — the parent emits the next case's body inline in the
chain.

Other trace-mode deltas to `compile.Func` emission, all small and enumerable:

- `if`/`for`/`switch` conditions: eval + `OpPop` (trace calls, discard result)
- `for`: no back-edge, no `Post` (symgo skips `Post`)
- `range`: `X` eval + `OpPop`; k/v bound as `Symbolic` (typed by elem-of-`X`
  when `X`'s typedef is known — `ElemOf` hook)
- `switch`: emit per-case-start chains — for i in 0..n-1: `[eval caseList[j] +
  OpPop]` then `[body[j]]` for j = i.. while case j ends in `fallthrough`.
  O(n²) emission, faithful to symgo's repeated case-expr evaluation
- `type-switch`: guard eval; per case bind `Symbolic{Typ: caseTd}` (resolved
  via the case expr's typeExpr → `TypeDef`); `StructKind` case → fresh empty
  `*runtime.Struct` (matching symgo's `object.Instance`), `default` → copy of
  the guard value (`valueCopy` already exists), unresolved/`UnknownKind` →
  interface-ish `Symbolic` (symgo forces `UnknownKind`→`InterfaceKind`)
- `defer`/`go` → plain `OpCall` + `OpPop` (evaluate now, discard)
- `ch <- v`, `select` comms → operand evals + pops, no channel op
- `&&`/`||` → `expr(X); expr(Y); OpBinary{LAnd|LOr}` (no short-circuit)
- `v, ok :=` comma-ok / type-assert on a `Symbolic` → `Tuple{Symbolic{caseTd},
  true}`; on concrete → normal op
- `x := f()` where `f` is out-of-policy → symbolic callee result; `Typ` on the
  cell from the declared type (the `Cell.Typ` declared-tag machinery already
  exists)

The compiler delta is therefore: a `trace bool` flag (or `Trace` entrypoint
reusing `compiler`) flipping ~12 statement/expression forms, arm-chunk
synthesis (reuse `funcLit` capture analysis), and two new ops. No VM
control-flow changes beyond `v.call` behavior.

### 9.3 Call-boundary pipeline (expanded N4)

In symbolic mode, `v.call` runs this ordered pipeline — a direct transliteration
of `evalCallExpr` + `applyFunction`:

1. **lit-scan**: for each `*Closure`/`*Function` arg carrying `Lit`/`Decl`,
   invoke it once with signature-typed `Symbolic` params (scanFunctionLiteral).
   Runs *before* the witness so nested usage is marked first, as today.
2. **`Hooks.CallWitness(fn, args)`** — the `defaultIntrinsic` replacement; also
   the funnel for memoization.
3. **Intrinsic dispatch**: registry lookup — `"pkg.Path.Fn"` (≈ `SymbolID`) and
   `"pkg.T.M"`/`"(*pkg.T).M"` receiver forms; hit → run handler, return its
   result. Handlers get a `VMCaller`-like context supporting reentrant `Apply`
   (docgen's `pattern.Apply` needs it).
4. **Policy dispatch**: callee's package out-of-policy → synthesize
   `Symbolic`s from `Decl.Type.Results` (Tuple when >1), return without a frame.
5. **Recursion bound**: frame scan for `Decl.Pos` collision + receiver-cell
   identity (method recursion allowed on different receivers); HOF recursion
   via closure identity (BoundCallStack analog). Hit → signature-typed
   `Symbolic` result.
6. Normal `prepFrame` + `loop`.
7. **Recover**: `Panic`/`Trap` → `Signal{Error}` result (arm-call containment).

Interface method on a symbolic receiver: `selectMember` yields a
symbolic-method value carrying `{ifaceTd, name, PossibleTypes}`; `v.call` on it
records `calledInterfaceMethods[iface.name]` and, for each concrete `T` in
`PossibleTypes`, marks `T.m` used through the witness (symgo's per-member
concrete call marking), then returns a signature-typed `Symbolic`.
`Finalize()` = engine pass: for each recorded interface call, enumerate
implementers across in-policy packages via `TypeMethods`+`IfaceReqs` and mark
their methods.

### 9.4 Facade mapping (`Eval`/`Apply`)

- `Eval(fileAst, pkg)` ≈ `LoadFile` + a new `MaterializeFile(file)` driver:
  force every decl of the file in order under trace mode — symgo's
  `evalGenDecl` evaluates var initializers **eagerly** at `Eval` time (the
  laziness is only for *imported* packages), so a global never touched by
  traced code still gets traced today. Lazy `Globals.Get` alone would miss
  those traces — the driver must force-eval, not wait for access.
- `Apply(fn, args, pkg)` → `v.call(fn, symbolicArgs)`; args built from
  `fn.Decl.Type.Params` → `typeExpr`→`TypeDef`→`Symbolic{Typ}` (the standalone
  `podhmo/minigo` `Sig`/`TypeExpr` inspect accessors are exactly this).
- `FindObjectInPackage` → index lookup + materialize. `CallStack` → VM frame
  introspection. `Files()`/`EvaluatorForTest` → thin shims.

### 9.5 Hard cases and open questions

1. **Init/var cycles** (`var a = f(); f() reads a`): symgo's
   `evaluationInProgress` guard → placeholder; minigo2's lazy `Globals` need
   the same guard → `Symbolic`, not a trap.
2. **`iota`/const specs**: symgo evaluates each spec with `iota` bound in env;
   verify minigo2's const materialization matches.
3. **Generics**: `inferBinds` on `Symbolic` args will fail — degrade to a
   signature-typed `Symbolic` result rather than trapping.
4. **Tracer granularity**: symgo emits per-AST-node events; the VM offers
   per-instruction `Pos` + the call witness. Consumers use calls/stacks —
   accept coarser visit granularity, validate against docgen's `WithTracer`.
5. **Repeated case-expr evaluation**: faithful O(n²) emission reproduces
   symgo's duplicate side effects — sets dedupe fine, but stateful intrinsics
   (counters) see repeats; document.
6. **`goto`/labels across arms**: `pendingGotos` resolve against emitted code;
   a goto into a "dead" arm becomes a jump into arm-chunk — out of scope;
   trap it (symgo has `evalLabeledStmt`, keep as coverage-floor item).
7. **`recover()`**: symgo treats it as an unknown call → placeholder; on the
   VM it's a builtin — under symbolic mode return `Symbolic`/false-equivalent.
8. **Coverage-floor grind**: every node `compile.Func` traps on is a candidate
   "emit `Symbolic` push instead" conversion in trace mode. Expect a tail —
   the symgo test suite is the checklist.
9. **Preserve-vs-normalize**: the §9.1 quirks (if-drops-return vs
   switch-propagates-return, error swallow vs propagate) may themselves be
   symgo bugs — decide deliberately: preserve in v1 for test parity, normalize
   behind a flag later.
10. **`Files()`/file-scope identity**: symgo's `FileScope` tracks per-file
    imports; minigo2's `p.Scopes[file]` is the same thing — direct map.

### 9.6 Spike order (what to build first to de-risk)

1. `runtime.Symbolic` + flag-gated tolerance in the value ops + `OpSignal` /
   `OpArmDispatch` + arm-chunks for **`if` only** — drive one fixture
   end-to-end. If the env-merge and containment semantics hold here, the rest
   is repetition.
2. The `v.call` pipeline (witness + intrinsic keys + policy stub + recursion
   bound) — this is where all consumer hooks converge.
3. `MaterializeFile` + `Apply` facade; point `tools/goinspect` at it.
4. Then the per-construct grind + conformance port, in the §6 order.
