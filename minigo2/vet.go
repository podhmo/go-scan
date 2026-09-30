// Vet pass: lists call sites into stub packages that are neither registered
// special forms nor bound host symbols. A stub member's body is
// `panic("minigo intrinsic")` — reaching it at run time is a loud but late
// failure, so this checker surfaces the same mistake statically (plan §16).
package minigo2

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"sort"
	"strconv"

	"github.com/podhmo/go-scan/minigo2/index"
	"github.com/podhmo/go-scan/minigo2/runtime"
)

// stubPanic is the marker body of a stub-package member: a declaration that
// exists only to satisfy gofmt/gopls and is meant to be intercepted by a
// special form or a bound host symbol.
const stubPanic = "minigo intrinsic"

// VetResult is one vet finding: an unregistered call into a stub member.
type VetResult struct {
	Pos      token.Position
	Call     string // "pkg.Sym" as written (alias.Sym form)
	Resolved string // canonical "import/path.Sym" it resolves to
}

func (r VetResult) String() string {
	return fmt.Sprintf("%s: call %s resolves to unregistered stub member %s", r.Pos, r.Call, r.Resolved)
}

// Vet loads ref (without initializing it) and reports every call site
// `pkgAlias.Sym(...)` where Sym's declaration is a stub (body is exactly
// `panic("minigo intrinsic")`) and the symbol is neither a registered
// special form nor a bound host symbol on this engine. Dot imports are not
// tracked — a `Sym(...)` call into a dot-imported stub package is invisible
// to the checker.
func (e *Engine) Vet(ctx context.Context, ref string) ([]VetResult, error) {
	p, err := e.Package(ctx, ref)
	if err != nil {
		return nil, err
	}
	var out []VetResult
	for _, sf := range p.Files {
		imports := map[string]string{} // local name -> import path
		for _, im := range sf.Imports {
			if im.Alias == "_" || im.Alias == "." {
				continue
			}
			imports[im.LocalName()] = im.Path
		}
		ast.Inspect(sf.AST, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			x, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			path, ok := imports[x.Name]
			if !ok {
				return true // method call or local value selector
			}
			name := sel.Sel.Name
			if e.registeredVetSymbol(path, name) {
				return true
			}
			if !e.stubMember(ctx, path, name) {
				return true
			}
			out = append(out, VetResult{
				Pos:      e.fset.Position(call.Lparen),
				Call:     x.Name + "." + name,
				Resolved: path + "." + name,
			})
			return true
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Pos.Filename < out[j].Pos.Filename ||
			(out[i].Pos.Filename == out[j].Pos.Filename && out[i].Pos.Line < out[j].Pos.Line)
	})
	return out, nil
}

// registeredVetSymbol reports whether path.Name is already intercepted:
// a special form the compiler emits OpSpecialCall for, or a symbol in a
// host-bound package.
func (e *Engine) registeredVetSymbol(path, name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.specials[runtime.SymbolID{PackagePath: path, Name: name}]; ok {
		return true
	}
	if b, ok := e.binds[path]; ok {
		if _, ok := b.Globals.Get(name); ok {
			return true
		}
	}
	return false
}

// stubMember reports whether path.Name resolves to a stub declaration: a
// function whose body is exactly `panic("minigo intrinsic")`. The callee
// package is located and indexed but never initialized; packages that
// cannot be located answer false (nothing to inspect).
func (e *Engine) stubMember(ctx context.Context, path, name string) bool {
	p, err := e.Package(ctx, path)
	if err != nil || p.Index == nil {
		return false
	}
	d, ok := p.Index.Funcs[name]
	if !ok || d.Kind != index.FuncDecl || d.Func == nil || d.Func.Body == nil {
		return false
	}
	if len(d.Func.Body.List) != 1 {
		return false
	}
	stmt, ok := d.Func.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := stmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "panic" || len(call.Args) != 1 {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	msg, err := strconv.Unquote(lit.Value)
	return err == nil && msg == stubPanic
}
