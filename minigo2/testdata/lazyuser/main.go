package main

import "github.com/podhmo/go-scan/minigo2/testdata/lazyboom"

// OK never references lazyboom: importing it must stay inert.
func OK() int { return 1 }

// Bad touches lazyboom.Get, which triggers its (panicking) init.
func Bad() int { return lazyboom.Get() }

func main() {}
