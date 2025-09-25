# GoInspect Call-Graph Explorer

`goinspect` is a command-line tool that analyzes Go source code and displays the call graph for functions within a specified package. It uses the `symgo` symbolic execution engine from the `go-scan` library to trace function calls, providing a high-level overview of code relationships.

This tool serves as a practical example of how to build static analysis tools using the `go-scan` and `symgo` libraries.

## Installation and Usage

To use the tool, you can run it directly from the source code.

### Prerequisites
- Go 1.21 or later.

### Running the tool

1.  Navigate to the tool's directory:
    ```sh
    cd examples/goinspect
    ```

2.  Run the tool, specifying the target package pattern with the `--pkg` flag:
    ```sh
    go run . --pkg=./testdata/src/myapp
    ```

## Command-Line Options

-   `--pkg <pattern>`: (Required) The Go package pattern to inspect (e.g., `./...`, `example.com/mymodule/pkg/util`).
-   `--include-unexported`: (Optional) Include unexported functions as analysis entry points. Defaults to `false`.
-   `--short`: (Optional) Use a short format for function signatures in the output, replacing arguments with `(...)`.
-   `--expand`: (Optional) Use an expanded format that assigns a unique ID to each function to handle cycles and repeated calls gracefully.
-   `--log-level <level>`: (Optional) Set the logging level. Can be `debug`, `info`, `warn`, or `error`. Defaults to `info`.

## Example Output

Given the following code in `./testdata/src/myapp`:

```go
package main

import "fmt"

type Person struct {
	Name string
}

func (p *Person) Greet() {
	fmt.Printf("Hello, my name is %s\n", p.Name)
}

func main() {
	p := &Person{Name: "Alice"}
	p.Greet()
}
```

Running `go run . --pkg=./testdata/src/myapp` produces the following output:

```
func (*Person).Greet()
  func fmt.Printf(string, ...any)
func main.main()
  func (*Person).Greet()
    func fmt.Printf(string, ...any)
```

Using the `--short` flag (`go run . --pkg=./testdata/src/myapp --short`):
```
func (*Person).Greet()
  func fmt.Printf(...)
func main.main()
  func (*Person).Greet()
```

## Known Limitations

`goinspect` relies on the `symgo` symbolic execution engine, and its accuracy is subject to the capabilities of `symgo`. Current testing has revealed the following limitations:

-   **Cross-Package Calls**: Calls to functions in packages outside the primary analysis scope are currently not displayed in the call graph. The underlying engine does not yet represent these as terminal nodes.
-   **Higher-Order & Anonymous Functions**: The engine has limited support for analyzing anonymous functions (function literals) passed as arguments. As a result, the tool may not be able to resolve their signatures correctly or trace calls made from within their bodies.

These limitations are tracked in the project's `TODO.md` and will be addressed by enhancing the `symgo` library in the future.