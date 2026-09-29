package main

import (
	. "github.com/podhmo/go-scan/minigo2/testdata/dotextra"
	. "github.com/podhmo/go-scan/minigo2/testdata/greet"
	. "github.com/podhmo/go-scan/minigo2/testdata/lazyboom"
)

func Greeting() string { return Hello("x") }

func Number() int { return Value() + Count }

func TouchBoom() int { return Get() }
