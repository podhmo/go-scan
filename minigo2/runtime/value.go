// Package runtime holds the minigo2 value model, environments, and the lazy
// package machinery. Values are boxed `any` for now; a tagged
// representation is a later optimization.
package runtime

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"sync"

	"github.com/podhmo/go-scan/minigo2/bytecode"
	"github.com/podhmo/go-scan/minigo2/syntax"
)

// Value is a boxed interpreter value. Concrete types:
//
//	int64, float64, string, bool, Nil,
//	*Cell, *Slice, *Map, *Struct, *TypeDef,
//	*Function, *Closure, *BoundMethod, *BuiltinFunc, *Tuple,
//	*Package, *Iterator, *GoValue
type Value = any

// Nil is the nil value.
type Nil struct{}

// NIL is the singleton nil.
var NIL Value = Nil{}

// Cell is a mutable slot. Every declared variable is a cell, which makes
// closures, pointers and addressable receivers uniform: a pointer IS a cell.
type Cell struct{ Elem Value }

// Tuple packs multiple values (multi return / multi assign).
type Tuple struct{ Elems []Value }

// Slice is a Go slice value.
type Slice struct{ Elems []Value }

// Map is a Go map value (keys must be comparable basics for now).
type Map struct {
	Pairs map[Value]Value
	Order []Value // insertion order for stable-ish range
}

// Chan is a channel value. minigo2 approximates goroutines by running `go`
// calls synchronously, so channels are unbounded queues: sends never block
// (buffer capacity is not modeled) and a receive on an empty open channel —
// which could never be satisfied in a single-threaded world — traps instead
// of deadlocking the interpreter.
type Chan struct {
	Elems  []Value
	Closed bool
}

// TypeDef is a runtime type descriptor for a named type.
type TypeDef struct {
	Pkg     *Package
	Name    string
	File    *syntax.File
	Spec    *ast.TypeSpec
	Kind    TypeKind
	Fields  []string             // struct field order
	FTags   map[string]string    // struct tags
	Anon    ast.Expr             // underlying type AST (non-struct named types)
	Methods map[string]*Function // lazily built method set
	once    sync.Once
}

// TypeKind classifies a named type's underlying shape.
type TypeKind uint8

const (
	KindStruct     TypeKind = iota
	KindNamedBasic          // type MyInt int etc.
	KindSlice
	KindMap
	KindFunc
	KindInterface
	KindAlias
	KindChan
)

// Struct is an instance of a KindStruct TypeDef.
type Struct struct {
	Def    *TypeDef
	Fields []Value
}

// BoundMethod binds a receiver to a function.
type BoundMethod struct {
	Recv Value // *Cell for pointer receivers, plain Value otherwise
	Fn   *Function
}

// BuiltinFunc is a host-native function (len, println, host intrinsics).
type BuiltinFunc struct {
	Name string
	Fn   func(vm VMCaller, args []Value) (Value, error)
}

// VMCaller is the piece of the VM builtins need (kept narrow to avoid a
// runtime->vm dependency).
type VMCaller interface {
	Call(fn Value, args []Value) (Value, error)
	// Recover implements the recover() builtin: it returns the in-flight
	// panic value when called directly by a deferred function, else nil.
	Recover() Value
}

// Function is a compiled-or-compilable function. Chunk is produced lazily
// on first call via Compile.
type Function struct {
	Pkg     *Package
	File    *syntax.File
	Decl    *ast.FuncDecl // nil for the synthetic package __init__
	Name    string
	Recv    string // receiver type name, "" for plain funcs
	PtrRecv bool

	Compile func(*Function) error // injected by the engine
	once    sync.Once
	cerr    error
	Chunk   *bytecode.Chunk
}

// EnsureCompiled compiles the function on first use.
func (f *Function) EnsureCompiled() error {
	f.once.Do(func() {
		if f.Compile != nil {
			f.cerr = f.Compile(f)
		}
	})
	return f.cerr
}

// Closure is a function value with captured upvalue cells.
type Closure struct {
	Fn     *Function
	Upvals []*Cell
}

// Iterator is the state of an in-progress range loop.
type Iterator struct {
	Kind   byte // 's' slice, 'm' map, 'i' int, 'x' string, 'c' chan
	Elems  []Value
	Keys   []Value // map keys
	Idx    int
	Limit  int // for integer ranges
	String string
	Chan   *Chan // for channel ranges
}

// GoValue wraps a host reflect value at the FFI boundary (implemented in
// ffi.go; declared here as the box used by Globals/Register).
type GoValue struct{ V any }

// Panic is a script-level panic value; catchable by recover().
type Panic struct {
	Value  Value
	Frames []string // "func at file:line" entries collected while unwinding
}

func (p *Panic) Error() string {
	if len(p.Frames) == 0 {
		return fmt.Sprintf("panic: %v", p.Value)
	}
	return fmt.Sprintf("panic: %v\n%s", p.Value, strings.Join(p.Frames, "\n"))
}

// Trap is a VM-level failure (unsupported construct, invalid operation).
// It bypasses script recover() and unwinds to the engine boundary.
type Trap struct {
	Pos    token.Pos
	Reason string
	Frames []string // "func at file:line" entries collected while unwinding
}

func (t *Trap) Error() string {
	if len(t.Frames) == 0 {
		return fmt.Sprintf("runtime trap: %s", t.Reason)
	}
	return fmt.Sprintf("runtime trap: %s\n%s", t.Reason, strings.Join(t.Frames, "\n"))
}
