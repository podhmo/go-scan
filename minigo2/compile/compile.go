// Package compile lowers per-function Go AST to stack-VM bytecode.
//
// The compiler is a total function: every Go program compiles. Any construct
// outside the supported subset emits OpTrap at its position instead of an
// error, so a single unsupported node never blocks the rest of the package.
//
// Scoping: only locals and upvalues are resolved statically. Anything that
// is not a local compiles to OpGlobal/OpSetGlobal and is resolved dynamically
// at run time (package globals, then file imports, then builtins) — this is
// what keeps imports lazy.
package compile

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"strconv"

	"github.com/podhmo/go-scan/minigo2/bytecode"
	"github.com/podhmo/go-scan/minigo2/index"
	"github.com/podhmo/go-scan/minigo2/runtime"
	"github.com/podhmo/go-scan/minigo2/syntax"
)

// fscope is the static scope model of one function while compiling.
type fscope struct {
	parent  *fscope
	blocks  []map[string]int // name -> local slot
	nlocals int
	upvals  []bytecode.UpvalDesc
	upmap   map[string]int
}

func newFScope(parent *fscope) *fscope {
	return &fscope{parent: parent, upmap: map[string]int{}}
}

func (s *fscope) pushBlock() { s.blocks = append(s.blocks, map[string]int{}) }
func (s *fscope) popBlock()  { s.blocks = s.blocks[:len(s.blocks)-1] }

func (s *fscope) declare(name string) int {
	slot := s.nlocals
	s.nlocals++
	if len(s.blocks) == 0 {
		s.pushBlock()
	}
	s.blocks[len(s.blocks)-1][name] = slot
	return slot
}

func (s *fscope) lookupLocal(name string) (int, bool) {
	for i := len(s.blocks) - 1; i >= 0; i-- {
		if slot, ok := s.blocks[i][name]; ok {
			return slot, true
		}
	}
	return 0, false
}

// inCurrentBlock reports whether name is declared in the innermost block —
// `x := 1` on an existing same-block name is a redefinition (assignment),
// while an outer-block name is shadowed by a fresh cell.
func (s *fscope) inCurrentBlock(name string) bool {
	if len(s.blocks) == 0 {
		return false
	}
	_, ok := s.blocks[len(s.blocks)-1][name]
	return ok
}

// find reports how name is reachable inside this function's frame:
// a local slot (isUpval=false) or an upvalue index (isUpval=true).
func (s *fscope) find(name string) (isUpval bool, idx int, ok bool) {
	if slot, found := s.lookupLocal(name); found {
		return false, slot, true
	}
	if i, found := s.upmap[name]; found {
		return true, i, true
	}
	if s.capture(name) {
		return true, s.upmap[name], true
	}
	return false, 0, false
}

// capture pulls name from the enclosing frame into this function's upvalue
// table. Returns false when the name is not bound in any enclosing function
// (a global reference).
func (s *fscope) capture(name string) bool {
	if s.parent == nil {
		return false
	}
	if _, done := s.upmap[name]; done {
		return true
	}
	fromUpval, idx, ok := s.parent.find(name)
	if !ok {
		return false
	}
	s.upvals = append(s.upvals, bytecode.UpvalDesc{FromParentUpval: fromUpval, Index: idx})
	s.upmap[name] = len(s.upvals) - 1
	return true
}

// ctrlCtx is the patching context for breakable/continuable constructs
// (for, range, switch, select).
type ctrlCtx struct {
	isLoop     bool
	continueIP int   // -1 until post position is known
	continues  []int // continue jumps to patch once continueIP is known
	breaks     []int // instruction indices to patch to construct end
}

// compiler holds the state for one chunk under construction.
type compiler struct {
	pkg  *runtime.Package // may be nil
	file *syntax.File
	fs   *fscope
	ch   *bytecode.Chunk
	ctrl []*ctrlCtx
}

func (c *compiler) emit(op bytecode.Op, a, b int, pos token.Pos) int {
	c.ch.Code = append(c.ch.Code, bytecode.Instruction{Op: op, A: int32(a), B: int32(b), C: -1, Pos: pos})
	return len(c.ch.Code) - 1
}

func (c *compiler) emit3(op bytecode.Op, a, b, cc int, pos token.Pos) int {
	i := c.emit(op, a, b, pos)
	c.ch.Code[i].C = int32(cc)
	return i
}

func (c *compiler) patchA(i, target int) { c.ch.Code[i].A = int32(target) }

func (c *compiler) trap(pos token.Pos, format string, args ...any) {
	c.emit(bytecode.OpTrap, c.constIdx(fmt.Sprintf(format, args...)), 0, pos)
}

func (c *compiler) constIdx(v any) int {
	c.ch.Consts = append(c.ch.Consts, v)
	return len(c.ch.Consts) - 1
}

func (c *compiler) nameIdx(s string) int { return c.constIdx(s) }

// ---- name resolution ----

func (c *compiler) getRef(name string, pos token.Pos) {
	isUp, idx, ok := c.fs.find(name)
	switch {
	case !ok:
		c.emit(bytecode.OpGlobal, c.nameIdx(name), 0, pos)
	case !isUp:
		c.emit(bytecode.OpLocal, idx, 0, pos)
	default:
		c.emit(bytecode.OpUpval, idx, 0, pos)
	}
}

func (c *compiler) setRef(name string, pos token.Pos) {
	isUp, idx, ok := c.fs.find(name)
	switch {
	case !ok:
		c.emit(bytecode.OpSetGlobal, c.nameIdx(name), 0, pos)
	case !isUp:
		c.emit(bytecode.OpSetLocal, idx, 0, pos)
	default:
		c.emit(bytecode.OpSetUpval, idx, 0, pos)
	}
}

func (c *compiler) refRef(name string, pos token.Pos) {
	isUp, idx, ok := c.fs.find(name)
	switch {
	case !ok:
		c.emit(bytecode.OpGlobalRef, c.nameIdx(name), 0, pos)
	case !isUp:
		c.emit(bytecode.OpLocalRef, idx, 0, pos)
	default:
		c.trap(pos, "cannot take address of captured variable %s", name)
	}
}

// ---- public entry points ----

// Func compiles fn.Decl into fn.Chunk.
func Func(fn *runtime.Function) error {
	c := &compiler{pkg: fn.Pkg, file: fn.File, fs: newFScope(nil), ch: &bytecode.Chunk{Name: fn.Name}}
	c.fs.pushBlock()

	// Params (and the receiver for methods) are pre-bound by the VM into
	// slots 0..NParams-1; they are only declared here, not re-created.
	nparams := 0
	if fn.Decl.Recv != nil {
		recv := "$recv"
		if len(fn.Decl.Recv.List) > 0 && len(fn.Decl.Recv.List[0].Names) > 0 {
			recv = fn.Decl.Recv.List[0].Names[0].Name
		}
		c.fs.declare(recv)
		nparams++
	}
	if fn.Decl.Type.Params != nil {
		for _, field := range fn.Decl.Type.Params.List {
			names := field.Names
			if len(names) == 0 {
				names = []*ast.Ident{{Name: fmt.Sprintf("$arg%d", nparams)}}
			}
			for _, n := range names {
				c.fs.declare(n.Name)
				nparams++
			}
			if _, ok := field.Type.(*ast.Ellipsis); ok {
				c.ch.IsVararg = true
			}
		}
	}
	c.ch.NParams = nparams

	// Named results are local cells initialized to nil.
	nresults := 0
	if fn.Decl.Type.Results != nil {
		nresults = countResults(fn.Decl.Type.Results)
		for _, field := range fn.Decl.Type.Results.List {
			for _, n := range field.Names {
				slot := c.fs.declare(n.Name)
				c.ch.NamedSlots = append(c.ch.NamedSlots, slot)
				c.emit(bytecode.OpNil, 0, 0, n.Pos())
				c.emit(bytecode.OpNewLocal, slot, 0, n.Pos())
			}
		}
	}
	c.ch.NResults = nresults

	c.stmt(fn.Decl.Body)
	// implicit return
	c.emit(bytecode.OpReturn, nresults, 0, fn.Decl.End())
	c.ch.NLocals = c.fs.nlocals
	c.ch.Upvals = c.fs.upvals
	fn.Chunk = c.ch
	return nil
}

// Expr compiles a bare AST expression into a chunk that pushes the
// expression's value and returns. It backs the OpEvalAST migration bridge:
// interpreter-visible fragments (e.g. future special-form bodies) are kept
// as AST and compiled on first execution. The expression resolves names
// like a function body does — locals don't exist, so free identifiers fall
// through to package globals, imports, and builtins at run time.
func Expr(pkg *runtime.Package, file *syntax.File, e ast.Expr) (*bytecode.Chunk, error) {
	c := &compiler{pkg: pkg, file: file, fs: newFScope(nil), ch: &bytecode.Chunk{Name: "<eval>"}}
	c.fs.pushBlock()
	c.expr(e)
	c.emit(bytecode.OpReturn, 1, 0, e.End())
	c.ch.NLocals = c.fs.nlocals
	return c.ch, nil
}

func countResults(fl *ast.FieldList) int {
	n := 0
	for _, f := range fl.List {
		if len(f.Names) == 0 {
			n++
		} else {
			n += len(f.Names)
		}
	}
	return n
}

// InitFunc compiles the synthetic package initializer: const/var declarations
// in file order, then init() calls.
func InitFunc(pkg *runtime.Package) (*bytecode.Chunk, error) {
	c := &compiler{pkg: pkg, fs: newFScope(nil), ch: &bytecode.Chunk{Name: pkg.Name + ".__init__"}}
	c.fs.pushBlock()

	// iota is a real identifier in const specs; bind it as a hidden local
	// (declared last wins — it shadows nothing here).
	iotaSlot := c.fs.declare("iota")
	c.emit(bytecode.OpConst, c.constIdx(int64(0)), 0, 0)
	c.emit(bytecode.OpNewLocal, iotaSlot, 0, 0)

	// One representative decl per spec, in source order.
	var specReps []*index.Decl
	seen := map[*ast.ValueSpec]bool{}
	for _, d := range pkg.Index.Decls {
		if d.Kind != index.ConstDecl && d.Kind != index.VarDecl {
			continue
		}
		vs := d.Spec.(*ast.ValueSpec)
		if seen[vs] {
			continue
		}
		seen[vs] = true
		specReps = append(specReps, d)
	}
	// Go initializes vars/consts in dependency order, not textual order.
	for _, d := range orderSpecs(pkg.Index, specReps) {
		if d.Kind == index.ConstDecl {
			c.emit(bytecode.OpConst, c.constIdx(int64(d.Idx)), 0, d.Pos)
			c.emit(bytecode.OpSetLocal, iotaSlot, 0, d.Pos)
		}
		c.valueSpec(d.Spec.(*ast.ValueSpec), d)
	}
	for _, d := range pkg.Index.Inits {
		fv := &runtime.Function{Pkg: pkg, File: d.File, Decl: d.Func, Name: "init"}
		c.emit(bytecode.OpConst, c.constIdx(fv), 0, d.Pos)
		c.emit(bytecode.OpCall, 0, 0, d.Pos)
		c.emit(bytecode.OpPop, 0, 0, d.Pos)
	}
	c.emit(bytecode.OpReturn, 0, 0, 0)
	c.ch.NLocals = c.fs.nlocals
	c.ch.Upvals = c.fs.upvals
	return c.ch, nil
}

// valueSpec emits a whole var/const spec: all of its names are bound.
// Vars become package cells (OpNewGlobal); consts plain values (OpSetGlobal).
func (c *compiler) valueSpec(vs *ast.ValueSpec, d *index.Decl) {
	isConst := d.Kind == index.ConstDecl
	bind := func(name *ast.Ident) {
		op := bytecode.OpNewGlobal
		if isConst {
			op = bytecode.OpSetGlobal
		}
		c.emit(op, c.nameIdx(name.Name), 0, name.Pos())
	}
	vals := vs.Values
	if isConst && len(vals) == 0 {
		vals = d.Inherited
	}
	switch {
	case len(vals) == 0:
		for _, name := range vs.Names {
			c.emit(bytecode.OpNil, 0, 0, name.Pos())
			bind(name)
		}
	case len(vals) == 1 && len(vs.Names) > 1:
		c.expr(vals[0])
		c.emit3(bytecode.OpUnpack, len(vs.Names), 0, 0, vs.Pos())
		for i := len(vs.Names) - 1; i >= 0; i-- {
			bind(vs.Names[i])
		}
	default:
		for i, name := range vs.Names {
			c.expr(vals[i])
			bind(name)
		}
	}
}

// ---- statements ----

func (c *compiler) stmt(s ast.Stmt) {
	switch st := s.(type) {
	case *ast.BlockStmt:
		c.fs.pushBlock()
		for _, x := range st.List {
			c.stmt(x)
		}
		c.fs.popBlock()
	case *ast.ExprStmt:
		c.expr(st.X)
		c.emit(bytecode.OpPop, 0, 0, st.Pos())
	case *ast.DeclStmt:
		gd := st.Decl.(*ast.GenDecl)
		for _, spec := range gd.Specs {
			switch gd.Tok {
			case token.VAR, token.CONST:
				vs := spec.(*ast.ValueSpec)
				isConst := gd.Tok == token.CONST
				if len(vs.Values) == 1 && len(vs.Names) > 1 {
					c.expr(vs.Values[0])
					c.emit3(bytecode.OpUnpack, len(vs.Names), 0, 0, vs.Pos())
					for i := len(vs.Names) - 1; i >= 0; i-- {
						c.bindLocal(vs.Names[i].Name, vs.Names[i].Pos(), isConst)
					}
					break
				}
				for i, name := range vs.Names {
					if len(vs.Values) == 0 {
						c.emit(bytecode.OpNil, 0, 0, name.Pos())
					} else {
						c.expr(vs.Values[i])
					}
					c.bindLocal(name.Name, name.Pos(), isConst)
				}
			case token.TYPE:
				// local type declarations are rare; treat as no-op for MVP
			case token.IMPORT:
				c.trap(st.Pos(), "import inside function is not valid Go")
			}
		}
	case *ast.AssignStmt:
		c.assign(st)
	case *ast.IncDecStmt:
		op := bytecode.BinAdd
		if st.Tok == token.DEC {
			op = bytecode.BinSub
		}
		one := func() { c.emit(bytecode.OpConst, c.constIdx(int64(1)), 0, st.Pos()) }
		switch t := st.X.(type) {
		case *ast.Ident:
			c.getRef(t.Name, t.Pos())
			one()
			c.emit(bytecode.OpBinary, int(op), 0, st.Pos())
			c.setRef(t.Name, t.Pos())
		case *ast.SelectorExpr:
			c.expr(t.X)
			c.emit(bytecode.OpDup, 0, 0, t.Pos())
			c.emit(bytecode.OpSelect, c.nameIdx(t.Sel.Name), 0, t.Pos())
			one()
			c.emit(bytecode.OpBinary, int(op), 0, st.Pos())
			c.emit(bytecode.OpSetField, c.nameIdx(t.Sel.Name), 0, t.Pos())
		default:
			c.trap(st.Pos(), "++/-- on %T is not supported", st.X)
		}
	case *ast.IfStmt:
		c.ifStmt(st)
	case *ast.ForStmt:
		c.forStmt(st)
	case *ast.RangeStmt:
		c.rangeStmt(st)
	case *ast.SwitchStmt:
		c.switchStmt(st)
	case *ast.ReturnStmt:
		c.returnStmt(st)
	case *ast.BranchStmt:
		c.branchStmt(st)
	case *ast.EmptyStmt:
	case *ast.DeferStmt:
		c.callStmt(st.Call, bytecode.OpDefer, st.Pos())
	case *ast.GoStmt:
		c.callStmt(st.Call, bytecode.OpGo, st.Pos())
	case *ast.SendStmt:
		c.expr(st.Chan)
		c.expr(st.Value)
		c.emit(bytecode.OpSend, 0, 0, st.Pos())
	case *ast.SelectStmt:
		c.selectStmt(st)
	case *ast.LabeledStmt:
		c.trap(st.Pos(), "labels/goto are not supported")
	case *ast.TypeSwitchStmt:
		c.trap(st.Pos(), "type switch is not supported yet")
	case *ast.CaseClause, *ast.CommClause:
		c.trap(st.Pos(), "case clause outside switch")
	default:
		c.trap(s.Pos(), "unsupported statement %T", s)
	}
}

func (c *compiler) bindLocal(name string, pos token.Pos, _ bool) {
	if name == "_" {
		c.emit(bytecode.OpPop, 0, 0, pos)
		return
	}
	slot := c.fs.declare(name)
	c.emit(bytecode.OpNewLocal, slot, 0, pos)
}

// callStmt compiles `defer f(x)` / `go f(x)`: callee and args are
// evaluated immediately; op (OpDefer/OpGo) decides when the call runs.
func (c *compiler) callStmt(call *ast.CallExpr, op bytecode.Op, pos token.Pos) {
	if call.Ellipsis.IsValid() {
		c.trap(pos, "spread calls are not supported")
		return
	}
	c.calleeExpr(call.Fun)
	for _, a := range call.Args {
		c.expr(a)
	}
	c.emit(op, len(call.Args), 0, pos)
}

// assign handles =, :=, and compound ops.
func (c *compiler) assign(st *ast.AssignStmt) {
	isDefine := st.Tok == token.DEFINE
	simple := st.Tok == token.ASSIGN || isDefine

	if !simple {
		// compound: only identifier LHS for now
		if len(st.Lhs) != 1 || len(st.Rhs) != 1 {
			c.trap(st.Pos(), "compound assignment requires single operands")
			return
		}
		id, ok := st.Lhs[0].(*ast.Ident)
		if !ok {
			c.trap(st.Pos(), "compound assignment on non-identifier")
			return
		}
		c.getRef(id.Name, id.Pos())
		c.expr(st.Rhs[0])
		if op, ok := binOpOf(st.Tok); ok {
			c.emit(bytecode.OpBinary, int(op), 0, st.Pos())
		} else {
			c.trap(st.Pos(), "unsupported assign op %s", st.Tok)
			return
		}
		c.setRef(id.Name, id.Pos())
		return
	}

	n := len(st.Lhs)
	if len(st.Rhs) == 1 && n > 1 {
		if n == 2 {
			switch x := st.Rhs[0].(type) {
			case *ast.UnaryExpr:
				// comma-ok receive: v, ok := <-ch
				if x.Op == token.ARROW {
					c.expr(x.X)
					c.emit(bytecode.OpRecvOK, 0, 0, x.Pos())
					c.emit3(bytecode.OpUnpack, 2, 0, 0, st.Pos())
					for i := n - 1; i >= 0; i-- {
						c.storeTarget(st.Lhs[i], isDefine)
					}
					return
				}
			case *ast.IndexExpr:
				// comma-ok map access: v, ok := m[k]
				c.expr(x.X)
				c.expr(x.Index)
				c.emit(bytecode.OpIndexOK, 0, 0, x.Pos())
				c.emit3(bytecode.OpUnpack, 2, 0, 0, st.Pos())
				for i := n - 1; i >= 0; i-- {
					c.storeTarget(st.Lhs[i], isDefine)
				}
				return
			}
		}
		c.expr(st.Rhs[0])
		c.emit3(bytecode.OpUnpack, n, 0, 0, st.Pos())
	} else {
		for _, r := range st.Rhs {
			c.expr(r)
		}
	}
	// store in reverse order (stack top = last value)
	for i := n - 1; i >= 0; i-- {
		c.storeTarget(st.Lhs[i], isDefine)
	}
}

// storeTarget emits the store for one LHS expression; the value is on stack.
func (c *compiler) storeTarget(lhs ast.Expr, isDefine bool) {
	switch t := lhs.(type) {
	case *ast.Ident:
		if t.Name == "_" {
			c.emit(bytecode.OpPop, 0, 0, t.Pos())
			return
		}
		if isDefine && !c.fs.inCurrentBlock(t.Name) {
			slot := c.fs.declare(t.Name)
			c.emit(bytecode.OpNewLocal, slot, 0, t.Pos())
		} else {
			c.setRef(t.Name, t.Pos())
		}
	case *ast.SelectorExpr:
		// stack: [.. v]; eval base -> [v base]; swap -> [base v];
		// OpSetField pops v then base.
		c.expr(t.X)
		c.emit(bytecode.OpSwap, 0, 0, t.Pos())
		c.emit(bytecode.OpSetField, c.nameIdx(t.Sel.Name), 0, t.Pos())
	case *ast.IndexExpr:
		// stack: [v]; eval base,idx -> [v base idx]; rot3 -> [base idx v]
		c.expr(t.X)
		c.expr(t.Index)
		c.emit(bytecode.OpRot3, 0, 0, t.Pos())
		c.emit(bytecode.OpSetIndex, 0, 0, t.Pos())
	case *ast.StarExpr:
		c.expr(t.X)
		c.emit(bytecode.OpSwap, 0, 0, t.Pos())
		c.emit(bytecode.OpSetInd, 0, 0, t.Pos())
	default:
		c.trap(lhs.Pos(), "unsupported assignment target %T", lhs)
	}
}

func (c *compiler) ifStmt(st *ast.IfStmt) {
	c.fs.pushBlock()
	if st.Init != nil {
		c.stmt(st.Init)
	}
	c.expr(st.Cond)
	jElse := c.emit(bytecode.OpJumpFalse, 0, 0, st.Cond.Pos())
	c.stmt(st.Body)
	jEnd := c.emit(bytecode.OpJump, 0, 0, st.Pos())
	c.patchA(jElse, len(c.ch.Code))
	if st.Else != nil {
		c.stmt(st.Else)
	}
	c.patchA(jEnd, len(c.ch.Code))
	c.fs.popBlock()
}

func (c *compiler) forStmt(st *ast.ForStmt) {
	c.fs.pushBlock()
	if st.Init != nil {
		c.stmt(st.Init)
	}
	lc := &ctrlCtx{isLoop: true, continueIP: -1}
	c.ctrl = append(c.ctrl, lc)
	condIP := len(c.ch.Code)
	var jEnd int
	if st.Cond != nil {
		c.expr(st.Cond)
		jEnd = c.emit(bytecode.OpJumpFalse, 0, 0, st.Cond.Pos())
	}
	c.stmt(st.Body)
	lc.continueIP = len(c.ch.Code)
	if st.Post != nil {
		c.stmt(st.Post)
	}
	// continues inside the body were emitted before post's position existed
	for _, ci := range lc.continues {
		c.patchA(ci, lc.continueIP)
	}
	c.emit(bytecode.OpJump, condIP, 0, st.Pos())
	end := len(c.ch.Code)
	if st.Cond != nil {
		c.patchA(jEnd, end)
	}
	for _, b := range lc.breaks {
		c.patchA(b, end)
	}
	c.ctrl = c.ctrl[:len(c.ctrl)-1]
	c.fs.popBlock()
}

func (c *compiler) rangeStmt(st *ast.RangeStmt) {
	c.fs.pushBlock()
	c.expr(st.X)
	c.emit(bytecode.OpIter, 0, 0, st.X.Pos())
	itSlot := c.fs.declare("$it")
	c.emit(bytecode.OpNewLocal, itSlot, 0, st.X.Pos())

	nvars := 0
	if st.Key != nil {
		nvars++
	}
	if st.Value != nil {
		nvars++
	}
	lc := &ctrlCtx{isLoop: true}
	c.ctrl = append(c.ctrl, lc)
	topIP := len(c.ch.Code)
	nextI := c.emit3(bytecode.OpRangeNext, 0, itSlot, nvars, st.Pos())
	lc.continueIP = topIP

	// OpRangeNext pushes nvars values (key, val); bind in reverse.
	if st.Value != nil {
		c.bindRangeVar(st.Value, st.Tok == token.DEFINE)
	}
	if st.Key != nil {
		c.bindRangeVar(st.Key, st.Tok == token.DEFINE)
	}
	c.stmt(st.Body)
	c.emit(bytecode.OpJump, topIP, 0, st.Pos())
	end := len(c.ch.Code)
	c.patchA(nextI, end)
	for _, b := range lc.breaks {
		c.patchA(b, end)
	}
	c.ctrl = c.ctrl[:len(c.ctrl)-1]
	c.fs.popBlock()
}

func (c *compiler) bindRangeVar(e ast.Expr, define bool) {
	if id, ok := e.(*ast.Ident); ok && id.Name == "_" {
		c.emit(bytecode.OpPop, 0, 0, e.Pos())
		return
	}
	c.storeTarget(e, define)
}

func (c *compiler) switchStmt(st *ast.SwitchStmt) {
	c.fs.pushBlock()
	if st.Init != nil {
		c.stmt(st.Init)
	}
	tagSlot := -1
	if st.Tag != nil {
		c.expr(st.Tag)
		tagSlot = c.fs.declare("$tag")
		c.emit(bytecode.OpNewLocal, tagSlot, 0, st.Tag.Pos())
	}
	cc := &ctrlCtx{}
	c.ctrl = append(c.ctrl, cc)

	var defaultBody []ast.Stmt
	var jumpOuts []int
	for _, s := range st.Body.List {
		clause := s.(*ast.CaseClause)
		if clause.List == nil {
			defaultBody = clause.Body
			continue
		}
		// tests: tag == e (or truthy e for tag-less switch); JumpTrue -> body
		bodyJumps := []int{}
		for _, e := range clause.List {
			if tagSlot >= 0 {
				c.emit(bytecode.OpLocal, tagSlot, 0, e.Pos())
				c.expr(e)
				c.emit(bytecode.OpBinary, int(bytecode.BinEql), 0, e.Pos())
			} else {
				c.expr(e)
			}
			bodyJumps = append(bodyJumps, c.emit(bytecode.OpJumpTrue, 0, 0, e.Pos()))
		}
		// no test matched: continue to next clause's tests (emitted after
		// this body)
		jNext := c.emit(bytecode.OpJump, 0, 0, clause.Pos())
		bodyStart := len(c.ch.Code)
		for _, bj := range bodyJumps {
			c.patchA(bj, bodyStart)
		}
		for _, bs := range clause.Body {
			c.stmt(bs)
		}
		jumpOuts = append(jumpOuts, c.emit(bytecode.OpJump, 0, 0, clause.Pos()))
		c.patchA(jNext, len(c.ch.Code))
	}
	if defaultBody != nil {
		for _, bs := range defaultBody {
			c.stmt(bs)
		}
	}
	end := len(c.ch.Code)
	for _, j := range jumpOuts {
		c.patchA(j, end)
	}
	for _, b := range cc.breaks {
		c.patchA(b, end)
	}
	c.ctrl = c.ctrl[:len(c.ctrl)-1]
	c.fs.popBlock()
}

// selectStmt compiles select using the single-threaded approximation: every
// case's channel operand (and send value) is evaluated eagerly in source
// order — as the Go spec requires — then the first ready case runs. Send
// cases are ready on an open channel (sends never block); receive cases are
// ready on a non-empty or closed channel. With no ready case, `default`
// runs; without a default the select would block forever, so it traps.
func (c *compiler) selectStmt(st *ast.SelectStmt) {
	c.fs.pushBlock()
	cc := &ctrlCtx{}
	c.ctrl = append(c.ctrl, cc)

	var defaultBody []ast.Stmt
	var exits []int
	for _, s := range st.Body.List {
		clause := s.(*ast.CommClause)
		if clause.Comm == nil {
			defaultBody = clause.Body
			continue
		}
		var jReady int
		var bind func()
		switch comm := clause.Comm.(type) {
		case *ast.SendStmt:
			c.expr(comm.Chan)
			c.expr(comm.Value)
			c.emit(bytecode.OpSelSend, 0, 0, comm.Pos())
			jReady = c.emit(bytecode.OpJumpTrue, 0, 0, comm.Pos())
		default:
			recv, lhs, define := selectRecv(clause.Comm)
			if recv == nil {
				c.trap(clause.Comm.Pos(), "unsupported select case %T", clause.Comm)
				continue
			}
			c.expr(recv.X)
			c.emit(bytecode.OpSelRecv, len(lhs), 0, recv.Pos())
			jReady = c.emit(bytecode.OpJumpTrue, 0, 0, recv.Pos())
			l := lhs
			d := define
			bind = func() { c.bindRecv(l, d) }
		}
		jNext := c.emit(bytecode.OpJump, 0, 0, clause.Pos())
		bodyStart := len(c.ch.Code)
		c.patchA(jReady, bodyStart)
		c.fs.pushBlock()
		if bind != nil {
			bind()
		}
		for _, bs := range clause.Body {
			c.stmt(bs)
		}
		c.fs.popBlock()
		exits = append(exits, c.emit(bytecode.OpJump, 0, 0, clause.Pos()))
		c.patchA(jNext, len(c.ch.Code))
	}
	if defaultBody != nil {
		for _, bs := range defaultBody {
			c.stmt(bs)
		}
	} else {
		c.trap(st.Pos(), "select would block (single-threaded approximation)")
	}
	end := len(c.ch.Code)
	for _, j := range exits {
		c.patchA(j, end)
	}
	for _, b := range cc.breaks {
		c.patchA(b, end)
	}
	c.ctrl = c.ctrl[:len(c.ctrl)-1]
	c.fs.popBlock()
}

// selectRecv extracts a receive case from a CommClause's comm statement:
// `case <-ch`, `case v := <-ch`, `case v, ok := <-ch`, or assignments.
func selectRecv(comm ast.Stmt) (recv *ast.UnaryExpr, lhs []ast.Expr, define bool) {
	arrow := func(e ast.Expr) *ast.UnaryExpr {
		if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.ARROW {
			return u
		}
		return nil
	}
	switch s := comm.(type) {
	case *ast.ExprStmt:
		if u := arrow(s.X); u != nil {
			return u, nil, false
		}
	case *ast.AssignStmt:
		if len(s.Rhs) == 1 {
			if u := arrow(s.Rhs[0]); u != nil {
				return u, s.Lhs, s.Tok == token.DEFINE
			}
		}
	}
	return nil, nil, false
}

// bindRecv binds a select receive payload (already on the stack) to the
// case's LHS: a Tuple for two binds, a plain value for one, discarded else.
func (c *compiler) bindRecv(lhs []ast.Expr, define bool) {
	switch len(lhs) {
	case 0:
		c.emit(bytecode.OpPop, 0, 0, 0)
	case 1:
		c.storeTarget(lhs[0], define)
	default:
		c.emit3(bytecode.OpUnpack, len(lhs), 0, 0, 0)
		for i := len(lhs) - 1; i >= 0; i-- {
			c.storeTarget(lhs[i], define)
		}
	}
}

func (c *compiler) returnStmt(st *ast.ReturnStmt) {
	if len(st.Results) == 0 {
		c.emit(bytecode.OpReturn, -1, 0, st.Pos()) // -1: use named result slots
		return
	}
	for _, r := range st.Results {
		c.expr(r)
	}
	c.emit(bytecode.OpReturn, len(st.Results), 0, st.Pos())
}

func (c *compiler) branchStmt(st *ast.BranchStmt) {
	if st.Label != nil {
		c.trap(st.Pos(), "labeled branch is not supported")
		return
	}
	switch st.Tok {
	case token.BREAK:
		if len(c.ctrl) > 0 {
			cc := c.ctrl[len(c.ctrl)-1]
			cc.breaks = append(cc.breaks, c.emit(bytecode.OpJump, 0, 0, st.Pos()))
			return
		}
		c.trap(st.Pos(), "break outside loop/switch")
	case token.CONTINUE:
		for i := len(c.ctrl) - 1; i >= 0; i-- {
			cc := c.ctrl[i]
			if !cc.isLoop {
				continue
			}
			if cc.continueIP >= 0 {
				c.emit(bytecode.OpJump, cc.continueIP, 0, st.Pos())
			} else {
				cc.continues = append(cc.continues, c.emit(bytecode.OpJump, 0, 0, st.Pos()))
			}
			return
		}
		c.trap(st.Pos(), "continue outside loop")
	case token.FALLTHROUGH:
		c.trap(st.Pos(), "fallthrough is not supported")
	case token.GOTO:
		c.trap(st.Pos(), "goto is not supported")
	}
}

// ---- expressions ----

func (c *compiler) expr(e ast.Expr) {
	switch x := e.(type) {
	case *ast.BasicLit:
		v, err := literalValue(x)
		if err != nil {
			c.trap(x.Pos(), "bad literal: %s", err)
			return
		}
		c.emit(bytecode.OpConst, c.constIdx(v), 0, x.Pos())
	case *ast.Ident:
		switch x.Name {
		case "nil":
			c.emit(bytecode.OpNil, 0, 0, x.Pos())
		case "true":
			c.emit(bytecode.OpConst, c.constIdx(true), 0, x.Pos())
		case "false":
			c.emit(bytecode.OpConst, c.constIdx(false), 0, x.Pos())
		default:
			c.getRef(x.Name, x.Pos())
		}
	case *ast.SelectorExpr:
		c.expr(x.X)
		c.emit(bytecode.OpSelect, c.nameIdx(x.Sel.Name), 0, x.Pos())
	case *ast.IndexExpr:
		c.expr(x.X)
		c.expr(x.Index)
		c.emit(bytecode.OpIndex, 0, 0, x.Pos())
	case *ast.SliceExpr:
		if x.Slice3 {
			c.trap(x.Pos(), "3-index slice is not supported")
			return
		}
		c.expr(x.X)
		if x.Low != nil {
			c.expr(x.Low)
		} else {
			c.emit(bytecode.OpNil, 0, 0, x.Pos())
		}
		if x.High != nil {
			c.expr(x.High)
		} else {
			c.emit(bytecode.OpNil, 0, 0, x.Pos())
		}
		c.emit(bytecode.OpSlice, 0, 0, x.Pos())
	case *ast.StarExpr:
		c.expr(x.X)
		c.emit(bytecode.OpDeref, 0, 0, x.Pos())
	case *ast.ParenExpr:
		c.expr(x.X)
	case *ast.UnaryExpr:
		c.unary(x)
	case *ast.BinaryExpr:
		c.binary(x)
	case *ast.CallExpr:
		c.call(x)
	case *ast.CompositeLit:
		c.compositeLit(x)
	case *ast.FuncLit:
		c.funcLit(x)
	case *ast.TypeAssertExpr:
		c.trap(x.Pos(), "type assertion is not supported yet")
	case *ast.IndexListExpr:
		c.trap(x.Pos(), "generics are not supported yet")
	case *ast.Ellipsis:
		c.trap(x.Pos(), "spread call is not supported")
	case *ast.KeyValueExpr:
		c.trap(x.Pos(), "key:value outside composite literal")
	default:
		c.trap(e.Pos(), "unsupported expression %T", e)
	}
}

func (c *compiler) unary(x *ast.UnaryExpr) {
	switch x.Op {
	case token.AND: // &x
		switch t := x.X.(type) {
		case *ast.Ident:
			c.refRef(t.Name, t.Pos())
		case *ast.CompositeLit:
			c.compositeLit(t)
			c.emit(bytecode.OpBox, 0, 0, x.Pos())
		default:
			c.trap(x.Pos(), "address-of %T is not supported", x.X)
		}
	case token.ADD:
		c.expr(x.X)
		c.emit(bytecode.OpUnary, int(bytecode.UnPos), 0, x.Pos())
	case token.SUB:
		c.expr(x.X)
		c.emit(bytecode.OpUnary, int(bytecode.UnNeg), 0, x.Pos())
	case token.NOT:
		c.expr(x.X)
		c.emit(bytecode.OpUnary, int(bytecode.UnNot), 0, x.Pos())
	case token.XOR:
		c.expr(x.X)
		c.emit(bytecode.OpUnary, int(bytecode.UnXor), 0, x.Pos())
	case token.ARROW:
		c.expr(x.X)
		c.emit(bytecode.OpRecv, 0, 0, x.Pos())
	default:
		c.trap(x.Pos(), "unsupported unary %s", x.Op)
	}
}

func (c *compiler) binary(x *ast.BinaryExpr) {
	switch x.Op {
	case token.LAND:
		c.expr(x.X)
		c.emit(bytecode.OpDup, 0, 0, x.Pos())
		j := c.emit(bytecode.OpJumpFalse, 0, 0, x.Pos())
		c.emit(bytecode.OpPop, 0, 0, x.Pos())
		c.expr(x.Y)
		c.patchA(j, len(c.ch.Code))
	case token.LOR:
		c.expr(x.X)
		c.emit(bytecode.OpDup, 0, 0, x.Pos())
		j := c.emit(bytecode.OpJumpTrue, 0, 0, x.Pos())
		c.emit(bytecode.OpPop, 0, 0, x.Pos())
		c.expr(x.Y)
		c.patchA(j, len(c.ch.Code))
	default:
		op, ok := binOpOf(x.Op)
		if !ok {
			c.trap(x.Pos(), "unsupported binary %s", x.Op)
			return
		}
		c.expr(x.X)
		c.expr(x.Y)
		c.emit(bytecode.OpBinary, int(op), 0, x.Pos())
	}
}

func binOpOf(tok token.Token) (bytecode.BinOp, bool) {
	switch tok {
	case token.ADD:
		return bytecode.BinAdd, true
	case token.SUB:
		return bytecode.BinSub, true
	case token.MUL:
		return bytecode.BinMul, true
	case token.QUO:
		return bytecode.BinQuo, true
	case token.REM:
		return bytecode.BinRem, true
	case token.AND:
		return bytecode.BinAnd, true
	case token.OR:
		return bytecode.BinOr, true
	case token.XOR:
		return bytecode.BinXor, true
	case token.AND_NOT:
		return bytecode.BinAndNot, true
	case token.SHL:
		return bytecode.BinShl, true
	case token.SHR:
		return bytecode.BinShr, true
	case token.LAND:
		return bytecode.BinLAnd, true
	case token.LOR:
		return bytecode.BinLOr, true
	case token.EQL:
		return bytecode.BinEql, true
	case token.NEQ:
		return bytecode.BinNeq, true
	case token.LSS:
		return bytecode.BinLss, true
	case token.LEQ:
		return bytecode.BinLeq, true
	case token.GTR:
		return bytecode.BinGtr, true
	case token.GEQ:
		return bytecode.BinGeq, true
	case token.ADD_ASSIGN:
		return bytecode.BinAdd, true
	case token.SUB_ASSIGN:
		return bytecode.BinSub, true
	case token.MUL_ASSIGN:
		return bytecode.BinMul, true
	case token.QUO_ASSIGN:
		return bytecode.BinQuo, true
	case token.REM_ASSIGN:
		return bytecode.BinRem, true
	case token.AND_ASSIGN:
		return bytecode.BinAnd, true
	case token.OR_ASSIGN:
		return bytecode.BinOr, true
	case token.XOR_ASSIGN:
		return bytecode.BinXor, true
	case token.SHL_ASSIGN:
		return bytecode.BinShl, true
	case token.SHR_ASSIGN:
		return bytecode.BinShr, true
	}
	return 0, false
}

func (c *compiler) call(x *ast.CallExpr) {
	// variadic spread f(xs...) is unsupported
	if x.Ellipsis.IsValid() {
		c.trap(x.Pos(), "spread calls are not supported")
		return
	}
	c.calleeExpr(x.Fun)
	for i, a := range x.Args {
		// make(T, ...) / new(T) take a type as first argument
		if i == 0 && isTypePositionCall(x) {
			c.typeExpr(a)
			continue
		}
		c.expr(a)
	}
	c.emit(bytecode.OpCall, len(x.Args), 0, x.Pos())
}

// calleeExpr compiles the called expression; a syntactic type form means a
// conversion call T(x).
func (c *compiler) calleeExpr(fun ast.Expr) {
	if isTypeForm(fun) {
		c.typeExpr(fun)
		return
	}
	c.expr(fun)
}

// isTypePositionCall reports whether the call's first argument is a type
// (make/new builtins).
func isTypePositionCall(x *ast.CallExpr) bool {
	id, ok := x.Fun.(*ast.Ident)
	return ok && (id.Name == "make" || id.Name == "new")
}

// isTypeForm reports whether e is syntactically a type expression (and thus
// a conversion when used as a call callee).
func isTypeForm(e ast.Expr) bool {
	switch e.(type) {
	case *ast.ArrayType, *ast.MapType, *ast.StructType, *ast.FuncType,
		*ast.InterfaceType, *ast.StarExpr, *ast.ChanType:
		return true
	}
	return false
}

func (c *compiler) compositeLit(x *ast.CompositeLit) {
	c.typeExpr(x.Type)
	kv := false
	for _, el := range x.Elts {
		if _, ok := el.(*ast.KeyValueExpr); ok {
			kv = true
			break
		}
	}
	for _, el := range x.Elts {
		if kv {
			kvel := el.(*ast.KeyValueExpr)
			// In struct literals the key is a field name, not an expression.
			// (For maps an identifier key is a real expression — we can't
			// distinguish statically without types, so identifiers compile
			// to their name string; map keys requiring identifiers are a
			// known gap for this phase.)
			if id, ok := kvel.Key.(*ast.Ident); ok {
				c.emit(bytecode.OpConst, c.constIdx(id.Name), 0, id.Pos())
			} else {
				c.expr(kvel.Key)
			}
			c.expr(kvel.Value)
		} else {
			c.expr(el)
		}
	}
	b := 0
	if kv {
		b = 1
	}
	c.emit(bytecode.OpMakeComposite, len(x.Elts), b, x.Pos())
}

// typeExpr emits a push of *runtime.TypeDef for a type expression.
func (c *compiler) typeExpr(e ast.Expr) {
	switch t := e.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		// named type: resolves through globals/imports at run time
		c.expr(t)
	case *ast.ArrayType:
		c.emit(bytecode.OpConst, c.constIdx(&runtime.TypeDef{Kind: runtime.KindSlice}), 0, e.Pos())
	case *ast.MapType:
		c.emit(bytecode.OpConst, c.constIdx(&runtime.TypeDef{Kind: runtime.KindMap}), 0, e.Pos())
	case *ast.StarExpr:
		c.typeExpr(t.X) // pointer types collapse to their element typedef for MVP
	case *ast.StructType:
		td := &runtime.TypeDef{Kind: runtime.KindStruct}
		for _, f := range t.Fields.List {
			for _, n := range f.Names {
				td.Fields = append(td.Fields, n.Name)
			}
		}
		c.emit(bytecode.OpConst, c.constIdx(td), 0, e.Pos())
	case *ast.FuncType:
		c.emit(bytecode.OpConst, c.constIdx(&runtime.TypeDef{Kind: runtime.KindFunc}), 0, e.Pos())
	case *ast.InterfaceType:
		c.trap(e.Pos(), "interface types are not supported yet")
	case *ast.ParenExpr:
		c.typeExpr(t.X)
	case *ast.IndexExpr, *ast.IndexListExpr:
		c.trap(e.Pos(), "generic type instantiation is not supported")
	case *ast.Ellipsis:
		c.typeExpr(t.Elt)
	case *ast.ChanType:
		c.emit(bytecode.OpConst, c.constIdx(&runtime.TypeDef{Kind: runtime.KindChan}), 0, e.Pos())
	default:
		c.trap(e.Pos(), "unsupported type expression %T", e)
	}
}

// funcLit compiles a function literal into a separate chunk and emits a
// closure creation.
func (c *compiler) funcLit(x *ast.FuncLit) {
	inner := &runtime.Function{Pkg: c.pkg, File: c.file, Name: "<funclit>"}
	ic := &compiler{pkg: c.pkg, file: c.file, fs: newFScope(c.fs), ch: &bytecode.Chunk{Name: "<funclit>"}}
	ic.fs.pushBlock()
	nparams := 0
	if x.Type.Params != nil {
		for _, field := range x.Type.Params.List {
			names := field.Names
			if len(names) == 0 {
				names = []*ast.Ident{{Name: fmt.Sprintf("$arg%d", nparams)}}
			}
			for _, n := range names {
				ic.fs.declare(n.Name)
				nparams++
			}
			if _, ok := field.Type.(*ast.Ellipsis); ok {
				ic.ch.IsVararg = true
			}
		}
	}
	ic.ch.NParams = nparams
	if x.Type.Results != nil {
		for _, field := range x.Type.Results.List {
			for _, n := range field.Names {
				slot := ic.fs.declare(n.Name)
				ic.ch.NamedSlots = append(ic.ch.NamedSlots, slot)
				ic.emit(bytecode.OpNil, 0, 0, n.Pos())
				ic.emit(bytecode.OpNewLocal, slot, 0, n.Pos())
			}
		}
		ic.ch.NResults = countResults(x.Type.Results)
	}
	ic.stmt(x.Body)
	ic.emit(bytecode.OpReturn, ic.ch.NResults, 0, x.End())
	ic.ch.NLocals = ic.fs.nlocals
	ic.ch.Upvals = ic.fs.upvals
	inner.Chunk = ic.ch

	c.emit(bytecode.OpMakeClosure, c.constIdx(inner), 0, x.Pos())
}

// literalValue converts a BasicLit to a runtime value.
func literalValue(l *ast.BasicLit) (any, error) {
	switch l.Kind {
	case token.INT:
		v := constant.MakeFromLiteral(l.Value, token.INT, 0)
		if i, ok := constant.Int64Val(v); ok {
			return i, nil
		}
		return nil, fmt.Errorf("int literal out of range: %s", l.Value)
	case token.FLOAT:
		v := constant.MakeFromLiteral(l.Value, token.FLOAT, 0)
		f, _ := constant.Float64Val(v)
		return f, nil
	case token.STRING:
		return strconv.Unquote(l.Value)
	case token.CHAR:
		s, err := strconv.Unquote(l.Value)
		if err != nil {
			return nil, err
		}
		r := []rune(s)
		if len(r) != 1 {
			return nil, fmt.Errorf("multi-char literal %s", l.Value)
		}
		return int64(r[0]), nil
	}
	return nil, fmt.Errorf("unsupported literal kind %s", l.Kind)
}

// orderSpecs sorts var/const specs in dependency order (Go spec: package-level
// initialization proceeds in dependency order, with source order as the
// tie-breaker). A spec depends on every package-level name free in its value
// expressions — including names reached transitively through function bodies
// (`var x = f()` depends on every package-level var f reads, and on what
// functions f calls read, recursively). Cyclic leftovers keep source order.
func orderSpecs(ix *index.Index, reps []*index.Decl) []*index.Decl {
	declared := map[string]bool{}
	for n := range ix.Vars {
		declared[n] = true
	}
	for n := range ix.Consts {
		declared[n] = true
	}

	// idents referenced by an AST (also covers nested func literals)
	refs := func(n ast.Node, out map[string]bool) {
		ast.Inspect(n, func(x ast.Node) bool {
			if id, ok := x.(*ast.Ident); ok {
				out[id.Name] = true
			}
			return true
		})
	}

	// funcRefs(name) = package-level names reachable from the function's
	// body, transitively through other functions/methods it references.
	funcRefsCache := map[string]map[string]bool{}
	var funcRefs func(name string, depth int) map[string]bool
	funcRefs = func(name string, depth int) map[string]bool {
		if depth > 16 {
			return nil // recursion budget: cycles/deep chains keep source order
		}
		if r, ok := funcRefsCache[name]; ok {
			return r
		}
		d := ix.Funcs[name]
		if d == nil {
			for _, td := range ix.Types {
				if m := td.Methods[name]; m != nil {
					d = m
					break
				}
			}
		}
		if d == nil {
			return nil
		}
		r := map[string]bool{}
		refs(d.Func, r)
		// follow function-valued references one level further
		for n := range r {
			if declared[n] {
				continue
			}
			for m := range funcRefs(n, depth+1) {
				r[m] = true
			}
		}
		funcRefsCache[name] = r
		return r
	}

	// spec -> specs it depends on (a spec provides its names)
	providedBy := map[string]*ast.ValueSpec{}
	for _, d := range reps {
		vs := d.Spec.(*ast.ValueSpec)
		for _, n := range vs.Names {
			providedBy[n.Name] = vs
		}
	}

	deps := map[*ast.ValueSpec]map[*ast.ValueSpec]bool{}
	for _, d := range reps {
		vs := d.Spec.(*ast.ValueSpec)
		vals := vs.Values
		if len(vals) == 0 {
			vals = d.Inherited
		}
		ds := map[*ast.ValueSpec]bool{}
		names := map[string]bool{}
		for _, e := range vals {
			refs(e, names)
		}
		// widen direct references through function bodies
		for n := range names {
			if declared[n] {
				continue
			}
			for m := range funcRefs(n, 0) {
				names[m] = true
			}
		}
		for n := range names {
			if !declared[n] {
				continue
			}
			if dep := providedBy[n]; dep != nil && dep != vs {
				ds[dep] = true
			}
		}
		deps[vs] = ds
	}

	// stable Kahn: repeatedly emit the first spec whose deps are all done
	var out []*index.Decl
	done := map[*ast.ValueSpec]bool{}
	remaining := reps
	for len(remaining) > 0 {
		progress := false
		var next []*index.Decl
		for _, d := range remaining {
			vs := d.Spec.(*ast.ValueSpec)
			ready := true
			for dep := range deps[vs] {
				if !done[dep] {
					ready = false
					break
				}
			}
			if ready {
				out = append(out, d)
				done[vs] = true
				progress = true
			} else {
				next = append(next, d)
			}
		}
		if !progress {
			out = append(out, next...) // cycle: keep source order
			break
		}
		remaining = next
	}
	return out
}
