package main

// ---- spread calls ----

func sum(nums ...int) int {
	t := 0
	for _, n := range nums {
		t += n
	}
	return t
}

func Spread() int {
	xs := []int{1, 2, 3}
	return sum(xs...) + sum(10, 20) // 6 + 30
}

func SpreadTail() int {
	return sum(5, []int{1, 2}...) // 8
}

// ---- compound assign & ++/-- on non-identifiers ----

type C struct{ N int }

func CompoundOps() int {
	s := &C{N: 1}
	s.N += 10 // 11
	s.N++     // 12
	xs := []int{1, 2}
	xs[0] += 5 // 6
	xs[1]++    // 3
	m := map[string]int{"k": 1}
	m["k"] *= 4                         // 4
	p := &s.N                           // FieldRef
	*p += 100                           // s.N = 112
	q := &xs[1]                         // IndexRef
	*q = 50                             // xs = [6,50]
	return s.N + xs[0] + xs[1] + m["k"] // 112+6+50+4
}

func PtrOps() int {
	v := 1
	p := &v
	*p += 10 // 11
	(*p)++   // 12
	return *p
}

// ---- labels, break/continue L, goto ----

func LabeledBreak() int {
	n := 0
Outer:
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			if j == 3 {
				break Outer
			}
			n++
		}
		n += 100
	}
	return n // 3
}

func LabeledContinue() int {
	n := 0
Outer:
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if j > i {
				continue Outer
			}
			n++
		}
		n += 10 // reached only when the inner loop completes (i == 2)
	}
	return n // (1+2+3) + 10
}

func GotoSkip() int {
	n := 0
	goto skip
	n = 100
skip:
	n += 5
	goto end
	n += 1000
end:
	return n // 5
}

func LabeledSwitch() int {
	n := 0
Loop:
	for i := 0; i < 5; i++ {
		switch i {
		case 3:
			break Loop // exits the for, not the switch
		default:
			n++
		}
	}
	return n // 3
}

// ---- interfaces ----

type Shape interface {
	Area() int
}

type Sq struct{ Side int }

func (s Sq) Area() int { return s.Side * s.Side }

type Named interface{ Name() string }
type Tag struct{ Label string }

func (t Tag) Name() string { return t.Label }

type Wrapped struct{ Sq } // embedded: promoted Area()

func InterfaceDispatch() int {
	var sh Shape = Sq{Side: 4}
	return sh.Area() // 16
}

func EmbeddedMethod() int {
	w := Wrapped{Sq: Sq{Side: 5}}
	return w.Area() // 25
}

func IfaceHolds() int {
	var x any = "hello"
	if _, ok := x.(Shape); ok {
		return -1
	}
	var s Shape = Sq{Side: 6}
	if _, ok := s.(Shape); !ok { // satisfies itself
		return -2
	}
	if _, ok := s.(Named); ok { // Sq lacks Name()
		return -3
	}
	return 1
}

// ---- type assertions + switches ----

func AssertOK() (int, bool) {
	var x any = "hi"
	if v, ok := x.(string); ok {
		return len(v), ok // 2,true
	}
	return 0, false
}

func AssertFail() bool {
	var x any = 7
	_, ok := x.(string)
	return ok // false
}

func TypeSwitch() int {
	classify := func(v any) int {
		switch v.(type) {
		case int:
			return 1
		case string:
			return 2
		case bool:
			return 3
		default:
			return 0
		}
	}
	return classify(1)*100 + classify("x")*10 + classify(nil) // 120
}

func TypeSwitchBind() string {
	var x any = "hey"
	switch v := x.(type) {
	case int:
		return "int"
	case string:
		return v + "!" // narrowed binding
	default:
		return "?"
	}
}

func AssertPanic() int {
	n := 0
	func() {
		defer func() {
			if r := recover(); r != nil {
				n = 99
			}
		}()
		var x any = 1
		_ = x.(string) // panics: 1 is not string
	}()
	return n // 99
}

// ---- generics ----

func Id[T any](v T) T { return v }

func Fst[T, U any](a T, b U) T { return a }

func Convert[T any](x int) T { return T(x) } // binds T -> typedef in conversion

func GenericFns() int {
	return Id[int](40) + Fst[int, string](2, "x") // 42
}

func GenericConvert() int {
	return Convert[int](40) + 2 // 42
}

type Pair[T any] struct{ A, B T }

func (p Pair[T]) Sum() T { return p.A + p.B }

func GenericType() int {
	p := Pair[int]{A: 20, B: 22}
	return p.Sum() // 42
}

// ---- select with labels (regression: selects keep breaking right) ----

func SelectLabel() int {
	c := make(chan int)
	n := 0
Loop:
	for i := 0; i < 3; i++ {
		select {
		case v := <-c:
			n += v
		default:
			if i == 2 {
				break Loop
			}
			n++
		}
	}
	return n // i=0: n=1; i=1: n=2; i=2: break
}

func main() {}
