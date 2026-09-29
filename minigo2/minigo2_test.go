package minigo2_test

import (
	"context"
	"strings"
	"testing"

	"github.com/podhmo/go-scan/minigo2"
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

func TestTrapOnCall(t *testing.T) {
	e := newEngine(t)
	// invariant: unsupported constructs compile fine, trap only when called
	if got := run(t, e, "./testdata/traponcall", "Good"); got != int64(7) {
		t.Fatalf("Good: got %v", got)
	}
	_, err := e.Run(context.Background(), "./testdata/traponcall", "Bad")
	if err == nil || !strings.Contains(err.Error(), "defer") {
		t.Fatalf("Bad: expected defer trap, got %v", err)
	}
	_, err = e.Run(context.Background(), "./testdata/traponcall", "Channy")
	if err == nil || !strings.Contains(err.Error(), "channel") {
		t.Fatalf("Channy: expected channel trap, got %v", err)
	}
}
