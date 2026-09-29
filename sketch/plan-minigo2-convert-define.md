# Migrating `examples/convert-define` from `minigo` to `minigo2`

> Status: conditions survey + migration plan. The `define` API itself is unchanged; this is an interpreter-swap under `internal/`.

## Goal

Replace every `minigo` (v1) dependency of `examples/convert-define` with `minigo2`,
keeping the user-visible behavior identical: `convert-define -file <defs.go>` runs the
DSL file's `main`, intercepts `define.Convert` / `define.Rule` calls with **quoted**
(unevaluated) arguments, and produces `model.ParsedInfo` for `generator.Generate`.

convert-define has **no `symgo` dependency** — the interpreter is the only `minigo`
surface it uses, so a full minigo2 migration is possible.

## What convert-define actually uses from v1 `minigo`

`internal/interpreter.go` touches four v1 surfaces:

| v1 surface | Purpose |
|---|---|
| `minigo.NewInterpreter(scanner)` | build an interpreter bound to a `goscan.Scanner` |
| `interp.RegisterSpecial("path.Sym", handler)` | intercept `define.Convert` / `define.Rule` calls |
| `interp.LoadFile(file); interp.Eval(ctx)` | load the DSL file and run its `main` |
| handler ctx: `*evaluator.Evaluator`, `*object.FileScope`, `pos`, `args []ast.Expr` | inside the special: read quoted args, map import aliases → paths, scan packages for `scanner.TypeInfo` / `scanner.FunctionInfo` |

The handlers never *evaluate* the DSL: `define.Convert`'s `*ast.FuncLit` argument is
walked wholesale (`c.Map`/`c.Convert`/`c.Compute` calls are data, not dispatched calls),
and `dst *destination.DstUser` type expressions are resolved to `scanner.TypeInfo` via
`fscope.Aliases` + `Scanner().ScanPackageFromImportPath`.

## Condition table — v1 surface → minigo2 mechanism

| # | Requirement | minigo2 mechanism | Status |
|---|---|---|---|
| 1 | Run the DSL file's `main` | `Engine.Run(ctx, dir, "main")` loads a **directory package**; DSL files carry `//go:build codegen` and are filtered out without matching `BuildConfig.Tags` | **gap** → file-level entry added: `Engine.LoadFile` / `Engine.RunFile` (the file is the package, constraints ignored) |
| 2 | Register `define.Convert` / `define.Rule` as quoted calls | `Engine.RegisterSpecial(runtime.SymbolID{PackagePath, Name}, handler)`; compiler emits `OpSpecialCall` for `alias.Name` resolving to a registered `SymbolID` — the `define` package is never located or parsed | implemented (round 4) |
| 3 | Quoted arguments (`[]ast.Expr`, FuncLit kept as AST) | `runtime.QuotedCall.Call.Args` — args are never compiled/evaluated; handler gets `ctx.Position`, `ctx.Format`, `ctx.Errorf` | implemented (round 4) |
| 4 | Import alias → import path (`fscope.Aliases[ident]`) | `ctx.Package().Scopes[ctx.File()][localName].Path` — built from the file's import table at parse time, no materialization. `SpecialContext.ResolveSymbol` (in-flight PR) will collapse this later | implemented (public `Scopes`) |
| 5 | `pkg.Type` / `pkg.Func` → `scanner.TypeInfo` / `scanner.FunctionInfo` | **host side**: keep the `goscan.Scanner` built by `NewRunner` and call `ScanPackageFromImportPath(path)`. minigo2's resolver is locator-level (`PackageMeta`) by design — it does not produce `scanner.TypeInfo` | no minigo2 change needed |
| 6 | `interp.Files()[0].AST.Name.Name` (package name of DSL file) | `pkg.Files[0].AST.Name.Name` on the `*runtime.Package` returned by `LoadFile` | implemented |
| 7 | `e.NewError(pos, ...)` | `ctx.Errorf(node, ...)` — position + message | implemented |
| 8 | `object.NIL` return from specials | `runtime.NIL` | implemented |
| 9 | `-tags` flag behavior | unchanged: the flag still only decorates generated output (`//go:build` header). The DSL file itself is loaded via `LoadFile`, which ignores build constraints for the named file | n/a |
| 10 | `main` as entry point | `Engine.Call(ctx, pkg, "main")` after `LoadFile` — same semantics as v1 `Eval` | via LoadFile |

## Decisions

- **File-level entry (`LoadFile`/`RunFile`) is the migration enabler.**
  The plan's entry model is directory packages (`minigo run ./app --entry F`), and
  `resolve.ReadPackageFiles` filters with `go/build` match rules. A DSL file guarded
  by `//go:build codegen` would need its tag mirrored into `BuildConfig.Tags` — brittle
  when the file sits in a directory with other non-DSL files. A named file is a
  self-contained package view: parse it, index it, run `main`. This matches v1
  `LoadFile` semantics exactly. Recorded as an out-of-plan addition in
  `plan-minigo-vm.md` (round-7 notes).
- **The `goscan.Scanner` stays on the host.** `model.StructInfo`/`TypeRule` are built
  from `scanner.TypeInfo`/`scanner.FieldType`; minigo2 deliberately has no scanner-level
  type API. `Runner.Scanner()` continues to serve `generator.Generate`.
- **`Scopes` over `ResolveSymbol`.** `ctx.Package().Scopes[ctx.File()]` already yields
  alias→path without materializing anything; when the in-flight `ResolveSymbol`
  `SpecialContext` method lands, the alias lookup can switch to it with no semantic
  change.
- **No method special forms.** `c.Map`/`c.Convert`/`c.Compute` inside the quoted
  `FuncLit` are walked as AST by the `define.Convert` handler — plan §12.7's
  recommendation, unchanged.

## Remaining work (tracked in TODO.md)

- `SpecialContext.Resolve` / `ResolveType` — still unimplemented; convert-define is now
  the first real consumer that would exercise them (it currently does alias→path +
  scanner itself).
- If `ResolveSymbol` lands on `SpecialContext`, convert-define's `importPathOf` helper
  can be reduced to `ctx.ResolveSymbol`.
