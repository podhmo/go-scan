package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var update = flag.Bool("update", false, "update golden files")

func Test(t *testing.T) {
	t.Parallel()

	testscript.Run(t, testscript.Params{
		Dir: "testdata/scripts",
	})
}

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"goinspect": func() int {
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
			flagPkg = flag.String("pkg", "", "package pattern (required)")
			flagIncludeUnexported = flag.Bool("include-unexported", false, "include unexported functions")
			flagShort = flag.Bool("short", false, "short output")
			flagExpand = flag.Bool("expand", false, "expand output")
			flagVerbose = flag.Bool("v", false, "verbose output")

			// The testscript engine will pass flags.
			flag.Parse()

			if *flagPkg == "" {
				flag.Usage()
				return 1
			}

			if err := run(context.Background()); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return 0
		},
	}))
}