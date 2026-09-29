package runtime

import (
	"fmt"
	"go/token"
	"sync"

	"github.com/podhmo/go-scan/minigo2/index"
	"github.com/podhmo/go-scan/minigo2/syntax"
)

// State is the package lifecycle stage.
type State int

const (
	Unseen State = iota
	Located
	Parsed
	Indexed
	Initializing
	Ready
	Failed
)

// Env is a name -> Value map for package globals (and builtins).
type Env struct {
	m map[string]Value
}

// NewEnv creates an empty Env.
func NewEnv() *Env { return &Env{m: map[string]Value{}} }

// Get returns the value bound to name.
func (e *Env) Get(name string) (Value, bool) {
	v, ok := e.m[name]
	return v, ok
}

// Set binds name to v.
func (e *Env) Set(name string, v Value) { e.m[name] = v }

// Names lists bound names.
func (e *Env) Names() []string {
	out := make([]string, 0, len(e.m))
	for k := range e.m {
		out = append(out, k)
	}
	return out
}

// ImportRef is the file-scope handle for one import. It materializes the
// target package (to Indexed) on first Lookup, and fully initializes it
// (Ready) on first member access.
type ImportRef struct {
	Path  string
	Alias string // "", "_", ".", or an identifier

	// Load materializes the package to Indexed state (injected by loader).
	Load func(path string) (*Package, error)

	once sync.Once
	pkg  *Package
	err  error
}

// Materialize loads the package to Indexed (parses + indexes, no init).
func (r *ImportRef) Materialize() (*Package, error) {
	r.once.Do(func() {
		if r.Load == nil {
			r.err = fmt.Errorf("no loader for import %q", r.Path)
			return
		}
		r.pkg, r.err = r.Load(r.Path)
	})
	return r.pkg, r.err
}

// Package is a lazily materialized package.
type Package struct {
	Path  string
	Name  string
	State State
	Dir   string

	Fset  *token.FileSet
	Files []*syntax.File
	Index *index.Index

	Globals *Env // values populated at Initialize / on member access

	// Scopes maps each parsed file to its file-scope import refs, keyed by
	// local name (alias or basename).
	Scopes map[*syntax.File]map[string]*ImportRef

	initOnce sync.Once
	initErr  error
	// Bootstrap builds and runs the package initializer (var/const decls +
	// init() funcs). Injected by the engine; called exactly once.
	Bootstrap func(*Package) error
}

// EnsureReady advances the package through Initialize to Ready.
func (p *Package) EnsureReady() error {
	p.initOnce.Do(func() {
		if p.State == Ready {
			return
		}
		p.State = Initializing
		if p.Bootstrap != nil {
			if err := p.Bootstrap(p); err != nil {
				p.initErr = err
				p.State = Failed
				return
			}
		}
		p.State = Ready
	})
	return p.initErr
}

// Member returns an exported member of the package: globals first, then the
// index (functions/types are materialized on demand via materialize).
// materialize is engine-provided and builds *Function / *TypeDef objects.
func (p *Package) Member(name string, materialize func(*Package, *index.Decl) (Value, error)) (Value, error) {
	if err := p.EnsureReady(); err != nil {
		return nil, err
	}
	if v, ok := p.Globals.Get(name); ok {
		return v, nil
	}
	if p.Index != nil {
		if d, ok := p.Index.Funcs[name]; ok {
			return materialize(p, d)
		}
		if d, ok := p.Index.Types[name]; ok && d.Decl != nil {
			return materialize(p, d.Decl)
		}
		if d, ok := p.Index.Consts[name]; ok {
			return materialize(p, d)
		}
		if d, ok := p.Index.Vars[name]; ok {
			return materialize(p, d)
		}
	}
	return nil, fmt.Errorf("undefined: %s.%s", p.Name, name)
}
