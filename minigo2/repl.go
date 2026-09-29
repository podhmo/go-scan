package minigo2

import (
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"

	"github.com/podhmo/go-scan/minigo2/index"
	"github.com/podhmo/go-scan/minigo2/runtime"
	"github.com/podhmo/go-scan/minigo2/syntax"
)

// REPL is a persistent Read-Eval-Print-Loop session over a scratch package.
// Each input line is classified as a top-level declaration (accumulated for
// later lines) or as statements, which run as a generated step function.
// Names introduced by `x := e`, `var` and `const` inputs are promoted to
// package globals so they persist across lines. `x := e` inside a REPL line
// therefore re-uses an existing global instead of shadowing it — a deliberate
// divergence from Go's scoping rules, matching Python-style REPL semantics.
type REPL struct {
	engine  *Engine // session engine (created fresh by NewREPL)
	pkg     *runtime.Package
	imports []string // import specs, e.g. `"fmt"` or `f "fmt"`
	decls   []string // accumulated func/type declarations
	steps   []string // generated step function sources
	n       int
}

// NewREPL creates a persistent REPL session on a fresh engine derived from
// the receiver's configuration.
func (e *Engine) NewREPL() *REPL {
	sess := e.NewSession()
	p := &runtime.Package{
		Path:     "<repl>",
		Name:     "repl",
		State:    runtime.Parsed,
		Fset:     sess.fset,
		Globals:  runtime.NewEnv(),
		Scopes:   map[*syntax.File]map[string]*runtime.ImportRef{},
		Imports:  map[*syntax.File][]*runtime.ImportRef{},
		Specials: sess.specials,
	}
	p.Bootstrap = sess.bootstrap
	return &REPL{engine: sess, pkg: p}
}

// Reset clears all accumulated state: values, declarations and imports.
func (r *REPL) Reset() {
	fresh := r.engine.NewREPL()
	*r = *fresh
}

// EvalLine evaluates one REPL input and returns its value. Declaration input
// (imports, func and type declarations) updates the persistent package and
// returns nil. Statement input runs as a generated step function; a trailing
// expression statement becomes the return value. Lines beginning with `:` are
// meta commands handled by the caller, not EvalLine.
func (r *REPL) EvalLine(ctx context.Context, input string) (runtime.Value, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return runtime.NIL, nil
	}

	// Classify: does the input parse as top-level declarations?
	fset := token.NewFileSet()
	if sf, err := syntax.ParseFile(fset, "repl-decl.go", []byte("package repl\n"+input)); err == nil {
		step, err := r.acceptDecls(fset, sf.AST)
		if err != nil {
			return nil, err
		}
		if err := r.reload(); err != nil {
			return nil, err
		}
		// Blank imports registered after package init need explicit
		// initialization: the synthetic __init__ runs once, on the first
		// reload, before this import existed.
		file := r.pkg.Files[0]
		for _, ref := range r.pkg.Imports[file] {
			if ref.Alias != "_" {
				continue
			}
			p, err := ref.Materialize()
			if err != nil {
				return nil, err
			}
			if err := p.EnsureReady(); err != nil {
				return nil, err
			}
		}
		if step == "" {
			return runtime.NIL, nil
		}
		return r.engine.Call(ctx, r.pkg, step)
	}

	// Otherwise parse the input as function-body statements.
	fset = token.NewFileSet()
	sf, err := syntax.ParseFile(fset, "repl-stmt.go", []byte("package repl\nfunc __s() any {\n"+input+"\n}"))
	if err != nil {
		return nil, fmt.Errorf("repl: cannot parse input: %w", err)
	}
	fn, ok := sf.AST.Decls[0].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		return nil, fmt.Errorf("repl: cannot parse input")
	}
	step, err := r.acceptStmts(fset, fn.Body.List)
	if err != nil {
		return nil, err
	}
	if err := r.reload(); err != nil {
		return nil, err
	}
	if step == "" {
		return runtime.NIL, nil
	}
	return r.engine.Call(ctx, r.pkg, step)
}

// acceptDecls folds top-level declarations into the REPL state and returns
// the generated step name when the input needs runtime evaluation (var/const
// declarations with values).
func (r *REPL) acceptDecls(fset *token.FileSet, f *ast.File) (string, error) {
	var stepBody []ast.Stmt
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.GenDecl:
			switch d.Tok {
			case token.IMPORT:
				for _, spec := range d.Specs {
					r.imports = append(r.imports, formatNode(fset, spec))
				}
			case token.VAR, token.CONST:
				stepBody = append(stepBody, r.hoistSpecs(d)...)
			case token.TYPE:
				r.decls = append(r.decls, formatNode(fset, d))
			default:
				return "", fmt.Errorf("repl: unsupported declaration: %s", d.Tok)
			}
		case *ast.FuncDecl:
			r.decls = append(r.decls, formatNode(fset, d))
		case *ast.BadDecl:
			return "", fmt.Errorf("repl: cannot parse declaration")
		default:
			return "", fmt.Errorf("repl: unsupported declaration")
		}
	}
	return r.addStep(fset, stepBody)
}

// hoistSpecs promotes each declared name to a package-global cell and lowers
// the spec to assignments executed as a step.
func (r *REPL) hoistSpecs(d *ast.GenDecl) []ast.Stmt {
	var out []ast.Stmt
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			r.hoist(name.Name)
		}
		if len(vs.Values) > 0 {
			lhs := make([]ast.Expr, len(vs.Names))
			for i, n := range vs.Names {
				lhs[i] = n
			}
			out = append(out, &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: vs.Values})
			continue
		}
		// `var x T` without a value initializes to a zero value.
		if vs.Type != nil {
			for _, n := range vs.Names {
				out = append(out, &ast.AssignStmt{
					Lhs: []ast.Expr{n},
					Tok: token.ASSIGN,
					Rhs: []ast.Expr{&ast.StarExpr{X: &ast.CallExpr{
						Fun:  ast.NewIdent("new"),
						Args: []ast.Expr{vs.Type},
					}}},
				})
			}
		}
	}
	return out
}

// acceptStmts rewrites a statement list so new `:=`/`var`/`const` names become
// package globals, and turns a trailing expression statement into the step's
// return value.
func (r *REPL) acceptStmts(fset *token.FileSet, body []ast.Stmt) (string, error) {
	out := make([]ast.Stmt, 0, len(body))
	for _, st := range body {
		switch s := st.(type) {
		case *ast.AssignStmt:
			if s.Tok == token.DEFINE {
				for _, lhs := range s.Lhs {
					if id, ok := lhs.(*ast.Ident); ok {
						r.hoist(id.Name)
					}
				}
				s.Tok = token.ASSIGN
			}
			out = append(out, s)
		case *ast.DeclStmt:
			if gd, ok := s.Decl.(*ast.GenDecl); ok && (gd.Tok == token.VAR || gd.Tok == token.CONST) {
				out = append(out, r.hoistSpecs(gd)...)
				continue
			}
			out = append(out, s)
		default:
			out = append(out, s)
		}
	}
	if n := len(out); n > 0 {
		if es, ok := out[n-1].(*ast.ExprStmt); ok {
			out[n-1] = &ast.ReturnStmt{Results: []ast.Expr{es.X}}
		}
	}
	return r.addStep(fset, out)
}

// hoist registers name as a persistent package-global cell, preserving an
// existing entry's value.
func (r *REPL) hoist(name string) {
	if name == "_" {
		return
	}
	if _, ok := r.pkg.Globals.Get(name); ok {
		return
	}
	r.pkg.Globals.Set(name, &runtime.Cell{Elem: runtime.NIL})
}

// addStep emits `func __stepN() any { <body> }` and returns its name; it
// returns "" when there is nothing to run.
func (r *REPL) addStep(fset *token.FileSet, body []ast.Stmt) (string, error) {
	if len(body) == 0 {
		return "", nil
	}
	// the step declares one result: fall through to `return nil` so the
	// implicit OpReturn never pops an empty stack
	if _, ok := body[len(body)-1].(*ast.ReturnStmt); !ok {
		body = append(body, &ast.ReturnStmt{Results: []ast.Expr{ast.NewIdent("nil")}})
	}
	r.n++
	name := fmt.Sprintf("__step%d", r.n)
	fn := &ast.FuncDecl{
		Name: ast.NewIdent(name),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("any")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
	r.steps = append(r.steps, formatNode(fset, fn))
	return name, nil
}

// reload re-parses the accumulated source, keeping Globals and State so
// hoisted values survive.
func (r *REPL) reload() error {
	var b strings.Builder
	b.WriteString("package repl\n")
	if len(r.imports) > 0 {
		b.WriteString("import (\n")
		for _, spec := range r.imports {
			b.WriteString("\t" + spec + "\n")
		}
		b.WriteString(")\n")
	}
	for _, d := range r.decls {
		b.WriteString(d + "\n")
	}
	for _, s := range r.steps {
		b.WriteString(s + "\n")
	}

	sf, err := syntax.ParseFile(r.engine.fset, "repl.go", []byte(b.String()))
	if err != nil {
		return fmt.Errorf("repl: internal error: accumulated source does not parse: %w\n%s", err, b.String())
	}
	idx, err := index.Build([]*syntax.File{sf})
	if err != nil {
		return fmt.Errorf("repl: internal error: %w", err)
	}
	p := r.pkg
	p.Files = []*syntax.File{sf}
	p.FileByName = map[string]*syntax.File{sf.Name: sf}
	p.Index = idx
	p.Scopes = map[*syntax.File]map[string]*runtime.ImportRef{sf: {}}
	p.Imports = map[*syntax.File][]*runtime.ImportRef{sf: {}}
	for _, imp := range sf.Imports {
		ref := &runtime.ImportRef{
			Path:  imp.Path,
			Alias: imp.Alias,
			Load:  func(path string) (*runtime.Package, error) { return r.engine.loadPath(context.Background(), path) },
		}
		p.Imports[sf] = append(p.Imports[sf], ref)
		if imp.Alias != "_" && imp.Alias != "." {
			p.Scopes[sf][imp.LocalName()] = ref
		}
	}
	return p.EnsureReady()
}

// Display renders a runtime value for REPL output.
func (r *REPL) Display(v runtime.Value) any {
	return display(v)
}

// formatNode prints a single AST node.
func formatNode(fset *token.FileSet, n ast.Node) string {
	var b strings.Builder
	if err := format.Node(&b, fset, n); err != nil {
		return fmt.Sprintf("/* format error: %s */", err)
	}
	return b.String()
}
