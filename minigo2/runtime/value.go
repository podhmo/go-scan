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

// FieldRef is the address-of a struct field (`&s.f`) — a cell-view over
// base.name. The base resolves at access time (struct value or pointer).
type FieldRef struct {
	Base Value
	Name string
}

// structOf resolves the base to the struct being referenced.
func (r *FieldRef) structOf() *Struct {
	v := r.Base
	for {
		if s, ok := v.(*Struct); ok {
			return s
		}
		dv, ok := Deref(v)
		if !ok {
			return nil
		}
		v = dv
	}
}

// Get reads the field value.
func (r *FieldRef) Get() (Value, bool) {
	s := r.structOf()
	if s == nil {
		return nil, false
	}
	for i, n := range s.Def.Fields {
		if n == r.Name {
			return s.Fields[i], true
		}
	}
	return nil, false
}

// Set writes the field value.
func (r *FieldRef) Set(v Value) bool {
	s := r.structOf()
	if s == nil {
		return false
	}
	for i, n := range s.Def.Fields {
		if n == r.Name {
			s.Fields[i] = v
			return true
		}
	}
	return false
}

// IndexRef is the address-of a slice element (`&s[i]`) — a cell-view over
// base[key]. (Map values are unaddressable in Go, so only slices qualify.)
type IndexRef struct {
	Base Value
	Key  Value
}

// sliceOf resolves the base to the slice being referenced.
func (r *IndexRef) sliceOf() *Slice {
	v := r.Base
	for {
		if s, ok := v.(*Slice); ok {
			return s
		}
		dv, ok := Deref(v)
		if !ok {
			return nil
		}
		v = dv
	}
}

// Get reads the element value.
func (r *IndexRef) Get() (Value, bool) {
	s := r.sliceOf()
	i, ok := r.Key.(int64)
	if s == nil || !ok || i < 0 || i >= int64(len(s.Elems)) {
		return nil, false
	}
	return s.Elems[i], true
}

// Set writes the element value.
func (r *IndexRef) Set(v Value) bool {
	s := r.sliceOf()
	i, ok := r.Key.(int64)
	if s == nil || !ok || i < 0 || i >= int64(len(s.Elems)) {
		return false
	}
	s.Elems[i] = v
	return true
}

// Deref unwraps any pointer-like value one level: Cell, FieldRef or
// IndexRef. It reports false for non-references.
func Deref(v Value) (Value, bool) {
	switch r := v.(type) {
	case *Cell:
		return r.Elem, true
	case *FieldRef:
		return r.Get()
	case *IndexRef:
		return r.Get()
	}
	return nil, false
}

// SetRef stores through any pointer-like value: Cell, FieldRef or IndexRef.
func SetRef(v, val Value) bool {
	switch r := v.(type) {
	case *Cell:
		r.Elem = val
		return true
	case *FieldRef:
		return r.Set(val)
	case *IndexRef:
		return r.Set(val)
	}
	return false
}

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

	TParams []string // generic type parameter names (type Foo[T any] ...)

	// Interfaces: MReqs are the directly declared method names; IEmbeds are
	// the embedded element expressions (io.Reader, ~int unions, ...). The
	// full required set is computed by the engine on demand.
	MReqs   []string
	IEmbeds []ast.Expr

	// Embedding: for each embedded struct field (no declared name),
	// EmbedSpecs[i] is its type AST and EmbedIdx[i] its index in Fields.
	// Resolved lazily into Embeds by the engine's FindMethod/MethodsOf.
	EmbedSpecs []ast.Expr
	EmbedIdx   []int
	Embeds     []*TypeDef
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

	TParams []string         // generic type parameter names
	Binds   map[string]Value // compile-time bindings: type params -> TypeDef args

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

// SymbolID is a canonical symbol address used for special-form dispatch:
// the defining package's import path plus the member name.
type SymbolID struct {
	PackagePath string
	Name        string
}

// QuotedCall is a special-form call site: the call and its arguments are
// kept as AST together with the caller's scope (name -> slot maps), so a
// handler can evaluate or inspect them on demand — partial evaluation.
type QuotedCall struct {
	Call   *ast.CallExpr
	File   *syntax.File
	Locals map[string]int // visible local name -> caller slot index
	Upvals map[string]int // visible upvalue name -> caller upvalue index
}

// SpecialFunc is a special-form handler. It fires only when the VM reaches
// the SPECIAL_CALL site; arguments arrive quoted, not evaluated.
type SpecialFunc func(ctx SpecialContext, call *QuotedCall) (Value, error)

// SpecialContext is the narrow interpreter surface handed to special-form
// handlers: position/scope access plus on-demand evaluation of quoted
// expressions inside the caller's frame.
type SpecialContext interface {
	// Position resolves an AST node's source position in the caller's file set.
	Position(n ast.Node) token.Position
	// File is the source file containing the special call.
	File() *syntax.File
	// Package is the package containing the special call.
	Package() *Package
	// Eval evaluates expr in the caller's scope (locals, upvalues,
	// package globals, imports, builtins).
	Eval(expr ast.Expr) (Value, error)
	// Call invokes a callable runtime value.
	Call(fn Value, args []Value) (Value, error)
	// Format renders an AST node back to source text.
	Format(n ast.Node) string
	// Errorf reports a failure attributed to an AST node.
	Errorf(n ast.Node, format string, args ...any) error
}

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
