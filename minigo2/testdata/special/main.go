package main

import (
	"example.com/dsl"
	greet "github.com/podhmo/go-scan/minigo2/testdata/greet"
	"github.com/podhmo/go-scan/minigo2/testdata/lazyboom"
)

// dsl is a host-bound package; dsl.Twice/Show/Skipped are registered
// special forms — the calls compile to OpSpecialCall with quoted args.

func TwiceIt() int {
	x := 21
	return dsl.Twice(x + 1) // handler evals (x+1)=22 -> 44
}

func Quoted() string {
	y := 9
	return dsl.Show(y + 1) // handler returns source text, never evaluates
}

func Lazy() int {
	boom := func() int { panic("boom") }
	// args stay quoted: the handler skips Eval, so boom never runs
	return dsl.Skipped(boom()) // 42
}

// ResolveSymbol maps quoted exprs to canonical SymbolIDs through the
// file's import table — without materializing the target package, so
// lazyboom's panicking init must NOT run here.
func SymPkg() string   { return dsl.SymOf(lazyboom.Get) }
func SymGreet() string { return dsl.SymOf(greet.Hello) }
func SymSelf() string  { return dsl.SymOf(TwiceIt) }

// SymShadow: a local variable shadowing the import alias must not resolve
// as the imported package's symbol.
func SymShadow() string {
	greet := 1
	_ = greet
	return dsl.SymOf(greet.Hello)
}

func SymLocal() string {
	boom := func() int { panic("boom") }
	return dsl.SymOf(boom) // local var: resolution must fail
}

func main() {}
