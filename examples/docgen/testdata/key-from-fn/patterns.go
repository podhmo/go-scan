//go:build minigo
// +build minigo

package main

import (
	"key-from-fn/foo"
)

// PatternConfig is a local stub for the real patterns.PatternConfig.
type PatternConfig struct {
	Name     string
	Fn       any
	Type     string
	ArgIndex int
}

var (
	// variables for testing instance methods
	v = foo.Foo{}
	p = &foo.Foo{}
	n = new(foo.Foo)
)

var Patterns = []PatternConfig{
	// Method from typed nil
	{
		Name: "pattern-for-method-from-nil",
		Fn:   (*foo.Foo)(nil).Bar,
		Type: "responseBody",
	},
	// Standalone function
	{
		Name: "pattern-for-function",
		Fn:   foo.Baz,
		Type: "responseBody",
	},
	// Method from value instance
	{
		Name: "pattern-for-method-from-value",
		Fn:   v.Qux,
		Type: "responseBody",
	},
	// Method from pointer to struct literal instance
	{
		Name: "pattern-for-method-from-pointer-literal",
		Fn:   p.Bar,
		Type: "responseBody",
	},
	// Method from new() instance
	{
		Name: "pattern-for-method-from-new",
		Fn:   n.Bar,
		Type: "responseBody",
	},
}
