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
  - [x] Structs
    - [x] Basic struct type declaration (`type Point struct { X int }`)
    - [x] Struct literal instantiation (`Point{X: 10, Y: 20}`)
    - [x] Field access (`p.X`)
    - [x] Basic embedded structs (promoted fields, initialization of embedded struct by type name)
    - [x] Field assignment (`p.X = 100`)
    - [ ] Unkeyed struct literals (e.g. `Point{10,20}`)
    - [ ] Type checking for struct field assignments and initializers.
    - [ ] Zero value for struct types (uninitialized fields are NULL, explicit zero struct via `T{}`)
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
  - [x] Loading imported package constants (`pkg.MyConst`) via `go-scan`.
  - [x] Loading imported package functions (`pkg.MyFunc`)
  - [ ] Support for imported global variables (e.g., `pkg.MyVar`) (Mentioned as future in README)
    - [ ] Extract imported global variable information using `go-scan`.
    - [ ] Make imported global variables accessible in the interpreter.
  - [ ] Investigate `go-scan`'s Lazy Import (`PackageResolver`) for multi-file/package support in `minigo` (Future - see go-scan Integration section)
  - [x] Update README.md to reflect current import capabilities (constants and functions).
  - [ ] Clarify and document behavior for imports when `go.mod` is not found or when script is outside a module (relates to `trouble.md`).
  - [ ] Consider strategy for true Go standard library imports (e.g., `import "os"`) beyond current built-in shims (relates to `trouble.md`).
  - [ ] Define and implement `const` declarations in minigo scripts (currently only `var`).
  - [ ] Support for built-in global variables (e.g., `iota`, `nil` if not already fully supported).

## Standard Library / Built-ins
- [x] `fmt.Sprintf` (basic implementation)
- [x] `strings.Join` (refactored to accept slice of strings and separator)
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
  - [ ] Test type checking during struct instantiation (e.g., `Point{X: "not-an-int"}`).
  - [ ] Test non-keyed struct literals (e.g., `Point{10, 20}`).
  - [x] Test modifying struct fields (e.g., `p.X = 30`).
  - [ ] Test returning struct from main and checking its value directly from LoadAndRun's result.
  - [ ] Test struct definition within a function (local type declarations).
  - [ ] Test for duplicate field names in struct literal (e.g. `Point{X:1, X:2}`).
  - [ ] Test using a non-struct type in a composite literal (e.g. `var x int; _ = x{}`).
  - [ ] Test empty struct literal for non-empty struct (e.g. `type P struct {X int}; _ = P{}`).
  - [ ] Test for `nil` keyword/object and its behavior, especially with map access returning `NULL`.
- [ ] Support for multiple files / packages (related to Lazy Import above)
- [ ] Debugger support (very long term)
- [ ] Better type system for `Object` interface (e.g. using generics if Go 2, or more specific interfaces)
  - [x] Implement `Hashable` interface for all relevant object types that can be map keys.
  - [ ] Implement `Callable` interface for function objects (UserDefinedFunction, BuiltinFunction).
- [ ] **Built-in Auto-generation (`autogen-builtin.md`) Review**
  - [x] Update `autogen-builtin.md` to reflect that MiniGo now possesses `object.Array` and `object.Slice` types, making its statement about "MiniGo lacks array types" outdated.
  - [x] Refactored `strings.Join` to accept a MiniGo slice/array and a separator, aligning with auto-generation ideals and MiniGo's current type capabilities.
- [ ] **Reflection / Introspection Features (`minigo/inspect` package - based on improvement.md)**
  - [ ] Implement `minigo/inspect` package as a built-in or standard library package.
  - [ ] Design and implement `ImportedFunction` object type in `object.go` to hold info about imported Go functions (distinct from callable `UserDefinedFunction`).
  - [ ] Modify `evalSelectorExpr` to create `ImportedFunction` objects for imported functions.
  - [ ] Modify `evalCallExpr` to disallow direct calls to `ImportedFunction` objects.
  - [ ] Implement `inspect.GetFunctionInfo(fnSymbol)`:
    - [ ] Takes an `ImportedFunction` object (or perhaps a callable `UserDefinedFunction` representing an import).
    - [ ] Uses `go-scan` data to populate `FunctionInfo` struct (Go side).
    - [ ] Converts `FunctionInfo` to a minigo map or struct-like object.
  - [ ] Implement `inspect.GetTypeInfo(typeName string)`:
    - [ ] Uses `go-scan` to get details for the type.
    - [ ] Populates `TypeInfo` struct (Go side) with kinds, fields, methods, etc.
    - [ ] Converts `TypeInfo` to a minigo map or struct-like object.
    - [ ] Handle recursive types and potential cycles.
  - [ ] Implement `inspect.GetPackageInfo(pkgPathOrSymbol)`:
    - [ ] Uses `go-scan` to get overall package details.
    - [ ] Populates `PackageInfo` struct (Go side) with imports, constants, variables, function names, type names.
    - [ ] Converts `PackageInfo` to a minigo map or struct-like object.
  - [ ] Define Go structs for `FunctionInfo`, `ParamInfo`, `ReturnInfo`, `TypeInfo`, `FieldInfo`, `MethodInfo`, `PackageInfo`, `ValueInfo` as proposed in `improvement.md`.
  - [ ] Ensure proper error handling for all `inspect` functions (symbol not found, type mismatch, etc.).
  - [ ] Add comprehensive tests for the `minigo/inspect` package functionality.

## Code Quality & Refactoring
- [x] Review error handling consistency (e.g. when to return `Error` object vs. `error`) - Improved with `Error` object.
- [x] Refactor `eval` function if it becomes too large (e.g. by node type) - *ongoing, new eval functions added for new node types*.
- [ ] Improve comments and documentation within the code
- [x] Confirm and remove `findFunction` in `interpreter.go` as it appears to be dead code after go-scan integration. (Confirmed as unused and effectively dead code due to current direct AST processing for main script and go-scan for imports)

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
  - [ ] Propose/discuss `PackageInfo` aggregating global variable type and value information for easier import.
- [ ] **Investigate `go-scan`'s Lazy Import (`PackageResolver`) for multi-file/package support in `minigo` (Future)**

*Legend for progress markers:*
- `[x]` Completed
- `[/]` Partially Completed (No items are partially completed now)
- `[ ]` Not Started / Incomplete
