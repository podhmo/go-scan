package conversions

// ---- []byte / []rune basics ----

func ByteFromString() int {
	b := []byte("hi")
	return len(b)*100 + int(b[0]) // 2*100+104 = 304
}

func RuneFromString() int {
	r := []rune("héllo")          // 5 runes
	return len(r)*100 + int(r[1]) // 5*100+233 = 733
}

func StringFromBytes() string {
	b := []byte("yo")
	return string(b) + "!"
}

func StringFromRunes() string {
	r := []rune{'a', 'b'}
	return string(r) + "z"
}

func ByteFromInts() int {
	b := []byte([]int{104, 105}) // illegal: Go rejects slice->[]byte (only string)
	return len(b)*100 + int(b[0])
}

func ByteFromByteSlice() int {
	b := []byte([]byte("zz"))
	return len(b) + int(b[0])
}

func ByteSliceFromUint8() int {
	var u []uint8 = []byte("q")
	b := []byte(u)
	return len(b)*100 + int(b[0]) // 1*100+113=213
}

func Uint8SliceFromByte() int {
	b := []byte("q")
	u := []uint8(b)
	return len(u)*100 + int(u[0])
}

// ---- named slice types ----

type B []byte

func NamedByteCast() int {
	b := B("hi")
	return len(b)*100 + int(b[1])
}

func NamedByteCastBack() string {
	b := B("hi")
	return string(b) // named []byte -> string
}

func NamedByteNil() int {
	b := B(nil)
	if b == nil {
		return 1
	}
	return -1
}

type C B

func ChainByteCast() int {
	c := C("hi") // type C B chain: underlying []byte
	return len(c)*100 + int(c[0])
}

type BAlias = []byte

func AliasByteCast() int {
	b := BAlias("hi")
	return len(b)*100 + int(b[0])
}

type SL []string

func NamedSliceCast() int {
	s := SL([]string{"a", "b"})
	return len(s)*10 + len(s[0])
}

type MyInt int
type Ints []MyInt
type Ints2 []MyInt

func NamedSliceOfNamed() int {
	s := Ints([]MyInt{3, 4})
	return int(s[0]) + int(s[1])
}

func NamedSliceRoundTrip() int {
	s := Ints([]int{5, 6}) // illegal: element MyInt != int
	return int(s[0]) + int(s[1])
}

func SliceCastToInts() int {
	s := []int(Ints{7, 8}) // illegal: []Ints(MyInt) -> []int
	return s[0] + s[1]
}

func NamedToNamedSlice() int {
	s := Ints{7, 8}
	t := Ints2(s) // Ints -> Ints2: identical underlying
	return int(t[0]) + int(t[1])
}

type MyByte byte
type MyByteSlice []MyByte

func NamedElemCast() int {
	s := MyByteSlice("hi") // string -> []MyByte: elem's underlying is byte
	return len(s)*100 + int(s[0])
}

func NamedElemCastBack() string {
	s := MyByteSlice{'h', 'i'}
	return string(s)
}

// ---- generics ----

func CastT[T any](v any) T {
	return T(v)
}

func GenericCast() int {
	return CastT[int64](int64(9))
}

func ToSlice[T any](v []any) []T {
	return []T{v[0].(T)}
}

func GenericSliceCast() int {
	s := ToSlice[int]([]any{41})
	return s[0] + 1
}

type Wrap[T any] []T

func GenericNamedSlice() int {
	w := Wrap[int]([]int{1, 2, 3})
	return w[0] + w[1] + w[2]
}

func GenericNamedSliceString() int {
	w := Wrap[string]([]string{"a", "b"})
	return len(w)*10 + len(w[0])
}

// StrToSliceT: []T(s) inside a generic body — T's instantiation decides
// whether the special string conversion is legal.
func StrToSliceT[T any](s string) []T {
	return []T(s)
}

func GenericTByteCast() int {
	s := StrToSliceT[byte]("hi")
	return len(s)*100 + int(s[0])
}

func GenericTByteCastBad() int {
	s := StrToSliceT[int]("hi") // T=int: not a byte/rune family element
	return len(s)
}

// GenericWrapElem: a Wrap[int]-typed slice's element coerces like int.
func GenericWrapElem() int {
	w := Wrap[int]{1, 2}
	w[1] = 40
	return w[0] + w[1]
}

// ---- maps / chans / pointers / structs ----

type M1 map[string]int
type M2 map[string]int

func NamedMapCast() int {
	m := M2(M1{"a": 5})
	return m["a"]
}

func NamedMapCastBad() int {
	type MInt map[int]int
	m := MInt(M1{"a": 5}) // map[string]int -> map[int]int: key differs
	return len(m)
}

type C1 chan int
type C2 chan int

func NamedChanCast() int {
	c := C2(make(C1))
	go func() { c <- 7 }()
	return <-c
}

type PSq *Sq
type Sq struct{ X int }

func NamedPtrCast() int {
	p := PSq(&Sq{X: 9})
	return p.X
}

type Sq2 struct{ X int }
type Sq3 struct{ Y int }

func StructCast() int {
	s := Sq2(Sq{X: 8}) // identical underlying structs
	return s.X
}

func StructCastBad() int {
	s := Sq2(Sq3{Y: 8}) // field names differ
	return s.X
}

// ---- error cases (all illegal in Go too) ----

func SliceToStringBad() string {
	return string([]int{104}) // []int is not byte/rune family
}

func SliceToSliceBad() int {
	r := []byte([]rune("hi")) // []int32 -> []uint8
	return len(r)
}

func StringToSliceBad() int {
	i := []int("hi") // string -> []int
	return len(i)
}

func PtrToSliceBad() int {
	var p *byte
	b := []byte(p)
	return len(b)
}

func NilToSliceBad() int {
	var r []rune
	b := []byte(r) // typed nil []rune -> []byte: shapes differ
	return len(b)
}

func NilToSliceOK() int {
	var r []rune
	b := []rune(r)
	if b == nil {
		return 1
	}
	return -1
}

// ---- misc ----

func SlicePtrCast() int {
	p := &[]byte{"a"[0]}
	_ = p
	return 1
}

func ByteOfStringIdx() int {
	s := "abc"
	b := []byte(s)
	b[0] = 'x'
	return int(s[0]) - int(b[0]) // 97-120 = -23 (copy semantics)
}

// ---- operations that must preserve the slice's declared type ----

func SlicedRuneString() string {
	r := []rune("héllo")
	return string(r[1:3]) // "él" — slicing keeps the []rune tag
}

func SlicedNamedSlice() int {
	t := Ints2(Ints{7, 8}[0:2]) // sub-slice keeps Ints: []MyInt -> Ints2
	return int(t[0]) + int(t[1])
}

func AppendKeepsByteTag() string {
	b := []byte("h")
	return string(append(b, 'i')) // append keeps []byte: "hi"
}

func AppendKeepsRuneTag() string {
	r := []rune("h")
	return string(append(r, 'é'))
}

func AppendOnTypedNil() string {
	var r []rune
	return string(append(r, 'ü')) // nil's declared type survives append
}

// ---- nested generics ----

type Wrap2[T any] [][]T

func NestedGenericCast() int {
	w := Wrap2[int]([][]int{{1, 2}, {3}})
	return w[0][0] + w[0][1] + w[1][0]
}

func NestedGenericLit() int {
	w := Wrap2[int]{{1, 2}, {3}} // element coerce resolves []T under binds
	return w[0][0] + w[1][0]
}

func NestedGenericElemAssign() int {
	w := Wrap2[int]{{1}, {2}}
	w[0] = []int{9} // element type is []T with T=int
	return w[0][0] + w[1][0]
}

// ---- anonymous-struct sources ----

func AnonStructCast() int {
	s := Sq2(struct{ X int }{X: 5})
	return s.X
}

func AnonStructCastBad() int {
	s := Sq2(struct{ Y int }{Y: 5}) // different fields: Go rejects
	return s.X
}

// ---- typed nil through a named chain ----

func ChainNilRetag() int {
	c := C(B(nil)) // same canonical zero as C(nil)
	if c == nil {
		return 1
	}
	return -1
}
