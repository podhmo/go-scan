# Analysis of a REPL for `minigo`

## 1. Introduction

This document analyzes the feasibility and potential implementation strategies for adding a Read-Eval-Print Loop (REPL), or interactive shell, to the `examples/minigo` interpreter. The goal is to provide a more interactive way to experiment with the `minigo` language, which currently operates as a batch script executor.

This analysis will explore three distinct approaches:
1.  A **Simple REPL**: A basic, line-by-line interactive console.
2.  A **Rich REPL**: An enhanced user experience using a terminal UI library like `bubbletea`.
3.  **Interpreter-Specific Features**: Adding meta-commands to the REPL for introspection and environment manipulation.

The document will examine the existing architecture, discuss the implementation details and complexity of each approach, and conclude with a recommendation for a path forward.

## 2. Current Architecture

The `minigo` interpreter is currently a non-interactive, file-based executor. A user runs it from the command line, providing a path to a `.minigo` script file. The interpreter then performs the following steps:

1.  **Initialization**: In `examples/minigo/main.go`, a `minigo.Interpreter` instance is created. This object, defined in `minigo/minigo.go`, serves as the core of the runtime, holding the execution state.
2.  **State Management**: The `Interpreter` struct contains a persistent environment (`globalEnv`), a package cache, a symbol registry for Go interop, and configurable I/O streams (`stdin`, `stdout`, `stderr`). This stateful design is crucial and well-suited for a REPL, as it allows variables, functions, and imports to persist between executions.
3.  **File Loading**: The entire script file is read into memory.
4.  **Evaluation**: The script's content is passed to `interp.LoadFile()` and then `interp.Eval()`. The evaluation itself is handled by the `Evaluator` struct (`minigo/evaluator/evaluator.go`), which is a classic AST-walking interpreter. It recursively traverses the code's syntax tree and executes it sequentially.
5.  **Output**: The final result of the script's `main()` function (or the last expression) is printed to standard output, and the program exits.

This architecture is fundamentally single-threaded and processes one file in its entirety. There is no mechanism for interactive input or for preserving the interpreter's state across multiple, separate user inputs.

## 3. Approach 1: A Simple REPL

This approach focuses on creating a minimal, functional REPL with the fewest changes required. It would provide a classic interactive prompt.

### 3.1. Implementation Details

A simple REPL can be implemented by modifying the `main` function in `examples/minigo/main.go`. Instead of reading a file, it would enter an infinite loop.

```go
// main.go
func main() {
    interp, err := minigo.NewInterpreter()
    // ... register builtins ...

    scanner := bufio.NewScanner(os.Stdin)
    fmt.Print(">> ")

    for scanner.Scan() {
        line := scanner.Text()
        if line == "exit" {
            break
        }

        // How to evaluate a single line?
        // This is the main challenge. We cannot just call interp.Eval()
        // as that looks for a `main` function.

        fmt.Print(">> ")
    }
}
```

The core challenge is that the existing `Interpreter.Eval()` is designed to evaluate a whole file and then run its `main` function. For a REPL, we need to parse and evaluate one line at a time while maintaining the state in `interp.globalEnv`. This requires a new method on the `Interpreter`, such as `EvalString(input string)`.

This new method would:
1.  Parse the input string into an AST node using `parser.ParseFile`.
2.  Iterate through the declarations (`Decls`) in the parsed file.
3.  Call `e.eval.Eval(node, e.globalEnv, fileScope)` for each node.
4.  Inspect the result of the last evaluated expression and print it.

### 3.2. Pros and Cons

*   **Pros**:
    *   Relatively simple to implement.
    *   Reuses the vast majority of the existing `Interpreter` and `Evaluator` logic.
    *   No new external dependencies are required.
*   **Cons**:
    *   Poor user experience: no command history, no syntax highlighting, no multi-line editing.
    *   Error handling would be basic, likely printing the error and restarting the loop.
    *   Handling multi-line statements (like function or struct definitions) would be difficult.

## 4. Approach 2: A Rich REPL with Bubble Tea

This approach focuses on building a modern, user-friendly REPL using the `github.com/charmbracelet/bubbletea` library.

### 4.1. Implementation Details

Instead of a simple `bufio.Scanner` loop, we would build a `bubbletea` application. The `bubbletea` model would manage the REPL's state, including the input line, command history, and the `minigo.Interpreter` instance.

Key features would include:
*   **Command History**: Store each submitted line in a slice and allow the user to navigate it with up/down arrow keys.
*   **Multi-line Input**: The text input component from `bubbletea` can easily handle multi-line input, making it trivial to define functions or structs interactively.
*   **Syntax Highlighting**: While not built-in, `bubbletea` can be integrated with syntax highlighting libraries to provide real-time feedback.
*   **Autocompletion**: A more advanced feature where pressing `Tab` could suggest completions for variables or functions stored in the `interp.globalEnv`. This would require custom logic in the `bubbletea` update function.

### 4.2. Pros and Cons

*   **Pros**:
    *   Dramatically improved user experience.
    *   Provides a solid framework for adding more advanced features in the future.
    *   Handles complex UI interactions gracefully.
*   **Cons**:
    *   Introduces a new major dependency (`bubbletea` and its ecosystem).
    *   Significantly more complex to implement than the simple REPL.
    *   Requires learning the `bubbletea` framework (The Elm Architecture).

## 5. Approach 3: Interpreter-Specific Features

This approach is orthogonal to the UI and focuses on adding powerful "meta-commands" to the REPL that are not part of the `minigo` language itself. These commands would be intercepted by the REPL loop before being sent to the interpreter.

### 5.1. Implementation Details

The REPL loop (whether simple or `bubbletea`-based) would check if the input line starts with a special prefix, such as a colon (`:`).

*   **:help**: Display a list of available meta-commands.
*   **:symbols** or **:ls**: Inspect the `interp.globalEnv` and print a list of all user-defined variables and functions.
*   **:cd <path>**: Change the current working directory for the interpreter's internal `goscan.Scanner`. This would allow `import` statements to resolve packages relative to a new path. This requires exposing a way to reconfigure or interact with the scanner after the interpreter is created.
*   **:pkg <package_path>**: List all the exported symbols (functions, constants, types) from a given Go package by leveraging the `goscan.Scanner`. For example, `:pkg strings` would list `ToUpper`, `ToLower`, `Join`, etc.
*   **:reset**: Clear the current interpreter state (variables, functions) and start fresh, without having to restart the REPL application itself. This would involve creating a new `minigo.Interpreter` instance.

### 5.2. Pros and Cons

*   **Pros**:
    *   Adds powerful introspection and debugging capabilities, making the REPL a much more useful tool.
    *   Allows for environment control that is impossible from within the language itself.
*   **Cons**:
    *   Increases the complexity of the REPL's logic.
    *   Some features might require small modifications to the core `minigo` library to expose necessary components (like the scanner's configuration).

## 6. Conclusion & Recommendation

All three approaches have merit and are not mutually exclusive.

*   **Approach 1 (Simple REPL)** is the essential foundation. Without it, nothing else is possible.
*   **Approach 2 (Rich REPL)** is a significant user experience enhancement that makes the tool far more pleasant and productive to use.
*   **Approach 3 (Meta-Commands)** adds a layer of professional tooling that enables deep inspection and control, transforming the REPL from a toy into a serious development tool.

**Recommendation:**

The recommended path is a phased implementation:

1.  **Phase 1: Implement the Simple REPL (Approach 1).** This delivers immediate value and provides the core interactive functionality. This step would require creating a new `EvalString()` or similar method on the `Interpreter`.
2.  **Phase 2: Add essential Meta-Commands (Approach 3).** After the simple REPL is working, add `:help`, `:symbols`, and `:reset`. These provide high value for a relatively low implementation cost.
3.  **Phase 3: Transition to Bubble Tea (Approach 2).** Once the core logic is proven, replace the `bufio.Scanner` loop with a `bubbletea` front-end. This will immediately provide a better input experience and lay the groundwork for more advanced UI features.
4.  **Phase 4: Add advanced Meta-Commands and UI features.** With the `bubbletea` framework in place, incrementally add more complex features like `:cd`, `:pkg`, and autocompletion.
