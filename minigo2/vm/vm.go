// Package vm is the minigo2 stack-machine interpreter. One VM call executes
// one frame; nested calls recurse through VM.Call. Script panics and traps
// both unwind via Go panic and are converted to errors at the call boundary.
package vm

import (
	"fmt"
	"go/token"
	"reflect"
	"unicode/utf8"

	"github.com/podhmo/go-scan/minigo2/bytecode"
	"github.com/podhmo/go-scan/minigo2/index"
	"github.com/podhmo/go-scan/minigo2/runtime"
	"github.com/podhmo/go-scan/minigo2/syntax"
)

// Hooks are the engine-provided services the VM needs.
type Hooks struct {
	// Builtin resolves a predeclared name (len, append, int, ...).
	Builtin func(name string) (runtime.Value, bool)
	// Materialize builds the runtime value for a package-level decl
	// (function, type, var, const) on first access.
	Materialize func(pkg *runtime.Package, d *index.Decl) (runtime.Value, error)
}

// VM is a stack machine. It is safe for sequential use from one goroutine.
type VM struct {
	H Hooks
}

type frame struct {
	fn     *runtime.Function
	ch     *bytecode.Chunk
	locals []*runtime.Cell
	upvals []*runtime.Cell
	stack  []runtime.Value
	ip     int
}

func (f *frame) push(v runtime.Value) { f.stack = append(f.stack, v) }

func (f *frame) pop() runtime.Value {
	v := f.stack[len(f.stack)-1]
	f.stack = f.stack[:len(f.stack)-1]
	return v
}

func (f *frame) pos() token.Pos {
	if f.ip > 0 {
		return f.ch.Code[f.ip-1].Pos
	}
	return 0
}

func (f *frame) trap(format string, args ...any) {
	panic(&runtime.Trap{Pos: f.pos(), Reason: fmt.Sprintf(format, args...)})
}

// Call invokes a function-like value: Function, Closure, BoundMethod,
// BuiltinFunc, TypeDef (conversion), or Cell wrapping any of those.
func (v *VM) Call(callee runtime.Value, args []runtime.Value) (result runtime.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, asError(r)
		}
	}()
	for {
		switch c := callee.(type) {
		case *runtime.BuiltinFunc:
			return c.Fn(v, args)
		case *runtime.TypeDef:
			if len(args) != 1 {
				return nil, fmt.Errorf("conversion to %s needs exactly one argument", c.Name)
			}
			return convert(c, args[0])
		case *runtime.Cell:
			callee = c.Elem
			continue
		}
		break
	}
	fr, err := v.prepFrame(callee, args)
	if err != nil {
		return nil, err
	}
	v.exec(fr)
	if len(fr.stack) == 0 {
		return runtime.NIL, nil
	}
	return fr.stack[len(fr.stack)-1], nil
}

func asError(r any) error {
	switch e := r.(type) {
	case *runtime.Trap:
		return e
	case *runtime.Panic:
		return e
	case error:
		return e
	default:
		return fmt.Errorf("panic: %v", r)
	}
}

// prepFrame builds the frame for callee.
func (v *VM) prepFrame(callee runtime.Value, args []runtime.Value) (*frame, error) {
	var fn *runtime.Function
	var upvals []*runtime.Cell
	switch c := callee.(type) {
	case *runtime.Function:
		fn = c
	case *runtime.Closure:
		fn = c.Fn
		upvals = c.Upvals
	case *runtime.BoundMethod:
		fn = c.Fn
		args = append([]runtime.Value{c.Recv}, args...)
	default:
		return nil, fmt.Errorf("value of type %T is not callable", callee)
	}
	if err := fn.EnsureCompiled(); err != nil {
		return nil, err
	}
	ch := fn.Chunk
	fr := &frame{fn: fn, ch: ch, upvals: upvals}
	fr.locals = make([]*runtime.Cell, ch.NLocals)
	// bind params
	n := ch.NParams
	for i := 0; i < n; i++ {
		var a runtime.Value = runtime.NIL
		if i < len(args) {
			a = args[i]
		}
		if ch.IsVararg && i == n-1 {
			rest := runtime.NIL
			if i < len(args) {
				rest = &runtime.Slice{Elems: append([]runtime.Value{}, args[i:]...)}
			}
			fr.locals[i] = &runtime.Cell{Elem: rest}
			break
		}
		fr.locals[i] = &runtime.Cell{Elem: valueCopy(a)}
	}
	for i := n; i < ch.NLocals; i++ {
		fr.locals[i] = &runtime.Cell{Elem: runtime.NIL}
	}
	return fr, nil
}

func (v *VM) exec(f *frame) {
	code := f.ch.Code
	consts := f.ch.Consts
	for f.ip < len(code) {
		ins := code[f.ip]
		f.ip++
		switch ins.Op {
		case bytecode.OpNop:
		case bytecode.OpConst:
			f.push(consts[ins.A])
		case bytecode.OpNil:
			f.push(runtime.NIL)
		case bytecode.OpDup:
			f.push(f.stack[len(f.stack)-1])
		case bytecode.OpSwap:
			n := len(f.stack)
			f.stack[n-1], f.stack[n-2] = f.stack[n-2], f.stack[n-1]
		case bytecode.OpRot3:
			n := len(f.stack)
			f.stack[n-3], f.stack[n-2], f.stack[n-1] = f.stack[n-2], f.stack[n-1], f.stack[n-3]
		case bytecode.OpPop:
			f.pop()
		case bytecode.OpNewLocal:
			f.locals[ins.A] = &runtime.Cell{Elem: valueCopy(f.pop())}
		case bytecode.OpRenewVar:
			f.locals[ins.A] = &runtime.Cell{Elem: f.locals[ins.A].Elem}
		case bytecode.OpLocal:
			f.push(f.locals[ins.A].Elem)
		case bytecode.OpSetLocal:
			f.locals[ins.A].Elem = valueCopy(f.pop())
		case bytecode.OpLocalRef:
			f.push(f.locals[ins.A])
		case bytecode.OpUpval:
			f.push(f.upvals[ins.A].Elem)
		case bytecode.OpSetUpval:
			f.upvals[ins.A].Elem = f.pop()
		case bytecode.OpGlobal:
			f.push(v.resolveGlobal(f, consts[ins.A].(string)))
		case bytecode.OpNewGlobal:
			f.fn.Pkg.Globals.Set(consts[ins.A].(string), &runtime.Cell{Elem: valueCopy(f.pop())})
		case bytecode.OpSetGlobal:
			name := consts[ins.A].(string)
			val := valueCopy(f.pop())
			if old, ok := f.fn.Pkg.Globals.Get(name); ok {
				if c, isCell := old.(*runtime.Cell); isCell {
					c.Elem = val
					break
				}
			}
			f.fn.Pkg.Globals.Set(name, val)
		case bytecode.OpGlobalRef:
			name := consts[ins.A].(string)
			if old, ok := f.fn.Pkg.Globals.Get(name); ok {
				if c, isCell := old.(*runtime.Cell); isCell {
					f.push(c)
					break
				}
			}
			f.trap("cannot take address of %s", name)
		case bytecode.OpSelect:
			base := f.pop()
			f.push(v.selectMember(f, base, consts[ins.A].(string)))
		case bytecode.OpSetField:
			val := f.pop()
			base := f.pop()
			v.setField(f, base, consts[ins.A].(string), val)
		case bytecode.OpIndex:
			idx := f.pop()
			base := f.pop()
			f.push(v.index(f, base, idx))
		case bytecode.OpSetIndex:
			val := f.pop()
			idx := f.pop()
			base := f.pop()
			v.setIndex(f, base, idx, val)
		case bytecode.OpSlice:
			hi := f.pop()
			lo := f.pop()
			base := f.pop()
			f.push(v.slice(f, base, lo, hi))
		case bytecode.OpDeref:
			v := f.pop()
			if cell, ok := v.(*runtime.Cell); ok {
				f.push(cell.Elem)
			} else {
				f.trap("deref of non-pointer %T", v)
			}
		case bytecode.OpSetInd:
			val := f.pop()
			cell := f.pop()
			if c, ok := cell.(*runtime.Cell); ok {
				c.Elem = val
			} else {
				f.trap("indirect store to non-pointer %T", cell)
			}
		case bytecode.OpBox:
			f.push(&runtime.Cell{Elem: f.pop()})
		case bytecode.OpCall:
			argc := int(ins.A)
			args := make([]runtime.Value, argc)
			for i := argc - 1; i >= 0; i-- {
				args[i] = f.pop()
			}
			fn := f.pop()
			r, err := v.Call(fn, args)
			if err != nil {
				panic(&runtime.Trap{Pos: ins.Pos, Reason: err.Error()})
			}
			f.push(r)
		case bytecode.OpPack:
			n := int(ins.A)
			el := make([]runtime.Value, n)
			for i := n - 1; i >= 0; i-- {
				el[i] = f.pop()
			}
			f.push(&runtime.Tuple{Elems: el})
		case bytecode.OpUnpack:
			tv := f.pop()
			t, ok := tv.(*runtime.Tuple)
			if !ok {
				// single value to N targets: error unless N==1
				if ins.A == 1 {
					f.push(tv)
					break
				}
				f.trap("multi-assign from non-tuple %T", tv)
			}
			if len(t.Elems) != int(ins.A) {
				f.trap("unpack mismatch: %d values to %d names", len(t.Elems), ins.A)
			}
			for _, e := range t.Elems {
				f.push(e)
			}
		case bytecode.OpMakeComposite:
			f.push(v.makeComposite(f, ins))
		case bytecode.OpMakeClosure:
			proto := consts[ins.A].(*runtime.Function)
			cl := &runtime.Closure{Fn: proto}
			for _, d := range proto.Chunk.Upvals {
				if d.FromParentUpval {
					cl.Upvals = append(cl.Upvals, f.upvals[d.Index])
				} else {
					cl.Upvals = append(cl.Upvals, f.locals[d.Index])
				}
			}
			f.push(cl)
		case bytecode.OpBinary:
			b := f.pop()
			a := f.pop()
			f.push(binaryOp(f, bytecode.BinOp(ins.A), a, b))
		case bytecode.OpUnary:
			a := f.pop()
			f.push(unaryOp(f, bytecode.UnOp(ins.A), a))
		case bytecode.OpJump:
			f.ip = int(ins.A)
		case bytecode.OpJumpFalse:
			if !truthy(f.pop()) {
				f.ip = int(ins.A)
			}
		case bytecode.OpJumpTrue:
			if truthy(f.pop()) {
				f.ip = int(ins.A)
			}
		case bytecode.OpIter:
			f.push(newIterator(f, f.pop()))
		case bytecode.OpRangeNext:
			it := f.locals[ins.B].Elem.(*runtime.Iterator)
			if !iterNext(f, it, int(ins.C)) {
				f.ip = int(ins.A)
			}
		case bytecode.OpPanic:
			panic(&runtime.Panic{Value: f.pop()})
		case bytecode.OpTrap:
			panic(&runtime.Trap{Pos: ins.Pos, Reason: fmt.Sprint(consts[ins.A])})
		case bytecode.OpReturn:
			var results []runtime.Value
			n := int(ins.A)
			if n < 0 {
				// bare return: gather named result cells
				results = make([]runtime.Value, len(f.ch.NamedSlots))
				for i, s := range f.ch.NamedSlots {
					results[i] = f.locals[s].Elem
				}
				n = len(results)
			} else {
				results = make([]runtime.Value, n)
				for i := n - 1; i >= 0; i-- {
					results[i] = f.pop()
				}
			}
			switch len(results) {
			case 0:
				f.push(runtime.NIL)
			case 1:
				f.push(results[0])
			default:
				f.push(&runtime.Tuple{Elems: results})
			}
			return
		default:
			f.trap("unknown opcode %d", ins.Op)
		}
	}
}

// fileOf returns the source file an instruction belongs to, via its
// position (precise for __init__, which mixes decls from several files),
// falling back to the function's own file.
func fileOf(f *frame, pkg *runtime.Package) *syntax.File {
	if pos := f.pos(); pos.IsValid() && pkg.Fset != nil {
		name := pkg.Fset.PositionFor(pos, false).Filename
		if sf, ok := pkg.FileByName[name]; ok {
			return sf
		}
	}
	return f.fn.File
}

// resolveGlobal resolves a name in file scope order: imports, package
// globals (including lazily materialized decls), dot imports, builtins.
func (v *VM) resolveGlobal(f *frame, name string) runtime.Value {
	pkg := f.fn.Pkg
	file := fileOf(f, pkg)
	// 1. file imports
	if file != nil {
		if ref, ok := pkg.Scopes[file][name]; ok {
			return ref
		}
	}
	// 2. package globals / lazy members
	if gv, ok := pkg.Globals.Get(name); ok {
		if c, isCell := gv.(*runtime.Cell); isCell {
			return c.Elem
		}
		return gv
	}
	if pkg.Index != nil {
		if d, ok := lookupDecl(pkg, name); ok {
			mv, err := v.H.Materialize(pkg, d)
			if err != nil {
				f.trap("materialize %s: %s", name, err)
			}
			pkg.Globals.Set(name, mv)
			return mv
		}
	}
	// 3. dot imports (indexed, not initialized; exported names only)
	if file != nil && token.IsExported(name) {
		for _, ref := range pkg.Scopes[file] {
			if ref.Alias != "." {
				continue
			}
			p, err := ref.Materialize()
			if err != nil {
				f.trap("dot import %s: %s", ref.Path, err)
			}
			if gv, ok := p.Globals.Get(name); ok {
				return gv
			}
		}
	}
	// 4. builtins
	if bv, ok := v.H.Builtin(name); ok {
		return bv
	}
	f.trap("undefined: %s", name)
	return nil
}

func lookupDecl(pkg *runtime.Package, name string) (*index.Decl, bool) {
	if d, ok := pkg.Index.Funcs[name]; ok {
		return d, true
	}
	if td, ok := pkg.Index.Types[name]; ok && td.Decl != nil {
		return td.Decl, true
	}
	if d, ok := pkg.Index.Consts[name]; ok {
		return d, true
	}
	if d, ok := pkg.Index.Vars[name]; ok {
		return d, true
	}
	return nil, false
}

// selectMember implements base.name for import refs, packages, structs,
// typedefs, and cells.
func (v *VM) selectMember(f *frame, base runtime.Value, name string) runtime.Value {
	switch b := base.(type) {
	case *runtime.ImportRef:
		if !token.IsExported(name) {
			f.trap("cannot refer to unexported name %s.%s", b.Path, name)
		}
		p, err := b.Materialize()
		if err != nil {
			f.trap("import %s: %s", b.Path, err)
		}
		mv, err := p.Member(name, v.H.Materialize)
		if err != nil {
			f.trap("%s", err)
		}
		if c, isCell := mv.(*runtime.Cell); isCell {
			mv = c.Elem
		} else {
			p.Globals.Set(name, mv)
		}
		return mv
	case *runtime.Package:
		if !token.IsExported(name) {
			f.trap("cannot refer to unexported name %s.%s", b.Path, name)
		}
		mv, err := b.Member(name, v.H.Materialize)
		if err != nil {
			f.trap("%s", err)
		}
		if c, isCell := mv.(*runtime.Cell); isCell {
			mv = c.Elem
		}
		return mv
	case *runtime.Struct:
		return v.structMember(f, b, name, b)
	case *runtime.Cell:
		switch e := b.Elem.(type) {
		case *runtime.Struct:
			return v.structMember(f, e, name, b)
		default:
			f.trap("select %s on cell of %T", name, b.Elem)
		}
	case *runtime.TypeDef:
		if m, ok := b.Methods[name]; ok {
			return m // method expression: T.M(recv, ...)
		}
		f.trap("type %s has no method %s", b.Name, name)
	case *runtime.Slice:
		f.trap("select %s on slice", name)
	case *runtime.Map:
		f.trap("select %s on map", name)
	default:
		f.trap("select %s on %T", name, base)
	}
	return nil
}

func (v *VM) structMember(f *frame, s *runtime.Struct, name string, recv runtime.Value) runtime.Value {
	def := s.Def
	for i, fn := range def.Fields {
		if fn == name {
			return s.Fields[i]
		}
	}
	if m, ok := def.Methods[name]; ok {
		if err := m.EnsureCompiled(); err != nil {
			f.trap("%s", err)
		}
		r := recv
		if m.PtrRecv {
			// pointer receiver needs an addressable cell
			if _, isCell := r.(*runtime.Cell); !isCell {
				r = &runtime.Cell{Elem: r}
			}
		} else {
			// value receiver operates on a copy
			if c, isCell := r.(*runtime.Cell); isCell {
				r = c.Elem
			}
			r = valueCopy(r)
		}
		return &runtime.BoundMethod{Recv: r, Fn: m}
	}
	f.trap("%s has no field or method %s", def.Name, name)
	return nil
}

func (v *VM) setField(f *frame, base runtime.Value, name string, val runtime.Value) {
	switch b := base.(type) {
	case *runtime.Struct:
		for i, fn := range b.Def.Fields {
			if fn == name {
				b.Fields[i] = val
				return
			}
		}
		f.trap("%s has no field %s", b.Def.Name, name)
	case *runtime.Cell:
		if s, ok := b.Elem.(*runtime.Struct); ok {
			v.setField(f, s, name, val)
			return
		}
		f.trap("set field %s on cell of %T", name, b.Elem)
	default:
		f.trap("set field %s on %T", name, base)
	}
}

func (v *VM) index(f *frame, base, idx runtime.Value) runtime.Value {
	switch b := base.(type) {
	case *runtime.Cell:
		return v.index(f, b.Elem, idx)
	case *runtime.Slice:
		i, ok := idx.(int64)
		if !ok {
			f.trap("slice index is %T", idx)
		}
		return b.Elems[i]
	case *runtime.Map:
		return b.Pairs[idx]
	case string:
		i, ok := idx.(int64)
		if !ok {
			f.trap("string index is %T", idx)
		}
		return int64(b[i])
	default:
		f.trap("index on %T", base)
		return nil
	}
}

func (v *VM) setIndex(f *frame, base, idx, val runtime.Value) {
	switch b := base.(type) {
	case *runtime.Cell:
		v.setIndex(f, b.Elem, idx, val)
	case *runtime.Slice:
		i, ok := idx.(int64)
		if !ok {
			f.trap("slice index is %T", idx)
		}
		b.Elems[i] = val
	case *runtime.Map:
		if idx != nil && !reflect.TypeOf(idx).Comparable() {
			f.trap("map key %T is not comparable", idx)
		}
		if _, exists := b.Pairs[idx]; !exists {
			b.Order = append(b.Order, idx)
		}
		b.Pairs[idx] = val
	default:
		f.trap("index assign on %T", base)
	}
}

func (v *VM) slice(f *frame, base, lo, hi runtime.Value) runtime.Value {
	switch b := base.(type) {
	case *runtime.Cell:
		return v.slice(f, b.Elem, lo, hi)
	case *runtime.Slice:
		l, h := bounds(f, lo, hi, int64(len(b.Elems)))
		return &runtime.Slice{Elems: b.Elems[l:h]}
	case string:
		l, h := bounds(f, lo, hi, int64(len(b)))
		return b[l:h]
	default:
		f.trap("slice on %T", base)
		return nil
	}
}

func bounds(f *frame, lo, hi runtime.Value, n int64) (int64, int64) {
	l := int64(0)
	h := n
	if _, isNil := lo.(runtime.Nil); !isNil {
		l = lo.(int64)
	}
	if _, isNil := hi.(runtime.Nil); !isNil {
		h = hi.(int64)
	}
	return l, h
}

func (v *VM) makeComposite(f *frame, ins bytecode.Instruction) runtime.Value {
	n := int(ins.A)
	kv := ins.B == 1
	// pop typedef then elems? No: compiler emitted typedef first, then elems.
	// stack: [typedef, e1, e2, ...] — typedef is BELOW elems.
	elems := make([]runtime.Value, 0, n*2)
	total := n
	if kv {
		total = n * 2
	}
	raw := make([]runtime.Value, total)
	for i := total - 1; i >= 0; i-- {
		raw[i] = f.pop()
	}
	tdv := f.pop()
	td, ok := tdv.(*runtime.TypeDef)
	if !ok {
		f.trap("composite literal on non-type %T", tdv)
	}
	switch td.Kind {
	case runtime.KindSlice:
		s := &runtime.Slice{}
		if kv {
			s.Elems = make([]runtime.Value, n)
			for i := 0; i < n; i++ {
				idx := raw[i*2]
				ival, ok := idx.(int64)
				if !ok {
					f.trap("slice literal index %T", idx)
				}
				s.Elems[ival] = raw[i*2+1]
			}
		} else {
			s.Elems = raw
		}
		return s
	case runtime.KindMap:
		m := &runtime.Map{Pairs: map[runtime.Value]runtime.Value{}}
		for i := 0; i < n; i++ {
			k := raw[i*2]
			m.Pairs[k] = raw[i*2+1]
			m.Order = append(m.Order, k)
		}
		return m
	case runtime.KindStruct, runtime.KindNamedBasic, runtime.KindAlias:
		s := &runtime.Struct{Def: td, Fields: make([]runtime.Value, len(td.Fields))}
		for i := range s.Fields {
			s.Fields[i] = runtime.NIL
		}
		if kv {
			for i := 0; i < n; i++ {
				name, ok := raw[i*2].(string)
				if !ok {
					f.trap("struct literal key %T", raw[i*2])
				}
				found := false
				for fi, fn := range td.Fields {
					if fn == name {
						s.Fields[fi] = raw[i*2+1]
						found = true
						break
					}
				}
				if !found {
					f.trap("%s has no field %s", td.Name, name)
				}
			}
		} else {
			copy(s.Fields, raw)
		}
		return s
	default:
		f.trap("composite literal for kind %d", td.Kind)
	}
	_ = elems
	return nil
}

// valueCopy implements Go assignment semantics: structs copy by value;
// slices, maps and pointers share.
func valueCopy(v runtime.Value) runtime.Value {
	if s, ok := v.(*runtime.Struct); ok {
		cp := &runtime.Struct{Def: s.Def, Fields: make([]runtime.Value, len(s.Fields))}
		copy(cp.Fields, s.Fields)
		return cp
	}
	return v
}

// iterators

func newIterator(f *frame, coll runtime.Value) *runtime.Iterator {
	switch c := coll.(type) {
	case *runtime.Cell:
		return newIterator(f, c.Elem)
	case *runtime.Slice:
		return &runtime.Iterator{Kind: 's', Elems: c.Elems}
	case *runtime.Map:
		it := &runtime.Iterator{Kind: 'm', Keys: c.Order}
		for _, k := range c.Order {
			it.Elems = append(it.Elems, c.Pairs[k])
		}
		return it
	case int64:
		return &runtime.Iterator{Kind: 'i', Limit: int(c)}
	case string:
		return &runtime.Iterator{Kind: 'x', String: c}
	default:
		f.trap("range over %T", coll)
		return nil
	}
}

// iterNext pushes nvars values (key/index, elem) and returns false when done.
func iterNext(f *frame, it *runtime.Iterator, nvars int) bool {
	push := func(key, val runtime.Value) {
		if nvars == 2 {
			f.push(key)
			f.push(val)
			return
		}
		f.push(key) // single-var range yields index/key
	}
	switch it.Kind {
	case 's':
		if it.Idx >= len(it.Elems) {
			return false
		}
		push(int64(it.Idx), it.Elems[it.Idx])
		it.Idx++
		return true
	case 'm':
		if it.Idx >= len(it.Keys) {
			return false
		}
		push(it.Keys[it.Idx], it.Elems[it.Idx])
		it.Idx++
		return true
	case 'i':
		if it.Idx >= it.Limit {
			return false
		}
		push(int64(it.Idx), int64(it.Idx))
		it.Idx++
		return true
	case 'x':
		if it.Idx >= len(it.String) {
			return false
		}
		r, size := utf8.DecodeRuneInString(it.String[it.Idx:])
		push(int64(it.Idx), int64(r))
		it.Idx += size
		return true
	}
	return false
}

// arithmetic

func truthy(v runtime.Value) bool {
	switch x := v.(type) {
	case bool:
		return x
	case runtime.Nil:
		return false
	case int64:
		return x != 0
	case float64:
		return x != 0
	case string:
		return x != ""
	default:
		return true
	}
}

func binaryOp(f *frame, op bytecode.BinOp, a, b runtime.Value) runtime.Value {
	// equality works on any comparable pair
	switch op {
	case bytecode.BinEql:
		return eqlValue(a, b)
	case bytecode.BinNeq:
		return !eqlValue(a, b)
	}
	if s, ok := a.(string); ok {
		return stringBinOp(f, op, s, b)
	}
	if isFloat(a) || isFloat(b) {
		return floatBinOp(f, op, toFloat(a), toFloat(b))
	}
	if ai, ok := a.(int64); ok {
		bi, ok := b.(int64)
		if !ok {
			f.trap("binary %d on %T and %T", op, a, b)
		}
		return intBinOp(f, op, ai, bi)
	}
	if ab, ok := a.(bool); ok {
		bb, ok := b.(bool)
		if !ok {
			f.trap("binary %d on %T and %T", op, a, b)
		}
		switch op {
		case bytecode.BinLAnd:
			return ab && bb
		case bytecode.BinLOr:
			return ab || bb
		}
	}
	f.trap("binary %d on %T and %T", op, a, b)
	return nil
}

func intBinOp(f *frame, op bytecode.BinOp, a, b int64) runtime.Value {
	switch op {
	case bytecode.BinAdd:
		return a + b
	case bytecode.BinSub:
		return a - b
	case bytecode.BinMul:
		return a * b
	case bytecode.BinQuo:
		return a / b
	case bytecode.BinRem:
		return a % b
	case bytecode.BinAnd:
		return a & b
	case bytecode.BinOr:
		return a | b
	case bytecode.BinXor:
		return a ^ b
	case bytecode.BinAndNot:
		return a &^ b
	case bytecode.BinShl:
		return a << b
	case bytecode.BinShr:
		return a >> b
	case bytecode.BinLss:
		return a < b
	case bytecode.BinLeq:
		return a <= b
	case bytecode.BinGtr:
		return a > b
	case bytecode.BinGeq:
		return a >= b
	}
	f.trap("int binary %d", op)
	return nil
}

func floatBinOp(f *frame, op bytecode.BinOp, a, b float64) runtime.Value {
	switch op {
	case bytecode.BinAdd:
		return a + b
	case bytecode.BinSub:
		return a - b
	case bytecode.BinMul:
		return a * b
	case bytecode.BinQuo:
		return a / b
	case bytecode.BinLss:
		return a < b
	case bytecode.BinLeq:
		return a <= b
	case bytecode.BinGtr:
		return a > b
	case bytecode.BinGeq:
		return a >= b
	}
	f.trap("float binary %d", op)
	return nil
}

func stringBinOp(f *frame, op bytecode.BinOp, a string, b runtime.Value) runtime.Value {
	s, ok := b.(string)
	if !ok {
		f.trap("string binary on %T", b)
	}
	switch op {
	case bytecode.BinAdd:
		return a + s
	case bytecode.BinLss:
		return a < s
	case bytecode.BinLeq:
		return a <= s
	case bytecode.BinGtr:
		return a > s
	case bytecode.BinGeq:
		return a >= s
	}
	f.trap("string binary %d", op)
	return nil
}

func unaryOp(f *frame, op bytecode.UnOp, a runtime.Value) runtime.Value {
	switch op {
	case bytecode.UnNot:
		return !truthy(a)
	case bytecode.UnPos:
		return a
	case bytecode.UnNeg:
		switch x := a.(type) {
		case int64:
			return -x
		case float64:
			return -x
		}
		f.trap("unary - on %T", a)
	case bytecode.UnXor:
		if x, ok := a.(int64); ok {
			return ^x
		}
		f.trap("unary ^ on %T", a)
	}
	f.trap("unary %d on %T", op, a)
	return nil
}

func eqlValue(a, b runtime.Value) bool {
	if ai, ok := a.(int64); ok {
		switch bv := b.(type) {
		case int64:
			return ai == bv
		case float64:
			return float64(ai) == bv
		}
		return false
	}
	if af, ok := a.(float64); ok {
		switch bv := b.(type) {
		case int64:
			return af == float64(bv)
		case float64:
			return af == bv
		}
		return false
	}
	if _, ok := a.(runtime.Nil); ok {
		_, isNil := b.(runtime.Nil)
		return isNil
	}
	return a == b // pointers, strings, bools
}

func isFloat(v runtime.Value) bool {
	_, ok := v.(float64)
	return ok
}

func toFloat(v runtime.Value) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	}
	return 0
}

// convert implements T(x) — a call on a *TypeDef.
func convert(td *runtime.TypeDef, v runtime.Value) (runtime.Value, error) {
	switch td.Name {
	case "int", "int64", "int32", "byte", "rune":
		switch x := v.(type) {
		case int64:
			return x, nil
		case float64:
			return int64(x), nil
		case string:
			return int64([]rune(x)[0]), nil
		}
	case "float64", "float32":
		return toFloat(v), nil
	case "string":
		switch x := v.(type) {
		case string:
			return x, nil
		case int64:
			return string(rune(x)), nil
		case *runtime.Slice:
			// []byte or []rune -> string
			var bs []byte
			for _, e := range x.Elems {
				if i, ok := e.(int64); ok {
					bs = append(bs, byte(i))
				}
			}
			return string(bs), nil
		}
	case "bool":
		return truthy(v), nil
	}
	// named types: pass through
	if td.Kind == runtime.KindNamedBasic || td.Name != "" {
		return v, nil
	}
	return nil, fmt.Errorf("cannot convert %T to %s", v, td.Name)
}
