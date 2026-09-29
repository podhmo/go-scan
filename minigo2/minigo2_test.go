package minigo2_test

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo2"
	"github.com/podhmo/go-scan/minigo2/index"
	"github.com/podhmo/go-scan/minigo2/runtime"
)

func newEngine(t *testing.T) *minigo2.Engine {
	t.Helper()
	return minigo2.NewEngine("..")
}

func run(t *testing.T, e *minigo2.Engine, ref, fn string, args ...runtime.Value) runtime.Value {
	t.Helper()
	v, err := e.Run(context.Background(), ref, fn, args...)
	if err != nil {
		t.Fatalf("Run(%s, %s): %v", ref, fn, err)
	}
	return v
}

func TestEntryPoints(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		fn   string
		want any
	}{
		{"Answer", int64(67)}, // fib(10)=55 + Global=10 + B=2
		{"SumRange", int64(0 + 1 + 1 + 2 + 2 + 3 + 3 + 4)},
		{"Methods", int64(7)},
		{"Closure", int64(3)},
		{"Multi", int64(43)},
		{"Mapy", int64(5)},
		{"Consts", int64(6)},
		{"Strings", "hello!!!"},
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/t1", c.fn)
		if got != c.want {
			t.Errorf("%s: got %v (%T), want %v (%T)", c.fn, got, got, c.want, c.want)
		}
	}
}

func TestSwitchy(t *testing.T) {
	e := newEngine(t)
	for in, want := range map[int64]int64{1: 10, 2: 10, 3: 30, 99: -1} {
		got, err := e.Run(context.Background(), "./testdata/t1", "Switchy", in)
		if err != nil {
			t.Fatalf("Switchy(%d): %v", in, err)
		}
		if got != want {
			t.Errorf("Switchy(%d) = %v, want %d", in, got, want)
		}
	}
}

func TestLazyImport(t *testing.T) {
	e := newEngine(t)
	// OK never references lazyboom -> its panicking init must not run
	if got := run(t, e, "./testdata/lazyuser", "OK"); got != int64(1) {
		t.Fatalf("OK: got %v", got)
	}
	// Bad touches lazyboom.Get -> package init runs -> panic surfaces
	_, err := e.Run(context.Background(), "./testdata/lazyuser", "Bad")
	if err == nil || !strings.Contains(err.Error(), "BOOM") {
		t.Fatalf("Bad: expected BOOM panic, got %v", err)
	}
}

func TestBlankImportInitializes(t *testing.T) {
	e := newEngine(t)
	_, err := e.Run(context.Background(), "./testdata/blankimport", "OK")
	if err == nil || !strings.Contains(err.Error(), "BOOM") {
		t.Fatalf("blank import must initialize before entry: %v", err)
	}
}

func TestDotImports(t *testing.T) {
	e := newEngine(t)
	for _, tc := range []struct {
		name string
		want runtime.Value
	}{
		{name: "Greeting", want: "hi x"},
		{name: "Number", want: int64(11)},
	} {
		got := run(t, e, "./testdata/dotimports", tc.name)
		if diff := cmp.Diff(tc.want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", tc.name, diff)
		}
	}
	_, err := e.Run(context.Background(), "./testdata/dotimports", "TouchBoom")
	if err == nil || !strings.Contains(err.Error(), "BOOM") {
		t.Fatalf("dot-imported member must initialize its package: %v", err)
	}
}

func TestDepOrderAndFixes(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		fn   string
		want any
	}{
		{"Dep", int64(2)},          // var B = A+1 before var A — dep order
		{"ImportInit", "hi x"},     // import inside a package-level init expr
		{"ForContinue", int64(25)}, // 1+3+5+7+9
		{"RangeOne", int64(12)},    // single-var range yields indexes 0,1,2
		{"Redefine", int64(56)},    // x,y := keeps existing x binding
		{"StructCopy", int64(1)},   // b := a copies struct value
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/deporder", c.fn)
		if got != c.want {
			t.Errorf("%s: got %v (%T), want %v (%T)", c.fn, got, got, c.want, c.want)
		}
	}
}

func TestTrapOnCall(t *testing.T) {
	e := newEngine(t)
	// invariant: unsupported constructs compile fine, trap only when called
	if got := run(t, e, "./testdata/traponcall", "Good"); got != int64(7) {
		t.Fatalf("Good: got %v", got)
	}
	_, err := e.Run(context.Background(), "./testdata/traponcall", "Bad")
	if err == nil || !strings.Contains(err.Error(), "3-index") {
		t.Fatalf("Bad: expected 3-index-slice trap, got %v", err)
	}
	_, err = e.Run(context.Background(), "./testdata/traponcall", "Channy")
	if err == nil || !strings.Contains(err.Error(), "label not defined") {
		t.Fatalf("Channy: expected undefined-label trap, got %v", err)
	}
	// fallthrough + type assert work now: both old traps became real code
	if got := run(t, e, "./testdata/traponcall", "FallthroughAndAssert"); got != int64(16) {
		t.Fatalf("FallthroughAndAssert: got %v", got)
	}
}

func TestDeferRecover(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		fn   string
		want runtime.Value
	}{
		{"DeferOrder", int64(321)},        // defers run LIFO
		{"DeferArgCaptured", int64(1)},    // args evaluated at defer time
		{"NamedResultDefer", int64(15)},   // defer mutates named result
		{"RecoverValue", "boom"},          // defer+recover captures panic
		{"RecoverOutsideDefer", int64(1)}, // recover outside defer is nil
		{"StillPanic", runtime.NIL},       // recovered panic returns normally
		{"DeferBuiltinClose", int64(2)},   // deferred builtin runs at teardown
		{"DeferBuiltinRecover", int64(3)}, // defer recover() catches the panic
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/deferchan", c.fn)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", c.fn, diff)
		}
	}
	// a panic raised inside a defer propagates
	_, err := e.Run(context.Background(), "./testdata/deferchan", "ReraiseReplace")
	if err == nil || !strings.Contains(err.Error(), "second") {
		t.Fatalf("ReraiseReplace: expected 'second' panic, got %v", err)
	}
}

func TestChanSelect(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		fn   string
		want runtime.Value
	}{
		{"ChanQueue", int64(321)},     // FIFO send/receive
		{"ChanCommaOk", int64(1)},     // comma-ok on non-empty
		{"ChanClosedRecv", int64(42)}, // closed empty recv -> nil,false
		{"ChanRange", int64(60)},      // range drains the queue
		{"GoSync", int64(12)},         // go f() runs synchronously
		{"GoChanRoundtrip", int64(42)},
		{"SelectRecv", int64(5)},
		{"SelectDefault", int64(9)},
		{"SelectCommaOk", int64(3)},
		{"SelectSend", int64(11)},
		{"SelectConsumeBare", int64(2)},   // bare case <-ch consumes
		{"SelectEvalOrder", int64(11)},    // all operands eval on entry
		{"SelectSendEvalOrder", int64(7)}, // send chan+value eval too
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/deferchan", c.fn)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", c.fn, diff)
		}
	}
}

func TestInitOrderThroughFunc(t *testing.T) {
	e := newEngine(t)
	// var x = f() where f reads var y declared later: y must initialize first
	if got := run(t, e, "./testdata/initorder", "Answer"); got != int64(14) {
		t.Fatalf("Answer: got %v, want 14", got)
	}
}

func TestStdlibIntrinsics(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		fn   string
		want runtime.Value
	}{
		{"Sprintf", "hi 7"},        // real fmt.Sprintf without GOROOT parse
		{"StrconvAtoi", int64(42)}, // (val, err) tuple
		{"StringsJoin", "a,b,c"},
		{"ErrorsNew", "oops"},
		{"SortIntsInPlace", int64(123)}, // sort mutates the slice
		{"SlicesSortInPlace", "abc"},    // slices.Sort mutates too
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/intrins", c.fn)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", c.fn, diff)
		}
	}
}

func TestEvalExpr(t *testing.T) {
	e := newEngine(t)
	pkg, err := e.Package(context.Background(), "./testdata/t1")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fset, "eval.go", "Global + 2", 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.EvalExpr(context.Background(), pkg, nil, expr)
	if err != nil {
		t.Fatalf("EvalExpr: %v", err)
	}
	if diff := cmp.Diff(int64(12), got); diff != "" {
		t.Errorf("EvalExpr mismatch (-want +got):\n%s", diff)
	}
}

func TestAllowedRoots(t *testing.T) {
	// Roots pinned to ./testdata: t1 lives inside and must run, /tmp refuses.
	td, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}
	e := minigo2.NewEngine("..", minigo2.WithAllowedRoots(td))
	if _, err := e.Run(context.Background(), "./testdata/t1", "Answer"); err != nil {
		t.Fatalf("inside allowed root should run: %v", err)
	}
	outside := filepath.Dir(os.TempDir())
	if strings.HasPrefix(td, outside+string(filepath.Separator)) || td == outside {
		outside = "/"
	}
	if _, err := e.Run(context.Background(), outside, "main"); err == nil ||
		!strings.Contains(err.Error(), "outside the allowed roots") {
		t.Fatalf("expected outside-root rejection, got %v", err)
	}
}

func TestLazyInitMode(t *testing.T) {
	// LazyInit answers function/type queries without running initializers:
	// lazyboom's panicking init must NOT run when we only ask for its Func.
	e := minigo2.NewEngine("..", minigo2.WithInitMode(minigo2.LazyInit))
	pkg, err := e.Package(context.Background(), "./testdata/lazyboom")
	if err != nil {
		t.Fatal(err)
	}
	stub := func(p *runtime.Package, d *index.Decl) (runtime.Value, error) {
		return runtime.NIL, nil
	}
	if _, err := pkg.Member("Get", stub); err != nil {
		t.Fatalf("Member(Get): %v", err)
	}
	if pkg.State == runtime.Ready {
		t.Fatal("LazyInit Member must not initialize the package")
	}
	// GoCompatibleInit (default) surfaces the panic at member touch.
	e2 := newEngine(t)
	pkg2, err := e2.Package(context.Background(), "./testdata/lazyboom")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pkg2.Member("Get", stub); err == nil || !strings.Contains(err.Error(), "BOOM") {
		t.Fatalf("eager mode should surface init panic, got %v", err)
	}
}

func TestInitFailureSurfaces(t *testing.T) {
	// An initializer that panics after registering some globals must not
	// leave partial state answerable: Member returns the init error.
	e := newEngine(t)
	pkg, err := e.Package(context.Background(), "./testdata/initfail")
	if err != nil {
		t.Fatal(err)
	}
	stub := func(p *runtime.Package, d *index.Decl) (runtime.Value, error) {
		return runtime.NIL, nil
	}
	if _, err := pkg.Member("Good", stub); err == nil ||
		!strings.Contains(err.Error(), "init went wrong") {
		t.Fatalf("partial init state must surface the failure, got %v", err)
	}
	if _, err := e.Run(context.Background(), "./testdata/initfail", "Use"); err == nil ||
		!strings.Contains(err.Error(), "init went wrong") {
		t.Fatalf("Run on a failed package must surface the init error, got %v", err)
	}
}

func TestOsHostSurface(t *testing.T) {
	// Restricted engines (AllowedRoots set) do not get os.Getenv/os.Args.
	td, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}
	e := minigo2.NewEngine("..", minigo2.WithAllowedRoots(td))
	if _, err := e.Run(context.Background(), "./testdata/hostenv", "Read"); err == nil {
		t.Fatal("os.Getenv must be unbound under AllowedRoots")
	}
	// os.Exit never terminates the host, in any mode.
	if _, err := newEngine(t).Run(context.Background(), "./testdata/hostenv", "Exit"); err == nil ||
		!strings.Contains(err.Error(), "cannot terminate the host") {
		t.Fatalf("os.Exit must trap, got %v", err)
	}
}

func TestFeatures(t *testing.T) {
	e := newEngine(t)
	cases := []struct {
		fn   string
		want runtime.Value
	}{
		// spread calls
		{"Spread", int64(36)},
		{"SpreadTail", int64(8)},
		// compound assign & ++/-- on fields, indices, derefs
		{"CompoundOps", int64(172)},
		{"PtrOps", int64(12)},
		// labels, break/continue L, goto
		{"LabeledBreak", int64(3)},
		{"LabeledContinue", int64(16)},
		{"GotoSkip", int64(5)},
		{"LabeledSwitch", int64(3)},
		{"SelectLabel", int64(2)},
		// interfaces: dispatch, embedding, satisfaction
		{"InterfaceDispatch", int64(16)},
		{"EmbeddedMethod", int64(25)},
		{"IfaceHolds", int64(1)},
		// assertions + type switches
		{"AssertFail", false},
		{"TypeSwitch", int64(120)},
		{"TypeSwitchBind", "hey!"},
		{"AssertPanic", int64(99)},
		{"NilAssert", int64(1)},
		// generics
		{"GenericFns", int64(42)},
		{"GenericConvert", int64(42)},
		{"GenericType", int64(42)},
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/features", c.fn)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", c.fn, diff)
		}
	}
	// comma-ok assert returns a tuple
	got := run(t, e, "./testdata/features", "AssertOK")
	tp, ok := got.(*runtime.Tuple)
	if !ok || len(tp.Elems) != 2 || tp.Elems[0] != int64(2) || tp.Elems[1] != true {
		t.Fatalf("AssertOK: got %v", got)
	}
}

func TestSpecialForms(t *testing.T) {
	e := minigo2.NewEngine("..")
	e.Bind("example.com/dsl", map[string]runtime.Value{})

	twice := func(ctx runtime.SpecialContext, call *runtime.QuotedCall) (runtime.Value, error) {
		v, err := ctx.Eval(call.Call.Args[0])
		if err != nil {
			return nil, err
		}
		n, ok := v.(int64)
		if !ok {
			return nil, ctx.Errorf(call.Call, "Twice arg is %T, want int", v)
		}
		return n * 2, nil
	}
	e.RegisterSpecial(runtime.SymbolID{PackagePath: "example.com/dsl", Name: "Twice"}, twice)
	e.RegisterSpecial(runtime.SymbolID{PackagePath: "example.com/dsl", Name: "Show"},
		func(ctx runtime.SpecialContext, call *runtime.QuotedCall) (runtime.Value, error) {
			// the quoted arg renders as source, never evaluated
			return ctx.Format(call.Call.Args[0]), nil
		})
	e.RegisterSpecial(runtime.SymbolID{PackagePath: "example.com/dsl", Name: "Skipped"},
		func(ctx runtime.SpecialContext, call *runtime.QuotedCall) (runtime.Value, error) {
			return int64(42), nil // arg never evaluated -> no panic
		})

	cases := []struct {
		fn   string
		want runtime.Value
	}{
		{"TwiceIt", int64(44)}, // Eval sees caller's local x=21
		{"Quoted", "y + 1"},    // quoted, unevaluated source
		{"Lazy", int64(42)},    // handler never Evals -> boom() never runs
	}
	for _, c := range cases {
		got := run(t, e, "./testdata/special", c.fn)
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s mismatch (-want +got):\n%s", c.fn, diff)
		}
	}
}

func TestHostPolicy(t *testing.T) {
	// deny the whole os package: bound but empty — script sees "undefined"
	e := minigo2.NewEngine("..", minigo2.WithHostPolicy(func(path, sym string) bool {
		return path != "os"
	}))
	if got := run(t, e, "./testdata/hostpolicy", "PolicyOK"); got != "OK" {
		t.Fatalf("PolicyOK: got %v", got)
	}
	_, err := e.Run(context.Background(), "./testdata/hostpolicy", "PolicyDenied")
	if err == nil || !strings.Contains(err.Error(), "Getenv") {
		t.Fatalf("os.Getenv must be denied by policy, got %v", err)
	}
}
