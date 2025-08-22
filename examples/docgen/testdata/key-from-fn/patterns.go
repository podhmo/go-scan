//go:build minigo
// +build minigo

package main

import (
	"key-from-fn/foo"
)

// PatternConfig is a local stub for the real patterns.PatternConfig.
// We only define the fields we need for this test.
type PatternConfig struct {
	Name     string
	Fn       any
	Type     string // Using string for simplicity in the stub
	ArgIndex int
}

var Patterns = []PatternConfig{
	{
		Name:     "pattern-for-method",
		Fn:       (*foo.Foo)(nil).Bar,
		Type:     "responseBody",
		ArgIndex: 0,
	},
	{
		Name:     "pattern-for-function",
		Fn:       foo.Baz,
		Type:     "responseBody",
		ArgIndex: 0,
	},
}
