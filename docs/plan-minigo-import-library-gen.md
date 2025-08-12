# Plan: Generating Minigo Bindings for Go Libraries

This document outlines a strategy for deeply integrating existing Go libraries with the minigo interpreter. The primary goal is to allow minigo scripts to seamlessly import and use functions and variables from standard or third-party Go packages, such as `strings`, `bytes`, or even more complex ones like `golang.org/x/sync/errgroup`.

We will cover three main topics:
1.  **The Current Manual Binding System**: How to expose Go code to minigo today.
2.  **A Proposal for Automated Generation**: A plan to create a tool that automatically generates the necessary binding code for any given Go package.
3.  **Concurrency Considerations**: An analysis of how Go's concurrency features, particularly goroutines, can be safely and effectively used from within minigo.

## 1. Manual Binding with `Interpreter.Register`

The minigo interpreter already provides a mechanism for exposing Go functions and variables to scripts: the `Interpreter.Register` method. This method allows you to register a collection of symbols under a specific package path.

### How It Works

The `Register` method has the following signature:

```go
func (i *Interpreter) Register(pkgPath string, symbols map[string]any)
```

-   `pkgPath`: The path that will be used in the minigo `import` statement (e.g., `"strings"`).
-   `symbols`: A map where keys are the symbol names (e.g., `"ToUpper"`) and values are the actual Go functions or variables (e.g., `strings.ToUpper`).

When a minigo script executes `import "strings"`, the interpreter makes the registered symbols available. The evaluator's interop layer handles the complex work of:
-   Converting minigo arguments to the required Go types for function calls.
-   Calling the Go function using reflection.
-   Converting the Go function's return values back into minigo objects.

### Example: Manually Binding the `strings` Package

Here is how you would manually expose a few functions from the standard `strings` package to a minigo script:

**Go Code:**

```go
import (
    "context"
    "fmt"
    "log"
    "strings"

    "github.com/podhmo/go-scan/minigo"
)

func main() {
    // 1. Create a new interpreter instance.
    interp, err := minigo.NewInterpreter()
    if err != nil {
        log.Fatalf("Failed to create interpreter: %v", err)
    }

    // 2. Register the desired functions from the "strings" package.
    interp.Register("strings", map[string]any{
        "ToUpper":   strings.ToUpper,
        "ToLower":   strings.ToLower,
        "HasPrefix": strings.HasPrefix,
        "TrimSpace": strings.TrimSpace,
    })

    // 3. The minigo script to be executed.
    script := `
package main

import "strings"

var greeting = "  Hello, World!  "
var upper_greeting = strings.ToUpper(greeting)
var trimmed_greeting = strings.TrimSpace(greeting)
var has_prefix = strings.HasPrefix(trimmed_greeting, "Hello")
`

    // 4. Load and evaluate the script.
    if err := interp.LoadFile("myscript.mgo", []byte(script)); err != nil {
        log.Fatalf("Failed to load script: %v", err)
    }
    if _, err := interp.Eval(context.Background()); err != nil {
        log.Fatalf("Failed to evaluate script: %v", err)
    }

    // 5. Inspect the results.
    env := interp.GlobalEnvForTest()
    fmt.Println(env.Get("upper_greeting"))   // "  HELLO, WORLD!  "
    fmt.Println(env.Get("trimmed_greeting")) // "Hello, World!"
    fmt.Println(env.Get("has_prefix"))       // true
}
```

This approach is powerful and works well for exposing a small, curated set of functions. However, it is not scalable. Manually binding every function in a large package like `strings` would be tedious and error-prone. This limitation leads us to the need for an automated solution.

## 2. Proposal: Automated Binding Generation

To solve the scalability problem, we propose the creation of a command-line tool, tentatively named `minigo-gen-bindings`. This tool will inspect a Go package and automatically generate the Go code required to bind all of its exported functions and variables to the minigo interpreter.

### Tool Workflow

The `minigo-gen-bindings` tool would operate as follows:

1.  **Input**: The tool would accept a Go package import path as a command-line argument.
    ```sh
    go run ./tools/minigo-gen-bindings --pkg "strings" --output "minigo_bindings_strings.go"
    ```

2.  **Package Location**: It will use the `go/build` package to locate the source code for the target package. `build.Import("strings", "", build.FindOnly)` would provide the directory path of the "strings" package.

3.  **AST Parsing**: The tool will then parse all the `.go` files in that directory using `go/parser`. This creates an Abstract Syntax Tree (AST) for each file, which represents the source code in a structured way. This approach respects the project constraint of not using the `go/types` package.

4.  **Exported Symbol Discovery**: It will walk the AST of each file to find all exported top-level declarations:
    -   **Functions**: It will identify all `ast.FuncDecl` nodes whose `Name` is exported (starts with an uppercase letter).
    -   **Variables & Constants**: It will identify all `ast.ValueSpec` nodes within a `ast.GenDecl` and check for exported `Names`.

5.  **Code Generation**: Finally, the tool will use the collected symbol names to generate a new Go source file. This file will contain a function that performs the registration. The generation will likely use the `text/template` package for robustness.

### Example: Generated Output for `strings`

Running the tool for the `strings` package would produce a file like `minigo_bindings_strings.go` with the following content:

```go
// Code generated by minigo-gen-bindings. DO NOT EDIT.

package bindings

import (
	"strings"
	"github.com/podhmo/go-scan/minigo"
)

// RegisterStrings binds all exported symbols from the "strings" package to the interpreter.
func RegisterStrings(interp *minigo.Interpreter) {
	interp.Register("strings", map[string]any{
		"Clone":         strings.Clone,
		"Compare":       strings.Compare,
		"Contains":      strings.Contains,
		"ContainsAny":   strings.ContainsAny,
		"ContainsRune":  strings.ContainsRune,
		"Count":         strings.Count,
		"Cut":           strings.Cut,
		"CutPrefix":     strings.CutPrefix,
		"CutSuffix":     strings.CutSuffix,
		"EqualFold":     strings.EqualFold,
		"Fields":        strings.Fields,
		"FieldsFunc":    strings.FieldsFunc,
		"HasPrefix":     strings.HasPrefix,
		"HasSuffix":     strings.HasSuffix,
		"Index":         strings.Index,
		"IndexAny":      strings.IndexAny,
		"IndexByte":     strings.IndexByte,
		"IndexFunc":     strings.IndexFunc,
		"IndexRune":     strings.IndexRune,
		"Join":          strings.Join,
		"LastIndex":     strings.LastIndex,
		"LastIndexAny":  strings.LastIndexAny,
		"LastIndexByte": strings.LastIndexByte,
		"LastIndexFunc": strings.LastIndexFunc,
		"Map":           strings.Map,
		"Repeat":        strings.Repeat,
		"Replace":       strings.Replace,
		"ReplaceAll":    strings.ReplaceAll,
		"Split":         strings.Split,
		"SplitAfter":    strings.SplitAfter,
		"SplitAfterN":   strings.SplitAfterN,
		"SplitN":        strings.SplitN,
		"ToLower":       strings.ToLower,
		"ToLowerSpecial": strings.ToLowerSpecial,
		"ToTitle":       strings.ToTitle,
		"ToTitleSpecial": strings.ToTitleSpecial,
		"ToUpper":       strings.ToUpper,
		"ToUpperSpecial": strings.ToUpperSpecial,
		"ToValidUTF8":   strings.ToValidUTF8,
		"Trim":          strings.Trim,
		"TrimFunc":      strings.TrimFunc,
		"TrimLeft":      strings.TrimLeft,
		"TrimLeftFunc":  strings.TrimLeftFunc,
		"TrimPrefix":    strings.TrimPrefix,
		"TrimRight":     strings.TrimRight,
		"TrimRightFunc": strings.TrimRightFunc,
		"TrimSpace":     strings.TrimSpace,
		"TrimSuffix":    strings.TrimSuffix,
		// Note: Exported variables and constants would also be included here.
	})
}
```

The user can then simply call `bindings.RegisterStrings(interp)` in their Go application to make the entire `strings` package available to minigo scripts. This approach is robust, automated, and easily extendable to any Go package.

## 3. Concurrency and Goroutines

A key question is whether minigo can interact with Go's powerful concurrency features. For example, can it use packages like `sync` or `golang.org/x/sync/errgroup`? The answer is yes, with some important clarifications.

### The Execution Model

From the perspective of a minigo script, execution is **synchronous and single-threaded**. A minigo script does not have its own `go` keyword or a way to manage goroutines directly. It executes statements one after another.

When a minigo script calls a Go function (that has been bound via `Register`), the interpreter blocks and waits for the Go function to return. This is the same behavior as a standard function call in Go.

### Using Goroutines Within a Bound Go Function

The crucial point is that the bound Go function is a normal Go function. It can do anything a Go function can do, including creating and managing goroutines. The minigo interpreter is unaware of this internal concurrency; it only sees a single function call that takes some time to complete.

This means that packages like `errgroup` are perfectly safe and effective to use.

### Example: A Concurrent Fetcher

Imagine we want to fetch multiple web pages concurrently and return their titles. We could write a Go function that uses an `errgroup` for this purpose and expose it to minigo.

**Go Code:**

```go
import (
    "context"
    "io"
    "net/http"
    "strings"

    "golang.org/x/sync/errgroup"
    "github.com/podhmo/go-scan/minigo"
)

// fetchTitle is a helper that fetches a single URL and extracts its title.
func fetchTitle(ctx context.Context, url string) (string, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return "", err
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024)) // Limit read size
    if err != nil {
        return "", err
    }
    // Simple/naive title extraction
    s := string(body)
    start := strings.Index(s, "<title>")
    if start == -1 {
        return "No title found", nil
    }
    start += len("<title>")
    end := strings.Index(s[start:], "</title>")
    if end == -1 {
        return "No title found", nil
    }
    return s[start : start+end], nil
}

// FetchAllTitles uses an errgroup to fetch titles concurrently.
// This is the function we will expose to minigo.
func FetchAllTitles(urls []string) ([]string, error) {
    g, ctx := errgroup.WithContext(context.Background())
    titles := make([]string, len(urls))

    for i, url := range urls {
        i, url := i, url // https://golang.org/doc/faq#closures_and_goroutines
        g.Go(func() error {
            title, err := fetchTitle(ctx, url)
            if err == nil {
                titles[i] = title
            }
            return err
        })
    }

    if err := g.Wait(); err != nil {
        return nil, err
    }
    return titles, nil
}

// Now, let's register it and call it from minigo.
func run() {
    interp, _ := minigo.NewInterpreter()
    interp.Register("web", map[string]any{
        "FetchAllTitles": FetchAllTitles,
    })

    script := `
package main
import "web"

// Note: minigo doesn't have a native string slice literal,
// so we'd need to pass it in or have another way to construct it.
// For this example, assume we can construct the slice.
var urls = ["https://golang.org", "https://github.com"]
var titles, err = web.FetchAllTitles(urls)
`
    // ... load and eval script ...
}
```

In this example, the minigo script calls `web.FetchAllTitles`. The interpreter freezes while the Go function executes. Internally, `FetchAllTitles` fans out multiple goroutines. When all goroutines are finished, `g.Wait()` returns, `FetchAllTitles` returns its result, and the minigo interpreter continues, populating the `titles` and `err` variables.

The concurrency is entirely encapsulated within the Go function, which is a clean and safe way to leverage Go's power from a simple scripting environment. The same logic applies to using lower-level primitives from the `sync` package like `sync.Mutex` or `sync.WaitGroup`. They will work as expected within the Go function's context.

## 4. Conclusion

The minigo interpreter provides a solid foundation for Go interoperability via the `Interpreter.Register` method. While manual binding is effective for small-scale integrations, it does not scale to entire libraries.

The proposed `minigo-gen-bindings` tool addresses this challenge directly. By automatically scanning a Go package and generating the necessary binding code, it offers a path to seamlessly integrate any Go library with minigo. This approach provides:

-   **Automation**: Eliminates the tedious and error-prone task of writing manual bindings.
-   **Completeness**: Ensures that all exported functions and variables of a package are made available.
-   **Safety**: The generated code is type-safe at the Go level, while the interop layer already handles the dynamic nature of the scripting environment.
-   **Power**: Unlocks the full potential of the Go ecosystem for minigo scripts, including powerful concurrency patterns.

By implementing this generator, minigo can evolve into a more powerful and versatile scripting environment, deeply integrated with Go.
