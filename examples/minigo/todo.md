# MiniGo Interpreter TODO List

## Core Language Features
- [x] Basic integer literals and arithmetic (+, -, *, /)
- [x] String literals and concatenation (+)
- [x] Variable declarations (`var x = ...`, `var x string`) and assignments (`x = ...`)
- [x] Global and local scopes (basic implementation exists, closure support added)
- [x] Boolean literals (`true`, `false`) and operators (==, !=, <, >, <=, >= for integers/strings; ==, != for booleans)
- [x] `if`/`else` control flow statements
- [x] Unary operators (`-` for negation, `!` for logical NOT)
- [x] Parenthesized expressions
- [x] Function calls (built-in and user-defined)
- [x] Comments (handled by go/parser)
- [x] Basic error handling with file/line context

## Advanced Language Features
- [x] User-defined functions (`func foo() { ... }`)
  - [x] Parameters and arguments
  - [x] Return statements (`return x`)
  - [x] Closures (lexical scoping for functions)
  - [x] Recursive function calls
- [x] Data Structures
  - [x] Arrays (e.g., `var a [3]int`, `a[0] = 1`)
  - [x] Slices (dynamic arrays, includes `append()`)
  - [x] Maps (hash maps)
- [x] Control Flow
  - [x] `for` loops (various forms: `for {}`, `for i < N {}`, `for i := 0; i < N; i++ {}`, `for k, v := range collection {}`)
  - [x] `break` and `continue` statements (unlabeled)
- [ ] Types
  - [ ] More specific integer types (int8, int32, int64, uint etc.)
  - [ ] Floating point numbers
  - [ ] Structs
  - [ ] Type declarations (`type MyInt int`)
  - [ ] Enhance type checking using information from go-scan
    - [ ] Basic static type checking for variable assignments based on go-scan TypeInfo.
    - [ ] Type checking for function call arguments against parameter types from go-scan FunctionInfo.
    - [ ] Type checking for binary operator operands.
- [ ] Pointers
- [ ] Multiple return values from functions
- [ ] Variadic functions (for user-defined functions, built-ins have a simple form)
- [ ] Imports & Package Handling
  - [x] Basic import statement parsing (`import "path"` and `import alias "path"`) (Unsupported forms like . and _ are rejected)
  - [x] Loading imported package constants (`pkg.MyConst`) via `go-scan`. (As described in README)
  - [/] Loading imported package functions (`pkg.MyFunc`) (README mentions only constants, but basic registration exists)
    - [x] Registration of imported functions as `UserDefinedFunction` objects in `evalSelectorExpr`.
    - [ ] Ensure correct mapping of parameters and body from go-scan's `FunctionInfo` for imported functions.
    - [ ] Verify calling convention and environment setup for imported functions.
    - [ ] Add comprehensive tests for calling functions from imported packages.
  - [ ] Support for imported global variables (e.g., `pkg.MyVar`) (Mentioned as future in README)
    - [ ] Extract imported global variable information using `go-scan`.
    - [ ] Make imported global variables accessible in the interpreter.
  - [ ] Investigate `go-scan`'s Lazy Import (`PackageResolver`) for multi-file/package support in `minigo` (Future - see go-scan Integration section)
  - [ ] (Low Priority) Consider support for dot imports (e.g., `import . "mypkg"`)
  - [ ] (Low Priority) Consider support for blank imports (e.g., `import _ "mypkg"` for side-effects if init functions were supported)

## Standard Library / Built-ins
- [x] `fmt.Sprintf` (basic implementation)
- [x] `strings.Join` (custom varargs implementation for now)
- [x] `Null` object and implicit returns for `null`
- [x] `len()` function for strings, arrays, slices, maps
- [x] `append()` function for slices
- [x] More `fmt` functions (e.g., `Println`)
- [ ] More `strings` functions
- [ ] Basic I/O (e.g., reading files, printing to console beyond Sprintf)
- [ ] `panic` and `recover`

## Interpreter Internals & Tooling
- [ ] REPL (Read-Eval-Print Loop) mode
- [x] More robust error object type (`Error` object implemented)
- [ ] Performance optimizations
- [ ] Test suite expansion (more comprehensive tests for all features)
- [ ] Support for multiple files / packages (related to Lazy Import above)
- [ ] Debugger support (very long term)
- [ ] Better type system for `Object` interface (e.g. using generics if Go 2, or more specific interfaces)

## Code Quality & Refactoring
- [x] Review error handling consistency (e.g. when to return `Error` object vs. `error`) - Improved with `Error` object.
- [x] Refactor `eval` function if it becomes too large (e.g. by node type) - *ongoing, new eval functions added for new node types*.
- [ ] Improve comments and documentation within the code
- [ ] Confirm and remove `findFunction` in `interpreter.go` as it appears to be dead code after go-scan integration.

## go-scan Integration
- [x] **Integrate `go-scan` for initial parsing phase**
  - [x] Modify `LoadAndRun` to use `go-scan`'s `Scanner` to get `PackageInfo`.
  - [x] Adapt `minigo` to extract function definitions (`*ast.FuncDecl` or body) from `PackageInfo` (or `*ast.File` if provided by `go-scan`).
  - [x] Adapt `minigo` to extract global variable declarations from `PackageInfo` (using `*ast.File` from `pkgInfo.AstFiles` currently).
  - [x] Ensure error reporting (`formatErrorWithContext`) correctly uses `FileSet` and positional info from `go-scan`.
- [ ] **Contribute to or discuss `go-scan` enhancements**
  - [x] Propose/discuss `PackageInfo` retaining `*ast.File` for scanned files. (Implemented)
  - [x] Propose/discuss `FunctionInfo` directly referencing `*ast.FuncDecl`. (Implemented)
  - [ ] Propose/discuss `PackageInfo` aggregating global variable declarations.
    - [ ] (If go-scan supports it) Adapt `minigo` to use aggregated global variable information from `PackageInfo` directly.
  - [ ] Propose/discuss `PackageInfo` aggregating global variable type and value information for easier import.
- [ ] **Investigate `go-scan`'s Lazy Import (`PackageResolver`) for multi-file/package support in `minigo` (Future)**
  - This relates to "Imports & Package Handling" in "Advanced Language Features" and "Support for multiple files / packages" in "Interpreter Internals & Tooling".

*Legend for progress markers:*
- `[x]` Completed
- `[/]` Partially Completed
- `[ ]` Not Started / Incomplete
