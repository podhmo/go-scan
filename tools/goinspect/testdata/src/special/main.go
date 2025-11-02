package main

import "github.com/podhmo/go-scan/tools/goinspect/testdata/src/special/util"

func main() {
	privateFunc()
	util.UtilFunc()
}

func privateFunc() {
	// this is a private function
}
