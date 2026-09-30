package main

import (
	inithelper "github.com/podhmo/go-scan/minigo2/testdata/inithelper"
	"github.com/podhmo/go-scan/minigo2/testdata/inittable"
)

func ViaFunc() int { return inittable.Lookup() } // only func access -> init must run
func main()        {}

// Indirect: the init-built table is reached through another package's
// function that itself touches only func members of inittable.
func ViaIndirect() int { return inithelper.Get() }
