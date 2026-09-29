package minigo2

import (
	"fmt"
	"go/ast"
	"reflect"

	"github.com/podhmo/go-scan/minigo2/runtime"
)

// ---- interface satisfaction, method sets, embedded dispatch ----
//
// Duck-typing: an interface typedef carries its required method names
// (declared + transitively embedded); a value satisfies it when its
// callable method names cover the requirement set. Method names on
// structs include promoted methods via embedded fields, computed lazily
// here because promotion needs type resolution the index doesn't do.

// methodsOfValue returns the method names callable on a dynamic value.
// Structs offer declared + promoted methods; host GoValues expose their
// reflect method set. Other values have no methods.
func (e *Engine) methodsOfValue(v runtime.Value) (map[string]bool, error) {
	for {
		dv, ok := runtime.Deref(v)
		if !ok {
			break
		}
		v = dv
	}
	switch x := v.(type) {
	case *runtime.Struct:
		return e.methodSetOf(x.Def, map[*runtime.TypeDef]bool{}), nil
	case *runtime.GoValue:
		t := reflect.TypeOf(x.V)
		set := map[string]bool{}
		for i := 0; i < t.NumMethod(); i++ {
			set[t.Method(i).Name] = true
		}
		return set, nil
	default:
		return nil, nil
	}
}

// methodSetOf collects declared + promoted method names of a typedef.
func (e *Engine) methodSetOf(td *runtime.TypeDef, seen map[*runtime.TypeDef]bool) map[string]bool {
	if td == nil || seen[td] {
		return nil
	}
	seen[td] = true
	set := map[string]bool{}
	for m := range td.Methods {
		set[m] = true
	}
	for _, spec := range td.EmbedSpecs {
		emb, err := e.resolveTypeRef(td, spec)
		if err != nil || emb == nil {
			continue
		}
		if emb.Kind == runtime.KindInterface {
			// an embedded interface field satisfies its own requirements
			for m := range e.ifaceReqsRec(emb, map[*runtime.TypeDef]bool{}) {
				set[m] = true
			}
			continue
		}
		for m := range e.methodSetOf(emb, seen) {
			set[m] = true
		}
	}
	return set
}

// ifaceReqs returns the required method set of an interface typedef:
// declared methods union the requirements of embedded interface elements.
// Constraint elements (~T, unions) are approximated away — satisfaction
// checks treat them as fulfilled.
func (e *Engine) ifaceReqs(td *runtime.TypeDef) (map[string]bool, error) {
	return e.ifaceReqsRec(td, map[*runtime.TypeDef]bool{}), nil
}

func (e *Engine) ifaceReqsRec(td *runtime.TypeDef, seen map[*runtime.TypeDef]bool) map[string]bool {
	if td == nil || seen[td] {
		return nil
	}
	seen[td] = true
	set := map[string]bool{}
	for _, m := range td.MReqs {
		set[m] = true
	}
	for _, spec := range td.IEmbeds {
		sub, err := e.resolveTypeRef(td, spec)
		if err != nil || sub == nil {
			continue // constraint exprs (~T, |) don't resolve to typedefs
		}
		for m := range e.ifaceReqsRec(sub, seen) {
			set[m] = true
		}
	}
	return set
}

// findMethod resolves a promoted method on a struct through its embedded
// fields: (fn, recv, true) binds fn to the embedded field value. For an
// embedded interface field it returns (nil, field, true) — the VM selects
// the member on the concrete value stored there.
func (e *Engine) findMethod(s *runtime.Struct, name string) (*runtime.Function, runtime.Value, bool) {
	def := s.Def
	for i, spec := range def.EmbedSpecs {
		emb, err := e.resolveTypeRef(def, spec)
		if err != nil || emb == nil {
			continue
		}
		idx := def.EmbedIdx[i]
		if idx >= len(s.Fields) {
			continue
		}
		recv := s.Fields[idx]
		if emb.Kind == runtime.KindInterface {
			// the field holds a concrete value implementing the interface
			if reqs := e.ifaceReqsRec(emb, map[*runtime.TypeDef]bool{}); reqs[name] {
				return nil, recv, true
			}
			continue
		}
		if m, ok := emb.Methods[name]; ok {
			return m, recv, true
		}
		// deeper: promoted through nested embeds of the field's own type
		if rs, ok := structOf(recv); ok {
			if m, rr, ok := e.findMethod(rs, name); ok {
				return m, rr, true
			}
		}
	}
	return nil, nil, false
}

func structOf(v runtime.Value) (*runtime.Struct, bool) {
	for {
		if s, ok := v.(*runtime.Struct); ok {
			return s, true
		}
		dv, ok := runtime.Deref(v)
		if !ok {
			return nil, false
		}
		v = dv
	}
}

// resolveTypeRef resolves a type expression embedded in a decl of typedef
// `from` to a *TypeDef: package-local names via the index, pkg.Name via
// import refs, *T / T[...] peel to the base. Anything else fails —
// constraints and underlying-only types don't participate in method sets.
func (e *Engine) resolveTypeRef(from *runtime.TypeDef, x ast.Expr) (*runtime.TypeDef, error) {
	switch t := x.(type) {
	case *ast.StarExpr:
		return e.resolveTypeRef(from, t.X)
	case *ast.ParenExpr:
		return e.resolveTypeRef(from, t.X)
	case *ast.IndexExpr:
		return e.resolveTypeRef(from, t.X)
	case *ast.IndexListExpr:
		return e.resolveTypeRef(from, t.X)
	case *ast.ArrayType:
		return &runtime.TypeDef{Kind: runtime.KindSlice, Anon: t, Pkg: from.Pkg, File: from.File}, nil
	case *ast.MapType:
		return &runtime.TypeDef{Kind: runtime.KindMap, Anon: t, Pkg: from.Pkg, File: from.File}, nil
	case *ast.ChanType:
		return &runtime.TypeDef{Kind: runtime.KindChan, Anon: t, Pkg: from.Pkg, File: from.File}, nil
	case *ast.Ident:
		if from.Pkg != nil && from.Pkg.Index != nil {
			if info, ok := from.Pkg.Index.Types[t.Name]; ok && info.Decl != nil {
				vv, err := e.materialize(from.Pkg, info.Decl)
				if err != nil {
					return nil, err
				}
				if td, ok := vv.(*runtime.TypeDef); ok {
					return td, nil
				}
				return nil, fmt.Errorf("%s is not a type", t.Name)
			}
		}
		if bv, ok := e.builtins.Get(t.Name); ok {
			if td, ok := bv.(*runtime.TypeDef); ok {
				return td, nil
			}
		}
		return nil, fmt.Errorf("cannot resolve type %s", t.Name)
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok || from.Pkg == nil {
			return nil, fmt.Errorf("cannot resolve embedded type")
		}
		var scope map[string]*runtime.ImportRef
		if from.File != nil {
			scope = from.Pkg.Scopes[from.File]
		}
		ref, ok := scope[id.Name]
		if !ok {
			return nil, fmt.Errorf("unknown import %s", id.Name)
		}
		p, err := ref.Materialize()
		if err != nil {
			return nil, err
		}
		m, err := p.Member(t.Sel.Name, e.materialize)
		if err != nil {
			return nil, err
		}
		td, ok := m.(*runtime.TypeDef)
		if !ok {
			return nil, fmt.Errorf("%s.%s is not a type", id.Name, t.Sel.Name)
		}
		return td, nil
	}
	return nil, fmt.Errorf("unsupported embedded type expression %T", x)
}

// elemOf implements the Hooks.ElemOf hook: the element typedef of a
// container typedef, resolved from its underlying type AST (Anon or
// Spec.Type). Used by elided composite literal elements.
func (e *Engine) elemOf(td *runtime.TypeDef) (*runtime.TypeDef, error) {
	x := td.Anon
	if x == nil && td.Spec != nil {
		x = td.Spec.Type
	}
	for {
		switch t := x.(type) {
		case *ast.ParenExpr:
			x = t.X
			continue
		case *ast.StarExpr:
			x = t.X
			continue
		case *ast.Ellipsis:
			x = t.Elt
			continue
		case *ast.ArrayType:
			return e.resolveTypeRef(td, t.Elt)
		case *ast.MapType:
			return e.resolveTypeRef(td, t.Value)
		case *ast.ChanType:
			return e.resolveTypeRef(td, t.Value)
		}
		return nil, fmt.Errorf("cannot infer element type of %s", td.Name)
	}
}

// ---- shared helpers ----

// typeParamNames extracts parameter names from a type parameter list.
func typeParamNames(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var out []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			out = append(out, n.Name)
		}
	}
	return out
}

// embedBaseName derives the field name of an anonymous (embedded) struct
// field: the base type name, ignoring pointers, packages and type args.
func embedBaseName(x ast.Expr) string {
	switch t := x.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embedBaseName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return embedBaseName(t.X)
	case *ast.IndexListExpr:
		return embedBaseName(t.X)
	}
	return ""
}
