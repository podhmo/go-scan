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
- [ ] Data Structures
  - [ ] Arrays (e.g., `var a [3]int`, `a[0] = 1`)
  - [ ] Slices (dynamic arrays)
  - [ ] Maps (hash maps)
- [ ] Control Flow
  - [x] `for` loops (various forms: `for {}`, `for i < N {}`, `for i := 0; i < N; i++ {}`. Range-based loop not yet supported)
  - [x] `break` and `continue` statements (unlabeled)
- [ ] Types
  - [ ] More specific integer types (int8, int32, int64, uint etc.)
  - [ ] Floating point numbers
  - [ ] Structs
  - [ ] Type declarations (`type MyInt int`)
  - [ ] Type checking (currently very loose, more like a dynamic language)
- [ ] Pointers
- [ ] Multiple return values from functions
- [ ] Variadic functions (for user-defined functions, built-ins have a simple form)

## Standard Library / Built-ins
- [x] `fmt.Sprintf` (basic implementation)
- [x] `strings.Join` (custom varargs implementation for now)
- [x] `Null` object and implicit returns for `null`
- [ ] More `fmt` functions (e.g., `Println`)
- [ ] More `strings` functions
- [ ] Basic I/O (e.g., reading files, printing to console beyond Sprintf)
- [ ] `len()` function for strings, arrays, slices, maps
- [ ] `panic` and `recover`

## Interpreter Internals & Tooling
- [ ] REPL (Read-Eval-Print Loop) mode
- [x] More robust error object type (`Error` object implemented)
- [ ] Performance optimizations
- [ ] Test suite expansion (more comprehensive tests for all features)
- [ ] Support for multiple files / packages
- [ ] Debugger support (very long term)
- [ ] Better type system for `Object` interface (e.g. using generics if Go 2, or more specific interfaces)

## Code Quality & Refactoring
- [x] Review error handling consistency (e.g. when to return `Error` object vs. `error`) - Improved with `Error` object.
- [x] Refactor `eval` function if it becomes too large (e.g. by node type) - *ongoing, new eval functions added for new node types*.
- [ ] Improve comments and documentation within the code
- [ ] `findFunction` in `interpreter.go` might be dead code now.

## go-scan Integration
- [ ] **Integrate `go-scan` for initial parsing phase**
  - [ ] Modify `LoadAndRun` to use `go-scan`'s `Scanner` to get `PackageInfo`.
  - [ ] Adapt `minigo` to extract function definitions (`*ast.FuncDecl` or body) from `PackageInfo` (or `*ast.File` if provided by `go-scan`).
  - [ ] Adapt `minigo` to extract global variable declarations from `PackageInfo` (or `*ast.File` if provided by `go-scan`).
  - [ ] Ensure error reporting (`formatErrorWithContext`) correctly uses `FileSet` and positional info from `go-scan`.
- [ ] **Contribute to or discuss `go-scan` enhancements**
  - [ ] Propose/discuss `PackageInfo` retaining `*ast.File` for scanned files.
  - [ ] Propose/discuss `FunctionInfo` directly referencing `*ast.FuncDecl`.
  - [ ] Propose/discuss `PackageInfo` aggregating global variable declarations.
- [ ] **Investigate `go-scan`'s Lazy Import (`PackageResolver`) for multi-file/package support in `minigo` (Future)**
