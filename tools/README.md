# tools

## Availables

- [goinspect](#goinspect)


## goinspect

The `goinspect` example is a command-line tool that analyzes Go source code and displays the call graph for specified functions.

**Purpose**: To provide a high-level overview of call relationships for documentation and code understanding. It uses the `symgo` symbolic execution engine to trace execution paths and build a precise call graph.

**Key Features**:
- Uses `symgo` to trace function calls, including methods and calls on interfaces.
- **Default (Condensed) View**: By default, it prints a condensed call graph. Each function's call tree is shown only once. Subsequent calls to the same function are referenced by a unique, position-based ID (e.g., `#1`), preventing duplicate output.
- **Expanded View (`--expand`)**: Shows the full, unabridged call graph, expanding every call site. This is useful for seeing the complete call structure without summarization.
- **Short Format (`--short`)**: Abbreviates function signatures for a more compact view (e.g., `(...)`).
- **Accessor Detection**: Identifies and flags simple getter/setter methods with an `[accessor]` marker for clarity.
- Scopes analysis to specified packages (`--pkg`) and can include unexported functions (`--include-unexported`).

### Usage

```bash
go run ./tools/goinspect --pkg <package-pattern> [flags]
```

### Example: Default (Condensed) Output

Running `goinspect` on the `features` test package shows each function's call tree once. Note how `(*Data).ComplexLogic()` is referenced by its ID `#1` the second time it appears.

```
$ go run ./tools/goinspect --pkg ./tools/goinspect/testdata/src/features
func (*Data).ComplexLogic() #1
  [accessor] func (*Data).GetID() #2
func github.com/podhmo/go-scan/tools/goinspect/testdata/src/features.Main() #3
  [accessor] func (*Data).SetName(string) #4
  func (*Data).ComplexLogic() #1
  func github.com/podhmo/go-scan/tools/goinspect/testdata/src/features.Execute(unhandled_type_*ast.FuncType) #5
```

### Example: Expanded Output

Using the `--expand` flag on multiple packages (`./...`) prints the complete call graph without IDs, expanding every call.

```
$ go run ./tools/goinspect --pkg ./tools/goinspect/testdata/src/... --expand
func (*Data).ComplexLogic()
  [accessor] func (*Data).GetID()
  func github.com/podhmo/go-scan/tools/goinspect/testdata/src/another.Helper()
func github.com/podhmo/go-scan/tools/goinspect/testdata/src/features.Main()
  [accessor] func (*Data).SetName(string)
  func (*Data).ComplexLogic()
    [accessor] func (*Data).GetID()
    func github.com/podhmo/go-scan/tools/goinspect/testdata/src/another.Helper()
...
func github.com/podhmo/go-scan/tools/goinspect/testdata/src/myapp.Recursive(int)
  func github.com/podhmo/go-scan/tools/goinspect/testdata/src/myapp.Recursive(int) ... (cycle detected)
```

