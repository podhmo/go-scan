# MiniGo Interpreter TODO List

This document outlines future enhancements and features for the MiniGo interpreter.

## Core Language Features

-   **Assignment Statements (`ast.AssignStmt`)**: Implement evaluation for assignment statements (e.g., `x = "new_value"` or `x = y + 1`). This is crucial for variable manipulation after declaration. (Failed tests in `TestInterpreterEntryPoint` highlight this).
-   **Global Variable Evaluation**: Implement a mechanism to evaluate and register global variable declarations (top-level `var` statements) before or alongside executing the entry point function. (Failed tests in `TestVariableDeclarationAndStringLiteral` highlight this).
-   **Integer Literals and Arithmetic**:
    -   Add `INTEGER_OBJ` to `object.go`.
    -   Support parsing and evaluation of integer literals in `interpreter.go`.
    -   Implement arithmetic operations (`+`, `-`, `*`, `/`, `%`) for integers in `evalBinaryExpr`.
    -   Add comparison operators (`<`, `>`, `<=`, `>=`) for integers.
-   **Boolean Literals**: Support `true` and `false` as keywords or literals if not already covered by `Boolean` object (currently, `Boolean` objects are only produced by comparisons).
-   **If Expressions/Statements**: Implement `if`/`else if`/`else` control flow. This will require evaluation of a condition to a `Boolean` object.
-   **Function Calls (`ast.CallExpr`)**:
    -   Support calling user-defined functions. This involves:
        -   `FUNCTION_OBJ` in `object.go`.
        -   Parsing function literals/declarations (if different from top-level `FuncDecl`).
        -   Setting up new environments for function calls (closures).
        -   Argument passing.
        -   Handling `return` statements (requires `RETURN_VALUE_OBJ`).
    -   Support calling built-in functions (e.g., `len()`, `println()`). Requires `BUILTIN_OBJ`.
-   **Return Statements (`ast.ReturnStmt`)**: Implement handling of `return` statements to exit functions and return values. This will likely involve a `ReturnValue` wrapper object.
-   **Error Handling**:
    -   Improve error reporting with more precise location information (using `token.FileSet` to convert `token.Pos` to line/column).
    -   Introduce an `ERROR_OBJ` to propagate runtime errors as objects within the interpreter, allowing for potential recovery or inspection.
-   **More Data Types**:
    -   `NULL_OBJ` and `null` literal.
    -   Arrays/Slices.
    -   Maps (Hashes).
-   **String Concatenation**: Support `+` operator for string concatenation in `evalStringBinaryExpr`.
-   **Comments**: Ensure comments are correctly ignored (currently handled by `go/parser`).

## Interpreter & Tooling Enhancements

-   **REPL (Read-Eval-Print Loop)**: Create an interactive mode for the interpreter.
-   **Standard Library**: Begin implementation of a small standard library (e.g., basic I/O, string manipulation).
-   **Testing Framework Improvements**:
    -   Refine test helpers to make it easier to check interpreter state or evaluation results without relying on global variable side effects.
    -   Add more comprehensive tests for all implemented features and error conditions.
-   **`go-scan` Integration**: Leverage `go-scan` for more detailed token-level analysis or advanced error reporting if `go/parser`'s information is insufficient for some use cases.
-   **Kebab-case/Snake_case Conversion**: Implement built-in functions or a mechanism for string case conversions as originally requested (e.g., `to_kebab_case("MyString")`, `to_snake_case("MyString")`).

## Code Quality & Refactoring

-   **`FileSet` Propagation**: Pass `token.FileSet` to evaluation functions to allow for better error message formatting (line/column numbers instead of just `token.Pos` integers).
-   **Type System**: Consider if and how to implement a more formal type system or type checking, especially if the language evolves.
-   **Performance**: Analyze and optimize performance bottlenecks as the interpreter grows.

This list is not exhaustive and will evolve as the project progresses.
