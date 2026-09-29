// Command minigo runs a function in a package directory, or an
// interactive session.
//
//	minigo run ./path/to/pkg [--entry FuncName]
//	minigo ./path/to/pkg [FuncName]   # shorthand for run
//	minigo repl                       # interactive session
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/podhmo/go-scan/minigo2"
)

func main() {
	ctx := context.Background()
	args := os.Args[1:]
	if len(args) < 1 {
		usage()
	}
	var err error
	switch args[0] {
	case "repl":
		err = runREPL(ctx, os.Stdin, os.Stdout)
	case "run":
		err = run(ctx, args[1:])
	default:
		// shorthand: `minigo <ref> [func]`
		err = run(ctx, args)
	}
	if err != nil {
		slog.ErrorContext(ctx, "minigo", "error", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  minigo run <dir-or-importpath> [--entry Func]
  minigo <dir-or-importpath> [Func]
  minigo repl`)
	os.Exit(1)
}

func run(ctx context.Context, args []string) error {
	// extract -entry/--entry anywhere: Go's flag package stops at the
	// first positional, but `minigo run ./pkg --entry F` should work
	var entry string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--entry" || a == "-entry":
			i++
			if i >= len(args) {
				return fmt.Errorf("--entry requires a function name")
			}
			entry = args[i]
		case strings.HasPrefix(a, "--entry="), strings.HasPrefix(a, "-entry="):
			entry = strings.SplitN(a, "=", 2)[1]
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %q (supported: --entry)", a)
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) < 1 {
		usage()
	}
	ref := rest[0]
	fn := entry
	if fn == "" && len(rest) > 1 {
		fn = rest[1]
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	e := minigo2.NewEngine(cwd, minigo2.WithOutput(os.Stdout))
	r, err := e.Run(ctx, ref, fn)
	if err != nil {
		return err
	}
	if r != nil {
		fmt.Printf("%v\n", r)
	}
	return nil
}

const replHelp = `commands:
  :help   show this help
  :reset  clear all definitions and values
  :exit   quit (also :quit, :q, Ctrl-D)
input is a top-level declaration or statements; a trailing
expression is printed. new names introduced by := / var / const
persist as globals across lines. a line ending inside an open
() [] {} group (or after an operator) continues with a ".. "
prompt until it closes.`

func runREPL(ctx context.Context, in io.Reader, out io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	e := minigo2.NewEngine(cwd, minigo2.WithOutput(out))
	r := e.NewREPL()
	fmt.Fprintln(out, "minigo2 repl (:help for commands)")
	sc := bufio.NewScanner(in)
	var frag strings.Builder
	for {
		if frag.Len() == 0 {
			fmt.Fprint(out, ">> ")
		} else {
			fmt.Fprint(out, ".. ")
		}
		if !sc.Scan() {
			if frag.Len() > 0 {
				// EOF mid-fragment: surface the parse error rather
				// than dropping the input silently.
				if _, err := r.EvalLine(ctx, frag.String()); err != nil {
					fmt.Fprintf(out, "error: %s\n", err)
				}
			}
			fmt.Fprintln(out)
			return sc.Err()
		}
		text := sc.Text()
		if frag.Len() == 0 {
			line := strings.TrimSpace(text)
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, ":") {
				switch line {
				case ":exit", ":quit", ":q":
					return nil
				case ":reset":
					r.Reset()
					fmt.Fprintln(out, "state cleared")
				case ":help":
					fmt.Fprintln(out, replHelp)
				default:
					fmt.Fprintf(out, "unknown command %q (see :help)\n", line)
				}
				continue
			}
		}
		if frag.Len() > 0 {
			frag.WriteByte('\n')
		}
		frag.WriteString(text)
		src := frag.String()
		if minigo2.IncompleteInput(src) {
			continue
		}
		frag.Reset()
		v, err := r.EvalLine(ctx, src)
		if err != nil {
			fmt.Fprintf(out, "error: %s\n", err)
			continue
		}
		if d := r.Display(v); d != nil {
			fmt.Fprintf(out, "%v\n", d)
		}
	}
}
