package main

import "example.com/dsl"

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

func main() {}
