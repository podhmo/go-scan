package main

// Declared types are carried, not erased: `var x T` yields a typed zero,
// typed nils keep their declared type through ==, methods and asserts,
// and an interface slot holding a typed nil is non-nil (Go semantics).

type Sq struct{ Side int }

func (s Sq) Area() int { return s.Side * s.Side }

type PtrM struct{ N int }

// Safe is a pointer-receiver method that tolerates a nil receiver.
func (p *PtrM) Safe() int {
	if p == nil {
		return -1
	}
	return p.N
}

// ZeroStruct: var s Sq materializes a zero struct, not nil.
func ZeroStruct() int {
	var s Sq
	return s.Side + s.Area() // 0
}

// PkgLevelTypedZero: package-level `var x T` gets the same typed zero.
var pkgSq Sq

func PkgLevelTypedZero() int { return pkgSq.Side + 1 } // 1

// TypedNils: var xs []T / var m map[..] / var c chan are typed nils.
func TypedNils() int {
	var xs []int
	var m map[string]int
	var c chan int
	n := 0
	if xs == nil {
		n++
	}
	if m == nil {
		n++
	}
	if c == nil {
		n++
	}
	if len(xs)+len(m) != 0 {
		return -1
	}
	xs = append(xs, 3)
	return n*100 + len(xs) + xs[0] // 303
}

// NilOps: reads and writes on typed nils behave like Go.
func NilOps() int {
	var m map[string]int
	if v, ok := m["k"]; ok || v != 0 {
		return -1 // read on nil map: zero,false
	}
	delete(m, "k") // delete on nil map: no-op
	var xs []int
	n := 0
	for range xs {
		n++ // range over nil slice: zero iterations
	}
	return n + 1 // 1
}

// PtrNil: var p *T is a typed nil; p == nil and p.M() with a
// nil-tolerant pointer receiver both work.
func PtrNil() int {
	var p *PtrM
	if p != nil {
		return -1
	}
	if p.Safe() != -1 {
		return -2 // nil receiver dispatch
	}
	var q *Sq
	if q != nil {
		return -3
	}
	return 1
}

// IfaceNilBoxes: a typed nil crossing into an interface slot stays
// non-nil — `var x any = (*int)(nil)` behaves like Go.
func IfaceNilBoxes() int {
	var x any = (*int)(nil)
	if x == nil {
		return -1 // typed nil in an interface is NOT nil
	}
	if v, ok := x.(*int); !ok || v != nil {
		return -2 // assert unboxes to the typed nil: v == nil
	}
	return 1
}

// FuncNil: var f func() int is a typed nil; comparing and reading it is
// fine (calling it would panic like Go).
func FuncNil() int {
	var f func() int
	if f != nil {
		return -1
	}
	return 1
}

// LocalType: `type` declarations inside function bodies bind typedefs.
func LocalType() int {
	type P struct{ X int }
	p := P{X: 5}
	var q P
	return p.X + q.X // 5
}

// LocalTypeAssert: a local typedef also serves as an assert target.
func LocalTypeAssert() int {
	type P struct{ X int }
	var x any = P{X: 3}
	v, ok := x.(P)
	if !ok {
		return -1
	}
	return v.X // 3
}

// PtrElems: []T* literal elements auto-take their address like Go.
func PtrElems() int {
	ps := []*Sq{{Side: 3}, {Side: 4}}
	return ps[0].Side + ps[1].Side + ps[0].Area() // 3+4+9=16
}

// NamedPtrLit: named pointer types build the same way.
type NamedPtr *Sq

func NamedPtrVar() int {
	var p NamedPtr
	if p != nil {
		return -1
	}
	return 1
}

// MapKeyIdent: identifier keys in map literals are real expressions.
func MapKeyIdent() int {
	K := 2
	m := map[int]int{K: 10}
	return m[2] // 10
}

// ArrKeyIdent: identifier keys in array literals index, not field names.
func ArrKeyIdent() int {
	I := 1
	xs := []int{I: 7, 3: 9}
	return xs[1] + xs[3] // 16
}

// NamedReturnNil: `func f() (r *T)` bare return yields a typed nil.
func namedNil() (r *Sq) { return }

func NamedReturnNil() int {
	r := namedNil()
	if r != nil {
		return -1
	}
	return 1
}

// ReturnNilTyped: `return nil` under a *T result yields a typed nil.
func explicitNil() *Sq { return nil }

func ReturnNilTyped() int {
	if explicitNil() != nil {
		return -1
	}
	return 1
}

// ParamIfaceBox: passing a typed nil into an interface-typed param
// boxes it — the param is non-nil inside.
func holdsNil(x any) int {
	if x == nil {
		return -1
	}
	return 1
}

func ParamIfaceBox() int {
	var p *int
	return holdsNil(p) // 1: (*int)(nil) in any is non-nil
}

// InferCalls: call-site inference binds T/U from argument types.
func Id2[T any](v T) T          { return v }
func Fst2[T, U any](a T, b U) T { return a }

func InferCalls() int {
	return Id2(40) + Fst2(2, "x") // 42
}

// GenericZero: `var z T` inside a generic body yields T's zero value,
// and a typed-nil T returned as any keeps its type (non-nil interface).
func ZeroT[T any]() any {
	var z T
	return z
}

func GenericZero() int {
	z := ZeroT[int]()
	if z != 0 {
		return -1
	}
	zs := ZeroT[[]int]()
	if zs == nil {
		return -2 // any holding []int(nil) is a non-nil interface
	}
	return 1
}

// ConstraintOK: a satisfied constraint instantiates normally.
func TakesNum[T ~int | ~string](v T) T { return v }

func ConstraintOK() int {
	return TakesNum[int](40) + 2 // 42
}

// ConstraintBad: bool does not satisfy ~int|~string — the instantiation
// itself traps, like the Go type checker rejecting the program.
func ConstraintBad() int {
	_ = TakesNum[bool](true)
	return 0
}

func main() {}
