// Stdlib intrinsics: a small native surface bound into every engine so
// interpreted code doesn't have to parse GOROOT sources for the common
// cases. Script values are marshalled to Go natives at the boundary
// (goNative) so the real fmt/strconv/... implementations do the work.
// Errors return Go-style (value, err) tuples where the real API has one.
//
// Deliberately small — anything absent falls through to lazy GOROOT source
// interpretation, which stays the documented path for full fidelity.
package minigo2

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/podhmo/go-scan/minigo2/runtime"
)

// installStdlib binds the intrinsic packages onto the engine's import-path
// table; Bound packages win over source resolution (loadPath checks pkgs).
func (e *Engine) installStdlib() {
	h := &hostHelpers{v: e.vmm, e: e}
	e.Bind("fmt", map[string]runtime.Value{
		"Print":   h.fn("fmt.Print", func(a []any) (any, error) { return retErr(fmt.Fprint(h.out(), a...)) }),
		"Println": h.fn("fmt.Println", func(a []any) (any, error) { return retErr(fmt.Fprintln(h.out(), a...)) }),
		"Printf": h.fn2("fmt.Printf", func(a []any) (any, error) {
			return retErr(fmt.Fprintf(h.out(), str(a[0]), a[1:]...))
		}),
		"Sprint":   h.fn("fmt.Sprint", func(a []any) (any, error) { return fmt.Sprint(a...), nil }),
		"Sprintln": h.fn("fmt.Sprintln", func(a []any) (any, error) { return fmt.Sprintln(a...), nil }),
		"Sprintf": h.fn2("fmt.Sprintf", func(a []any) (any, error) {
			return fmt.Sprintf(str(a[0]), a[1:]...), nil
		}),
		"Errorf": h.fn2("fmt.Errorf", func(a []any) (any, error) {
			return fmt.Errorf(str(a[0]), a[1:]...), nil
		}),
	})
	e.Bind("errors", map[string]runtime.Value{
		"New": h.fn("errors.New", func(a []any) (any, error) { return errors.New(str(a[0])), nil }),
		"Join": h.fn("errors.Join", func(a []any) (any, error) {
			var errs []error
			for _, v := range a {
				if err := asErr(v); err != nil {
					errs = append(errs, err)
				}
			}
			return errVal(errors.Join(errs...)), nil
		}),
		"Is": h.fn2("errors.Is", func(a []any) (any, error) {
			return errors.Is(asErr(a[0]), asErr(a[1])), nil
		}),
		"Unwrap": h.fn("errors.Unwrap", func(a []any) (any, error) {
			return errVal(errors.Unwrap(asErr(a[0]))), nil
		}),
		// As is approximated: the script cannot spell the target type, so it
		// assigns the first non-nil cause in the chain to *target.
		"As": &runtime.BuiltinFunc{Name: "errors.As", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("errors.As needs 2 args")
			}
			for err := asErr(goNative(args[0])); err != nil; err = errors.Unwrap(err) {
				if runtime.SetRef(args[1], errVal(err)) {
					return true, nil
				}
				return nil, fmt.Errorf("errors.As: target must be a pointer (cell)")
			}
			return false, nil
		}},
	})
	e.Bind("strings", map[string]runtime.Value{
		"Contains":    h.fn2("strings.Contains", func(a []any) (any, error) { return strings.Contains(str(a[0]), str(a[1])), nil }),
		"ContainsAny": h.fn2("strings.ContainsAny", func(a []any) (any, error) { return strings.ContainsAny(str(a[0]), str(a[1])), nil }),
		"Compare":     h.fn2("strings.Compare", func(a []any) (any, error) { return int64(strings.Compare(str(a[0]), str(a[1]))), nil }),
		"Replace":     h.fn3("strings.Replace", func(a []any) (any, error) { return strings.Replace(str(a[0]), str(a[1]), str(a[2]), intOf(a[3])), nil }),
		"Cut": h.fn2("strings.Cut", func(a []any) (any, error) {
			b, af, ok := strings.Cut(str(a[0]), str(a[1]))
			return &runtime.Tuple{Elems: []runtime.Value{b, af, ok}}, nil
		}),
		"CutPrefix": h.fn2("strings.CutPrefix", func(a []any) (any, error) {
			af, ok := strings.CutPrefix(str(a[0]), str(a[1]))
			return &runtime.Tuple{Elems: []runtime.Value{af, ok}}, nil
		}),
		"CutSuffix": h.fn2("strings.CutSuffix", func(a []any) (any, error) {
			bf, ok := strings.CutSuffix(str(a[0]), str(a[1]))
			return &runtime.Tuple{Elems: []runtime.Value{bf, ok}}, nil
		}),
		"HasPrefix":   h.fn2("strings.HasPrefix", func(a []any) (any, error) { return strings.HasPrefix(str(a[0]), str(a[1])), nil }),
		"HasSuffix":   h.fn2("strings.HasSuffix", func(a []any) (any, error) { return strings.HasSuffix(str(a[0]), str(a[1])), nil }),
		"Index":       h.fn2("strings.Index", func(a []any) (any, error) { return int64(strings.Index(str(a[0]), str(a[1]))), nil }),
		"Join":        h.fn2("strings.Join", func(a []any) (any, error) { return strings.Join(strSlice(a[0]), str(a[1])), nil }),
		"Split":       h.fn2("strings.Split", func(a []any) (any, error) { return strsSlice(strings.Split(str(a[0]), str(a[1]))), nil }),
		"ToUpper":     h.fn("strings.ToUpper", func(a []any) (any, error) { return strings.ToUpper(str(a[0])), nil }),
		"ToLower":     h.fn("strings.ToLower", func(a []any) (any, error) { return strings.ToLower(str(a[0])), nil }),
		"TrimSpace":   h.fn("strings.TrimSpace", func(a []any) (any, error) { return strings.TrimSpace(str(a[0])), nil }),
		"ReplaceAll":  h.fn3("strings.ReplaceAll", func(a []any) (any, error) { return strings.ReplaceAll(str(a[0]), str(a[1]), str(a[2])), nil }),
		"Repeat":      h.fn2("strings.Repeat", func(a []any) (any, error) { return strings.Repeat(str(a[0]), intOf(a[1])), nil }),
		"Builder":     &runtime.TypeDef{Name: "Builder", Kind: runtime.KindNamedBasic},
		"NewReplacer": h.fn2("strings.NewReplacer", func(a []any) (any, error) { return strings.NewReplacer(strSlice(a[0])...), nil }),
		"Fields":      h.fn("strings.Fields", func(a []any) (any, error) { return strsSlice(strings.Fields(str(a[0]))), nil }),
		"EqualFold":   h.fn2("strings.EqualFold", func(a []any) (any, error) { return strings.EqualFold(str(a[0]), str(a[1])), nil }),
		"Count":       h.fn2("strings.Count", func(a []any) (any, error) { return int64(strings.Count(str(a[0]), str(a[1]))), nil }),
	})
	e.Bind("strconv", map[string]runtime.Value{
		"Atoi":    h.fn("strconv.Atoi", func(a []any) (any, error) { return retErr2(strconv.Atoi(str(a[0]))) }),
		"Itoa":    h.fn("strconv.Itoa", func(a []any) (any, error) { return strconv.Itoa(intOf(a[0])), nil }),
		"Quote":   h.fn("strconv.Quote", func(a []any) (any, error) { return strconv.Quote(str(a[0])), nil }),
		"Unquote": h.fn("strconv.Unquote", func(a []any) (any, error) { return retErr2(strconv.Unquote(str(a[0]))) }),
		"ParseUint": h.fn3("strconv.ParseUint", func(a []any) (any, error) {
			return retErr2(strconv.ParseUint(str(a[0]), intOf(a[1]), intOf(a[2])))
		}),
		"FormatFloat": h.fn3("strconv.FormatFloat", func(a []any) (any, error) {
			f, _ := a[0].(float64)
			return strconv.FormatFloat(f, byte(intOf(a[1])), intOf(a[2]), 64), nil
		}),
		"FormatBool": h.fn("strconv.FormatBool", func(a []any) (any, error) {
			b, _ := a[0].(bool)
			return strconv.FormatBool(b), nil
		}),
		"ParseInt": h.fn3("strconv.ParseInt", func(a []any) (any, error) { return retErr2(strconv.ParseInt(str(a[0]), intOf(a[1]), intOf(a[2]))) }),
		"ParseFloat": h.fn2("strconv.ParseFloat", func(a []any) (any, error) {
			return retErr2(strconv.ParseFloat(str(a[0]), intOf(a[1])))
		}),
		"ParseBool": h.fn("strconv.ParseBool", func(a []any) (any, error) { return retErr2(strconv.ParseBool(str(a[0]))) }),
		"FormatInt": h.fn2("strconv.FormatInt", func(a []any) (any, error) { return strconv.FormatInt(int64Of(a[0]), intOf(a[1])), nil }),
	})
	e.Bind("sort", map[string]runtime.Value{
		"Ints":     h.sortInPlace("sort.Ints"),
		"Float64s": h.sortInPlace("sort.Float64s"),
		"Strings":  h.sortInPlace("sort.Strings"),
		"Slice":    h.sortSlice,
		"SliceIsSorted": &runtime.BuiltinFunc{Name: "sort.SliceIsSorted", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			s, ok := args[0].(*runtime.Slice)
			if !ok {
				return nil, fmt.Errorf("sort.SliceIsSorted: first arg must be a slice")
			}
			less := args[1]
			for i := len(s.Elems) - 1; i > 0; i-- {
				r, err := h.v.Call(less, []runtime.Value{int64(i), int64(i - 1)})
				if err != nil {
					return nil, err
				}
				if b, _ := r.(bool); b {
					return false, nil
				}
			}
			return true, nil
		}},
	})
	e.Bind("slices", map[string]runtime.Value{
		"Sort": h.sortInPlace("slices.Sort"),
		"Contains": h.fn2("slices.Contains", func(a []any) (any, error) {
			return slices.Contains(anySlice(a[0]), a[1]), nil
		}),
		"Index": h.fn2("slices.Index", func(a []any) (any, error) {
			return int64(slices.Index(anySlice(a[0]), a[1])), nil
		}),
		"Clone": h.fn("slices.Clone", func(a []any) (any, error) {
			return slices.Clone(anySlice(a[0])), nil
		}),
		"Concat": h.fn("slices.Concat", func(a []any) (any, error) {
			var parts [][]any
			for _, p := range a {
				parts = append(parts, anySlice(p))
			}
			return slices.Concat(parts...), nil
		}),
		"Equal": h.fn2("slices.Equal", func(a []any) (any, error) {
			return slices.Equal(anySlice(a[0]), anySlice(a[1])), nil
		}),
		"IsSorted": h.fn("slices.IsSorted", func(a []any) (any, error) {
			el := scriptElems(a[0])
			return sort.SliceIsSorted(el, func(i, j int) bool { return lessScript(el[i], el[j]) }), nil
		}),
		"SortFunc": &runtime.BuiltinFunc{Name: "slices.SortFunc", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			s, ok := args[0].(*runtime.Slice)
			if !ok {
				return nil, fmt.Errorf("slices.SortFunc: first arg must be a slice")
			}
			cmp := args[1]
			var cerr error
			sort.SliceStable(s.Elems, func(i, j int) bool {
				if cerr != nil {
					return false
				}
				r, err := h.v.Call(cmp, []runtime.Value{s.Elems[i], s.Elems[j]})
				if err != nil {
					cerr = err
					return false
				}
				n, _ := r.(int64)
				return n < 0
			})
			if cerr != nil {
				return nil, cerr
			}
			return runtime.NIL, nil
		}},
		"EqualFunc": &runtime.BuiltinFunc{Name: "slices.EqualFunc", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			a, _ := args[0].(*runtime.Slice)
			b, _ := args[1].(*runtime.Slice)
			eq := args[2]
			if a == nil || b == nil {
				return nil, fmt.Errorf("slices.EqualFunc: first two args must be slices")
			}
			if len(a.Elems) != len(b.Elems) {
				return false, nil
			}
			for i := range a.Elems {
				r, err := h.v.Call(eq, []runtime.Value{a.Elems[i], b.Elems[i]})
				if err != nil {
					return nil, err
				}
				if ok, _ := r.(bool); !ok {
					return false, nil
				}
			}
			return true, nil
		}},
		"IndexFunc": &runtime.BuiltinFunc{Name: "slices.IndexFunc", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			s, ok := args[0].(*runtime.Slice)
			if !ok {
				return nil, fmt.Errorf("slices.IndexFunc: first arg must be a slice")
			}
			for i, el := range s.Elems {
				r, err := h.v.Call(args[1], []runtime.Value{el})
				if err != nil {
					return nil, err
				}
				if ok, _ := r.(bool); ok {
					return int64(i), nil
				}
			}
			return int64(-1), nil
		}},
		"Max": h.fn("slices.Max", func(a []any) (any, error) {
			el := scriptElems(a[0])
			if len(el) == 0 {
				return nil, fmt.Errorf("slices.Max: empty slice")
			}
			best := el[0]
			for _, x := range el[1:] {
				if lessScript(best, x) {
					best = x
				}
			}
			return best, nil
		}),
		"Min": h.fn("slices.Min", func(a []any) (any, error) {
			el := scriptElems(a[0])
			if len(el) == 0 {
				return nil, fmt.Errorf("slices.Min: empty slice")
			}
			best := el[0]
			for _, x := range el[1:] {
				if lessScript(x, best) {
					best = x
				}
			}
			return best, nil
		}),
		"Reverse": &runtime.BuiltinFunc{Name: "slices.Reverse", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			s, ok := args[0].(*runtime.Slice)
			if !ok {
				return nil, fmt.Errorf("slices.Reverse: arg must be a slice")
			}
			slices.Reverse(s.Elems)
			return runtime.NIL, nil
		}},
		"Insert": &runtime.BuiltinFunc{Name: "slices.Insert", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			s, ok := args[0].(*runtime.Slice)
			if !ok || len(args) < 2 {
				return nil, fmt.Errorf("slices.Insert(slice, i, elems...)")
			}
			i, _ := args[1].(int64)
			el := slices.Insert(s.Elems, int(i), args[2:]...)
			return &runtime.Slice{Elems: el}, nil
		}},
		"Delete": &runtime.BuiltinFunc{Name: "slices.Delete", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			s, ok := args[0].(*runtime.Slice)
			if !ok || len(args) != 3 {
				return nil, fmt.Errorf("slices.Delete(slice, i, j)")
			}
			i, _ := args[1].(int64)
			j, _ := args[2].(int64)
			el := slices.Delete(s.Elems, int(i), int(j))
			return &runtime.Slice{Elems: el}, nil
		}},
	})
	e.Bind("maps", map[string]runtime.Value{
		"Keys":   h.fn("maps.Keys", func(a []any) (any, error) { return mapKeys(a[0]), nil }),
		"Values": h.fn("maps.Values", func(a []any) (any, error) { return mapValues(a[0]), nil }),
		"Clone": h.fn("maps.Clone", func(a []any) (any, error) {
			if m, ok := a[0].(map[any]any); ok {
				return maps.Clone(m), nil
			}
			return nil, fmt.Errorf("maps.Clone: arg must be a map")
		}),
		"Copy": &runtime.BuiltinFunc{Name: "maps.Copy", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			dst, _ := args[0].(*runtime.Map)
			src, _ := args[1].(*runtime.Map)
			if dst == nil || src == nil {
				return nil, fmt.Errorf("maps.Copy: args must be maps")
			}
			for _, k := range src.Order {
				v := src.Pairs[k]
				if _, ok := dst.Pairs[k]; !ok {
					dst.Order = append(dst.Order, k)
				}
				dst.Pairs[k] = v
			}
			return runtime.NIL, nil
		}},
		"Equal": &runtime.BuiltinFunc{Name: "maps.Equal", Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
			a, _ := args[0].(*runtime.Map)
			b, _ := args[1].(*runtime.Map)
			if a == nil || b == nil {
				return args[0] == args[1], nil // nil == nil
			}
			if len(a.Pairs) != len(b.Pairs) {
				return false, nil
			}
			for k, av := range a.Pairs {
				bv, ok := b.Pairs[k]
				if !ok || !equalScript(av, bv) {
					return false, nil
				}
			}
			return true, nil
		}},
	})
	// os: an interpreted program must never observe or terminate the host
	// process — Exit is always a trap; the environment/argv surface is only
	// bound when the engine is unrestricted (no AllowedRoots).
	ospkg := map[string]runtime.Value{
		"Exit": h.fn("os.Exit", func(a []any) (any, error) {
			return nil, errors.New("os.Exit is not supported: an interpreted program cannot terminate the host process")
		}),
	}
	if len(e.cfg.AllowedRoots) == 0 {
		ospkg["Getenv"] = h.fn("os.Getenv", func(a []any) (any, error) { return os.Getenv(str(a[0])), nil })
		ospkg["Args"] = h.fn("os.Args", func(a []any) (any, error) { return strsSlice(os.Args), nil })
	}
	e.Bind("os", ospkg)
	e.Bind("time", map[string]runtime.Value{
		"Sleep": h.fn("time.Sleep", func(a []any) (any, error) { time.Sleep(durOf(a[0])); return nil, nil }),
		"Now":   h.fn("time.Now", func(a []any) (any, error) { return time.Now(), nil }),
		"Since": h.fn("time.Since", func(a []any) (any, error) {
			if t, ok := a[0].(time.Time); ok {
				return time.Since(t), nil
			}
			return nil, fmt.Errorf("time.Since: not a Time")
		}),
		"Parse": h.fn2("time.Parse", func(a []any) (any, error) {
			t, err := time.Parse(str(a[0]), str(a[1]))
			return &runtime.Tuple{Elems: []runtime.Value{scriptVal(t), errVal(err)}}, nil
		}),
		"Unix": h.fn2("time.Unix", func(a []any) (any, error) {
			return &runtime.GoValue{V: time.Unix(int64Of(a[0]), int64Of(a[1]))}, nil
		}),
		"Second":      time.Second,
		"Millisecond": time.Millisecond,
	})
}

// ---- value marshalling ----

// hostHelpers builds BuiltinFuncs whose Fn marshals arguments to Go natives
// and results back to runtime values.
type hostHelpers struct {
	v runtime.VMCaller
	e *Engine // for the configured output writer
}

// out returns the engine's output writer (io.Discard when unset).
func (h *hostHelpers) out() io.Writer {
	if h.e == nil || h.e.out == nil {
		return io.Discard
	}
	return h.e.out
}

func (h *hostHelpers) fn(name string, f func([]any) (any, error)) *runtime.BuiltinFunc {
	return &runtime.BuiltinFunc{Name: name, Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		a := make([]any, len(args))
		for i, v := range args {
			a[i] = goNative(v)
		}
		r, err := f(a)
		if err != nil {
			return nil, err
		}
		return scriptVal(r), nil
	}}
}

// fn2/fn3 are arity-checked variants.
func (h *hostHelpers) fn2(name string, f func([]any) (any, error)) *runtime.BuiltinFunc {
	return h.arity(name, 2, f)
}

func (h *hostHelpers) fn3(name string, f func([]any) (any, error)) *runtime.BuiltinFunc {
	return h.arity(name, 3, f)
}

func (h *hostHelpers) arity(name string, n int, f func([]any) (any, error)) *runtime.BuiltinFunc {
	return &runtime.BuiltinFunc{Name: name, Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		if len(args) < n {
			return nil, fmt.Errorf("%s needs %d args, got %d", name, n, len(args))
		}
		a := make([]any, len(args))
		for i, v := range args {
			a[i] = goNative(v)
		}
		r, err := f(a)
		if err != nil {
			return nil, err
		}
		return scriptVal(r), nil
	}}
}

// sortInPlace sorts a *runtime.Slice's elements directly — going through
// goNative would sort a fresh copy and leave the script's slice untouched.
func (h *hostHelpers) sortInPlace(name string) *runtime.BuiltinFunc {
	return &runtime.BuiltinFunc{Name: name, Fn: func(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("%s needs 1 arg, got %d", name, len(args))
		}
		s, ok := args[0].(*runtime.Slice)
		if !ok {
			return nil, fmt.Errorf("%s: arg must be a slice, got %T", name, args[0])
		}
		sortScript(s.Elems)
		return runtime.NIL, nil
	}}
}

// sortScript orders int64/float64/string elements ascending — the element
// sets sort.Ints, sort.Strings, and slices.Sort support.
func sortScript(el []runtime.Value) {
	sort.Slice(el, func(i, j int) bool {
		bv := runtime.Unwrap(el[j])
		switch a := runtime.Unwrap(el[i]).(type) {
		case int64:
			if b, ok := bv.(int64); ok {
				return a < b
			}
		case float64:
			if b, ok := bv.(float64); ok {
				return a < b
			}
		case string:
			if b, ok := bv.(string); ok {
				return a < b
			}
		}
		return false
	})
}

// sortSlice implements sort.Slice: the less function is a script callable.
// scriptElems exposes a slice argument's raw elements to Go-side helpers
// (the argument may arrive as a []any via goNative).
func scriptElems(v any) []runtime.Value {
	switch s := v.(type) {
	case *runtime.Slice:
		return s.Elems
	case []any:
		el := make([]runtime.Value, len(s))
		for i, x := range s {
			el[i] = scriptVal(x)
		}
		return el
	}
	return nil
}

// lessScript orders int64/float64/string (heterogeneous pairs rank by kind:
// numbers < strings < others, comparing numerically across int64/float64).
func lessScript(a, b runtime.Value) bool {
	an, aok := numOf(a)
	bn, bok := numOf(b)
	if aok && bok {
		return an < bn
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			return as < bs
		}
		return false // strings rank above numbers
	}
	if aok {
		return true
	}
	return false
}

func numOf(v runtime.Value) (float64, bool) {
	switch n := v.(type) {
	case *runtime.Named:
		return numOf(n.V)
	case int64:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// equalScript compares two script values by shape (identity-ish): used by
// maps.Equal where a deep compare is the closest available semantics.
func equalScript(a, b runtime.Value) bool {
	a = runtime.Unwrap(a)
	b = runtime.Unwrap(b)
	if an, ok := numOf(a); ok {
		bn, ok := numOf(b)
		return ok && an == bn
	}
	switch x := a.(type) {
	case string, bool:
		return x == b
	case *runtime.Slice:
		y, ok := b.(*runtime.Slice)
		if !ok || len(x.Elems) != len(y.Elems) {
			return false
		}
		for i := range x.Elems {
			if !equalScript(x.Elems[i], y.Elems[i]) {
				return false
			}
		}
		return true
	case *runtime.Map:
		y, ok := b.(*runtime.Map)
		if !ok || len(x.Pairs) != len(y.Pairs) {
			return false
		}
		for k, xv := range x.Pairs {
			yv, ok := y.Pairs[k]
			if !ok || !equalScript(xv, yv) {
				return false
			}
		}
		return true
	}
	return a == b
}

func (h *hostHelpers) sortSlice(_ runtime.VMCaller, args []runtime.Value) (runtime.Value, error) {
	s, ok := args[0].(*runtime.Slice)
	if !ok {
		return nil, fmt.Errorf("sort.Slice: first arg must be a slice")
	}
	less := args[1]
	sort.Slice(s.Elems, func(i, j int) bool {
		r, err := h.v.Call(less, []runtime.Value{int64(i), int64(j)})
		if err != nil {
			return false
		}
		b, _ := r.(bool)
		return b
	})
	return runtime.NIL, nil
}

// retErr wraps a (n int, err error) or single-value+error result into the
// script-visible tuple (value, err-or-nil).
func retErr(n int, err error) (any, error) {
	return &runtime.Tuple{Elems: []runtime.Value{int64(n), errVal(err)}}, nil
}

func retErr2[T any](v T, err error) (any, error) {
	return &runtime.Tuple{Elems: []runtime.Value{scriptVal(v), errVal(err)}}, nil
}

func errVal(err error) runtime.Value {
	if err == nil {
		return runtime.NIL
	}
	return &runtime.GoValue{V: err} // boxed: method calls (Error(), Unwrap()) dispatch via reflection
}

// scriptVal converts a Go-native result back to a runtime value. Concrete
// runtime types pass through; anything else (errors, host structs) is boxed
// as a GoValue — Value is `any`, so it cannot be a type-switch case itself.
func scriptVal(v any) runtime.Value {
	switch x := v.(type) {
	case nil:
		return runtime.NIL
	case bool, string, int64, float64:
		return x
	case int:
		return int64(x)
	case []string:
		return strsSlice(x)
	case time.Duration:
		return int64(x)
	case []any:
		el := make([]runtime.Value, len(x))
		for i, e := range x {
			el[i] = scriptVal(e)
		}
		return &runtime.Slice{Elems: el}
	case map[any]any:
		m := &runtime.Map{Pairs: map[runtime.Value]runtime.Value{}}
		for k, vv := range x {
			kk := scriptVal(k)
			if _, ok := m.Pairs[kk]; !ok {
				m.Order = append(m.Order, kk)
			}
			m.Pairs[kk] = scriptVal(vv)
		}
		return m
	case runtime.Nil, *runtime.Tuple, *runtime.Cell, *runtime.Slice,
		*runtime.Map, *runtime.Struct, *runtime.Function, *runtime.Closure,
		*runtime.BoundMethod, *runtime.BuiltinFunc, *runtime.GoValue,
		*runtime.Chan, *runtime.TypeDef, *runtime.Iterator, *runtime.Package,
		*runtime.ImportRef, *runtime.Named:
		return x
	default:
		return &runtime.GoValue{V: x}
	}
}

// goNative converts a runtime value to its Go-native counterpart for host
// calls: cells unwrap, slices/maps become []any / map[any]any, structs get a
// Stringer view so fmt prints them sensibly.
func goNative(v runtime.Value) any {
	switch x := v.(type) {
	case runtime.Nil:
		return nil
	case *runtime.Named:
		return goNative(x.V)
	case *runtime.Cell:
		return goNative(x.Elem)
	case *runtime.Slice:
		out := make([]any, len(x.Elems))
		for i, e := range x.Elems {
			out[i] = goNative(e)
		}
		return out
	case *runtime.Map:
		out := make(map[any]any, len(x.Pairs))
		for k, val := range x.Pairs {
			out[goNative(k)] = goNative(val)
		}
		return out
	case *runtime.Struct:
		return fmt.Sprintf("%s%+v", x.Def.Name, x.Fields)
	case *runtime.GoValue:
		return x.V
	default:
		return v
	}
}

// asErr coerces a marshalled script value to error for errors.* calls:
// GoValue-boxed errors unwrap to natives via goNative already, so v is
// either an error, nil, or a value that formats to one.
func asErr(v any) error {
	switch e := v.(type) {
	case nil:
		return nil
	case error:
		return e
	default:
		return fmt.Errorf("%v", v)
	}
}

func str(v any) string {
	if n, ok := v.(*runtime.Named); ok {
		return str(n.V)
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func intOf(v any) int {
	switch x := v.(type) {
	case *runtime.Named:
		return intOf(x.V)
	case int64:
		return int(x)
	case int:
		return x
	case float64:
		return int(x)
	}
	return 0
}

func int64Of(v any) int64 { return int64(intOf(v)) }

func durOf(v any) time.Duration { return time.Duration(int64Of(v)) }

func strSlice(v any) []string {
	if n, ok := v.(*runtime.Named); ok {
		return strSlice(n.V)
	}
	if s, ok := v.(*runtime.Slice); ok {
		out := make([]string, len(s.Elems))
		for i, e := range s.Elems {
			out[i] = str(goNative(e))
		}
		return out
	}
	if a, ok := v.([]any); ok {
		out := make([]string, len(a))
		for i, e := range a {
			out[i] = str(e)
		}
		return out
	}
	return nil
}

func anySlice(v any) []any {
	if n, ok := v.(*runtime.Named); ok {
		return anySlice(n.V)
	}
	if s, ok := v.(*runtime.Slice); ok {
		out := make([]any, len(s.Elems))
		for i, e := range s.Elems {
			out[i] = goNative(e)
		}
		return out
	}
	if a, ok := v.([]any); ok {
		return a
	}
	return nil
}

func strsSlice(ss []string) *runtime.Slice {
	el := make([]runtime.Value, len(ss))
	for i, s := range ss {
		el[i] = s
	}
	return &runtime.Slice{Elems: el}
}

func mapKeys(v any) *runtime.Slice {
	if n, ok := v.(*runtime.Named); ok {
		return mapKeys(n.V)
	}
	if m, ok := v.(*runtime.Map); ok {
		return &runtime.Slice{Elems: append([]runtime.Value{}, m.Order...)}
	}
	if m, ok := v.(map[any]any); ok {
		return &runtime.Slice{Elems: slices.Collect(maps.Keys(m))}
	}
	return &runtime.Slice{}
}

func mapValues(v any) *runtime.Slice {
	if n, ok := v.(*runtime.Named); ok {
		return mapValues(n.V)
	}
	if m, ok := v.(*runtime.Map); ok {
		el := make([]runtime.Value, len(m.Order))
		for i, k := range m.Order {
			el[i] = m.Pairs[k]
		}
		return &runtime.Slice{Elems: el}
	}
	if m, ok := v.(map[any]any); ok {
		return &runtime.Slice{Elems: slices.Collect(maps.Values(m))}
	}
	return &runtime.Slice{}
}
