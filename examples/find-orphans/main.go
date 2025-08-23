package main

import (
	"flag"
	"fmt"
	"log"
)

func main() {
	var (
		all          = flag.Bool("all", false, "scan every package in the module")
		includeTests = flag.Bool("include-tests", false, "include usage within test files")
		workspace    = flag.String("workspace-root", "", "scan all Go modules found under a given directory")
		verbose      = flag.Bool("v", false, "enable verbose output")
	)

	flag.Parse()

	// TODO: use the flags
	fmt.Println("all:", *all)
	fmt.Println("includeTests:", *includeTests)
	fmt.Println("workspace:", *workspace)
	fmt.Println("verbose:", *verbose)
	if flag.NArg() > 0 {
		log.Fatalf("Error: positional arguments are not supported, got: %v", flag.Args())
	}
}
