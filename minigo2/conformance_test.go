package minigo2_test

// Conformance harness: the same Go source is run under both minigo (v1
// tree-walking interpreter) and minigo2 (stack VM), and the entry-point
// results must match. Cases live in testdata/conformance/main.go; the v1
// run reads the same file as source so both engines see identical code.
//
// Documented divergences are skipped — e.g. single-threaded go/chan/select
// approximation and host-intrinsic packages (v1 has none).
import (
	"context"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/podhmo/go-scan/minigo"
	"github.com/podhmo/go-scan/minigo2/runtime"
)

// norm2 renders a minigo2 runtime value into the Go shape v1's Result.As
// produces, so both engines compare on equal footing.
func norm2(v runtime.Value) any {
	switch x := v.(type) {
	case *runtime.Cell:
		return norm2(x.Elem)
	case *runtime.Tuple:
		out := make([]any, len(x.Elems))
		for i, e := range x.Elems {
			out[i] = norm2(e)
		}
		return out
	case *runtime.Slice:
		out := make([]any, len(x.Elems))
		for i, e := range x.Elems {
			out[i] = norm2(e)
		}
		return out
	case runtime.Nil:
		return nil
	default:
		return v // int64, string, bool, float64 pass through
	}
}

// v1Result runs source under minigo v1 and normalizes the result to []any
// for multi-returns or a scalar otherwise.
func v1Result(t *testing.T, src []byte, entry string) any {
	t.Helper()
	res, err := minigo.Run(context.Background(), minigo.Options{
		Source:     src,
		EntryPoint: entry,
	})
	if err != nil {
		t.Fatalf("v1 Run(%s): %v", entry, err)
	}
	var got any
	if err := res.As(&got); err != nil {
		t.Fatalf("v1 As(%s): %v", entry, err)
	}
	return got
}

func TestConformance(t *testing.T) {
	src, err := os.ReadFile("testdata/conformance/main.go")
	if err != nil {
		t.Fatal(err)
	}
	e := newEngine(t)

	cases := []struct {
		fn    string
		args  []runtime.Value
		v1arg bool // v1 cannot take args: skip arg cases there
	}{
		// scalar results — no args (v1 Run has no arg passing)
		{fn: "FibNoArg"},   // fib(10) = 55, computed inside
		{fn: "LoopSum"},    // 45
		{fn: "RangeSlice"}, // 6
		{fn: "RangeMap"},   // 3 (v1 map iteration)
		{fn: "Closure"},    // 11
		{fn: "SwitchVal1"}, // 10 — wrappers since v1 can't pass args
		{fn: "SwitchVal2"}, // 20
		{fn: "UseMulti"},   // 34
		{fn: "StrCat"},     // "xxx"
	}

	for _, c := range cases {
		got2 := norm2(run(t, e, "./testdata/conformance", c.fn, c.args...))
		got1 := v1Result(t, src, c.fn)
		if diff := cmp.Diff(got1, got2); diff != "" {
			t.Errorf("%s: minigo vs minigo2 mismatch (-v1 +v2):\n%s", c.fn, diff)
		}
	}
}

// v1KnownDivergence documents where v1 and minigo2 legitimately disagree:
// v1's Result.As cannot unmarshal a defer-mutated named result (it comes
// back as an opaque RETURN_VALUE object) — v2 handles it correctly, so the
// harness only checks that v2 produces the Go-semantics value.
func TestConformanceV1Divergence(t *testing.T) {
	e := newEngine(t)
	// defer mutates the named result before return: 1 + 5 = 6
	if got := run(t, e, "./testdata/conformance", "DeferRun"); got != int64(6) {
		t.Fatalf("DeferRun: got %v, want 6", got)
	}
}
