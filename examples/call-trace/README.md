# call-trace

`call-trace` is a command-line tool to identify which command-line entry points (`main` functions) result in a call to a specified target function.

## Usage

```shell
go run ./examples/call-trace -target <target_function> [package_patterns...]
```

- `-target`: The target function to trace calls to.
  - For functions: `path/to/pkg.FuncName`
  - For methods: `(*path/to/pkg.TypeName).MethodName`
- `package_patterns...`: Go package patterns to analyze (e.g., `./...`). Defaults to `./...`.

## Example

Given the following code structure:
```
- myapp/
  - main.go
- mylib/
  - lib.go
```

`myapp/main.go`:
```go
package main
import "github.com/user/repo/mylib"
func main() { mylib.Helper() }
```

`mylib/lib.go`:
```go
package mylib
func Helper() {}
```

To find which main function calls `mylib.Helper`:
```shell
go run ./examples/call-trace -target github.com/user/repo/mylib.Helper ./...
```

### Example Output

```
Found 1 call stacks to github.com/user/repo/mylib.Helper:

--- Stack 1 ---
	:0:0:	in main
```
