// Command minigo runs a function in a package directory.
//
//	minigo run ./path/to/pkg [FuncName]
//	minigo eval ./path/to/pkg        # prints nothing; just initializes+runs main
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/podhmo/go-scan/minigo2"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: minigo <dir-or-importpath> [func]")
		os.Exit(1)
	}
	ref := args[0]
	fn := "main"
	if len(args) > 1 {
		fn = args[1]
	}
	cwd, err := os.Getwd()
	if err != nil {
		slog.ErrorContext(ctx, "getwd", "error", err)
		os.Exit(1)
	}
	e := minigo2.NewEngine(cwd)
	r, err := e.Run(ctx, ref, fn)
	if err != nil {
		slog.ErrorContext(ctx, "run", "error", err)
		os.Exit(1)
	}
	if r != nil {
		fmt.Printf("%v\n", r)
	}
}
