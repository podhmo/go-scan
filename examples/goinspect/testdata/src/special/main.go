package main

import "github.com/podhmo/go-scan/examples/goinspect/testdata/src/special/util"

func main() {
	privateFunc()
	util.UtilFunc()
}

func privateFunc() {
	// this is a private function
}