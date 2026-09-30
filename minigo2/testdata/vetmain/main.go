package main

import (
	"example.com/dsl"
	"github.com/podhmo/go-scan/minigo2/testdata/vetstub"
)

func main() {
	dsl.Registered()   // bound host symbol -> fine
	dsl.Unregistered() // bound package, missing symbol -> not a stub finding
	vetstub.Unreg()    // unregistered stub member -> vet finding
	vetstub.Real()     // real declaration -> fine
	_ = vetstub.Value  // not a call -> fine
	_ = dsl.Cfg        // selector without call -> fine
}
