# Analysis of `symgo` Implementation

This document analyzes the implementation of the `symgo` symbolic execution engine, focusing on its handling of control flow. The analysis compares the current source code with the principles outlined in `sketch/plan-symbolic-execution-like.md` to determine if the implementation deviates from its intended design.

## 1. Core Question

The primary questions to be answered are:
1.  Does the `symgo` source code implement more features of a standard interpreter than necessary for its purpose? Specifically, is control flow (e.g., `if`, `for`) handled in a way that attempts to mimic a traditional interpreter?
2.  Do the test suites for `symgo` enforce assumptions that are more aligned with a standard interpreter than a symbolic tracer?

## 2. Source Code Analysis

The investigation focused on `symgo/evaluator/evaluator.go`, which contains the core logic for AST traversal and evaluation.

### 2.1. `if` Statements (`evalIfStmt`)

The implementation of `if` statements is a clear departure from a standard interpreter.

- **Behavior:** The `evalIfStmt` function evaluates the `Init` statement and the `Cond` expression to trace any function calls within them. Then, it proceeds to evaluate **both** the `then` block (`Body`) and the `else` block (`Else`) in separate, isolated scopes.
- **Alignment with Design:** This is a direct and faithful implementation of the strategy in `sketch/plan-symbolic-execution-like.md` (Section 3.4), which states the goal is to "discover what *could* happen in any branch" rather than proving which single path is taken.

Furthermore, the `assignIdentifier` function contains special logic for variables with an `interface` type. When a value is assigned to an interface variable, the evaluator **adds** the concrete type of the value to a set of `PossibleTypes` on the variable object. This "additive update" mechanism is how the evaluator correctly merges the outcomes of both `if` and `else` branches, allowing it to track all possible concrete types an interface variable might hold. This is a sophisticated feature designed specifically for static analysis, not for standard execution.

### 2.2. `for` and `for...range` Loops (`evalForStmt`, `evalRangeStmt`)

The handling of loops is also intentionally non-interpreter-like.

- **Behavior:** Both `evalForStmt` and `evalRangeStmt` follow a "unroll once" strategy. They evaluate expressions in the loop's definition (the `Init` statement for a `for` loop, and the range expression for a `for...range` loop) to trace function calls. Then, they evaluate the loop's `Body` **exactly one time**. They do not evaluate the loop condition or post-statement, and they do not iterate.
- **Alignment with Design:** This perfectly matches the "Bounded Analysis" strategy described in the design document (Section 3.4). This approach extracts necessary symbolic information (i.e., function calls inside the loop body) while avoiding the halting problem and the complexity of analyzing loops to completion.

### 2.3. Recursion Handling (`applyFunction`)

The engine's recursion handling is another indicator of its specialized design, favoring termination and bounded analysis over deep execution.

- **Behavior:** The evaluator now employs a **bounded recursion** strategy, consistent with its handling of `for` loops. It detects recursive function calls by checking if the same function definition appears in the call stack more than once.
    - For methods, this check is state-aware: it only considers it a recursion if the call is on the **same receiver object**. This allows it to correctly trace through recursive data structures like linked lists (`n.Next.Traverse()`).
    - For regular functions, any direct recursive call is bounded.
- **Bounding Logic**: The analysis is allowed to go **one level deep** into a recursive call. If the same function is called a second time in the stack (meeting the criteria above), the evaluator halts the analysis for that path and returns a symbolic placeholder representing the function's result. It does **not** produce an error.
- **Alignment with Design:** This strategy is a significant refinement that improves the engine's robustness. By guaranteeing termination for all recursive patterns, it prevents hangs in tools like `find-orphans` when analyzing complex, deeply recursive code. This pragmatic choice reinforces the engine's design as a static analysis tool that prioritizes finishing its analysis over perfectly simulating the program's execution.

### 2.4. `switch` Statements (`evalSwitchStmt`, `evalTypeSwitchStmt`)

The handling of `switch` and `type switch` statements follows the same "explore all paths" philosophy as `if` statements, but with important nuances.

- **`switch` Statements (`evalSwitchStmt`)**: The evaluator does not attempt to determine which `case` branch will be executed at runtime. Instead, it iterates through **all** `case` clauses and evaluates the body of each one in a separate scope. This ensures all potential function calls are discovered.

- **`type-switch` Statements (`evalTypeSwitchStmt`)**: This statement is handled with a specific strategy to maximize path discovery, especially when the type being switched on is a symbolic interface.
    - **Behavior**: For each typed `case T:` block, the evaluator creates a **new, symbolic instance** of type `T` and assigns it to the case variable (e.g., `v`). This allows the tracer to hypothetically explore the code path within that block as if the interface variable had been of type `T`.
    - **Alignment with Design**: This is a direct implementation of the tracer philosophy. By creating new symbolic instances for each branch, the engine can analyze all possible paths without needing a concrete value, ensuring a complete call graph is generated. This contrasts with the `if-ok` assertion, which clones a known concrete value for a single known path.

### 2.5. Function, Closure, and Call Handling

The engine's handling of function calls, including complex cases like anonymous functions and closures, is highly specialized for static analysis.

- **Standard Calls (`applyFunction`)**: Function calls are handled by creating a new evaluation scope for the function body. The `object.Function` captures the environment where it was defined, which correctly models lexical scoping and allows closures to work as expected. When the function is called, its new scope is created as a child of its definition environment, not the caller's environment.

- **Anonymous Functions as Arguments (`scanFunctionLiteral`)**: The engine has a specific, powerful heuristic for this case. When `evalCallExpr` detects that an argument is a function literal (e.g., `http.HandlerFunc(func(w, r){...})`), it immediately triggers a separate, temporary evaluation of that literal's body (`scanFunctionLiteral`). This allows the engine to discover calls *inside* the anonymous function, even if the function it is passed to (`http.HandlerFunc`) is an un-analyzed external function or an intrinsic that doesn't evaluate its arguments. This is a crucial feature for tools like `find-orphans`.

- **Closures as Return Values**: Because function objects capture their definition environment, returning a closure from a function works correctly. When the returned closure is eventually called, it will execute within the environment it captured, giving it access to the variables from the function that created it.

### 2.6. Variable, Scope, and State Handling

The engine's approach to variables and scoping is fundamental to its ability to trace Go code correctly.

- **Lexical Scoping and Shadowing**: The evaluator correctly models Go's lexical scoping. Each block (`{...}`), `if`, `for`, or `switch` statement creates a new, enclosed environment. When a variable is defined with `:=` (`env.SetLocal`), it is created in the current, innermost scope, naturally shadowing any variable with the same name in an outer scope. When a variable is updated with `=`, the engine searches up through the chain of enclosing environments (`env.Get()`) to find and modify the first variable it encounters with that name.

- **Package-Level Declarations (`ensurePackageEnvPopulated`)**: Package-level declarations are handled in a specific way to balance correctness and performance.
    - **Constants**: Are evaluated eagerly and their concrete values are stored in the package's environment.
    - **Variables**: Are handled **lazily**. The engine creates an `object.Variable` placeholder with the initializer expression attached but unevaluated. The evaluation is deferred until the variable is actually used, at which point `forceEval` is called. This is a key performance optimization.

- **Special Interface Assignment**: As mentioned previously, assignment to interface-typed variables is "additive," which is a core feature of the symbolic engine that allows it to track multiple possible concrete types for an interface across different code paths.

### 2.7. Module and Import Handling

The engine's ability to handle dependencies is managed through a clear separation of concerns between the evaluator and a dedicated resolver.

- **Import Resolution**: When the evaluator encounters an identifier that is not defined in the current scope (e.g., `http` in `http.NewRequest`), the `evalIdent` function checks the current file's import declarations. If the identifier matches an import's package name, it triggers the package loading mechanism.

- **Package Loading (`getOrLoadPackage`, `resolver.go`)**: The `getOrLoadPackage` function acts as a caching layer. If a package is requested for the first time, it delegates to the `Resolver`. The `Resolver` is responsible for using the underlying `go-scan` library to find the package's source files on disk, parse them, and return the scanned package information. This on-demand, lazy loading of packages is a key performance feature.

- **Scan Policy**: The `Resolver` holds a `ScanPolicyFunc`, which allows the user of the `symgo` library to define which packages should be deeply analyzed (by parsing their function bodies) and which should be treated as symbolic dependencies (where function bodies are ignored). This is the mechanism that implements the "Intra-Module vs. Extra-Module" evaluation strategy described in the design documents. A recent bug was fixed where this mechanism was not correctly triggered from `evalSelectorExpr`, which has now been corrected to ensure the scan policy is always respected during symbol resolution.

### 2.8. Method Call Resolution

The engine has robust support for resolving method calls, including on interfaces and embedded types.

- **Embedded Method Calls (`accessor.go`)**: The `accessor` component implements a recursive, depth-first search to find methods on embedded types. When resolving a method call on a struct, it first checks for methods on the struct itself. If none is found, it iterates through the struct's embedded fields and recursively searches on their types. This correctly models Go's method promotion rules.

    A recent enhancement has made this process more robust. The search logic in the `accessor` was refined to be more resilient: it now considers an embedded type "unresolved" if its import path is missing (due to incomplete scanner information) or if the path is explicitly out-of-policy. Instead of halting the search immediately, it now continues to search through all other in-policy embedded types. Only if the member is not found after exhausting all scannable paths does the accessor return the special `ErrUnresolvedEmbedded` error. The `evaluator` is designed to catch this specific error, log a warning that it is assuming the member exists, and return a `SymbolicPlaceholder`. This allows analysis to continue without halting, which is critical for analyzing codebases that have legitimate dependencies on packages outside the primary analysis scope or where scanner information may be incomplete.

- **Interface Method Calls (`evalSelectorExpr`, `applyFunction`)**: A call to an interface method is handled in a purely symbolic way. The engine does not attempt to perform dynamic dispatch to a concrete type. Instead, `evalSelectorExpr` resolves the call to the method signature on the interface definition itself. Then, `applyFunction` recognizes this as a call to a symbolic interface method and returns a `SymbolicPlaceholder` representing the method's return value(s), correctly typed according to the signature. This is a crucial design choice for static analysis, where the concrete type of an interface is often unknown.

    A recent enhancement has made this process even more robust. If a method is not found in the interface's static definition (which can happen if the interface type is from a package outside the primary analysis scope), the evaluator no longer returns an error. Instead, it creates a synthetic `scanner.FunctionInfo` for the method on the fly. This placeholder `FunctionInfo` has the correct name but no parameters or results, as they are unknown. This allows the analysis to proceed and for a callable `SymbolicPlaceholder` to be returned, preventing the analysis from halting on incomplete type information. This behavior is critical for tools that analyze code that is assumed to be valid and compilable.

- **Interface Implementation Checks (`isImplementer`)**: The logic that checks if a concrete type implements an interface correctly follows Go's method set rules. Specifically, it understands that for a type `T`, its method set includes all methods with a value receiver `(t T)`. For a pointer type `*T`, its method set includes methods with both value `(t T)` and pointer `(t *T)` receivers. Because the method set of `*T` is a superset of `T`'s method set, the implementation check does not need to distinguish between pointer and value receivers when checking if a type satisfies an interface; it correctly handles both cases as per the language specification. This is an intentional design choice.

### 2.9. Other Evaluated Expressions

The engine selectively evaluates expressions to find function calls, prioritizing discovery over computation.

- **Composite Literals (`evalCompositeLit`)**: When evaluating struct or map literals, the engine traverses all key-value pairs. It recursively calls `Eval` on the value expressions (e.g., `Value` in `Field: Value`) and, for maps, on the key expressions as well. This ensures that any function calls used to generate field values or map keys are discovered.

- **Control Flow Conditions**:
    - **`if` and `switch`**: The condition expressions are evaluated to trace any function calls within them.
    - **`for`**: The condition expression of a `for` loop is **not** evaluated. This is a deliberate design choice to avoid the complexity and potential for non-termination involved in analyzing loop conditions. The engine favors a simpler "unroll once" strategy for loops.

### 2.10. `defer` and `go` Statement Handling

`defer` and `go` statements are handled in a simple, pragmatic way that is aligned with the tracer's goals.

- **Behavior**: The `evalDeferStmt` and `evalGoStmt` functions do not model the special runtime semantics of deferred or concurrent execution. They simply extract the `CallExpr` from the statement and evaluate it directly.
- **Alignment**: This approach correctly adds the deferred or concurrent function call to the call graph, which is the primary goal for tools like `find-orphans`. It avoids the immense complexity of simulating schedulers or execution order.

### 2.11. Pointer Operations

The engine correctly models pointer referencing and dereferencing to enable analysis of pointer-based code.

- **Reference (`&`)**: The `&` operator is handled in `evalUnaryExpr`. It evaluates the operand, then wraps the resulting object in a new `object.Pointer`. It also correctly constructs the corresponding pointer type information, ensuring type safety is maintained.

- **Dereference (`*`)**: The `*` operator is handled in `evalStarExpr`. It evaluates the operand to get a pointer object. If it's a concrete `object.Pointer`, it returns the `Value` the pointer points to. Crucially, if it's a `SymbolicPlaceholder` representing a pointer, it does not fail; instead, it returns a *new* symbolic placeholder representing the element type. This allows analysis to continue through chains of pointer dereferences even when the concrete values are not known.

### 2.12. Data Structure Recursion Handling

In addition to handling recursive function calls, the engine is robust against infinite recursion caused by cyclic data structures.

- **Embedded Structs (`accessor.go`)**: When resolving methods or fields on embedded structs, the `accessor` uses a `visited` map, keyed by type name, to track which types have already been seen during the current resolution. If a cycle is detected, the recursion is terminated.

- **Composite Literals and Variables (`evaluator.go`)**: When evaluating composite literals or variable initializers, the evaluator uses an `evaluationInProgress` map, keyed by the `ast.Node`. This prevents infinite loops for self-referential or mutually recursive definitions (e.g., `var V = T{F: &V}`).

## 3. Test Code Analysis

The tests in `symgo/evaluator/` confirm the non-interpreter behavior.

- **`evaluator_if_stmt_test.go`**: The tests do not check for conditional execution. They verify that function calls in the `if` condition are traced and that the statement integrates correctly with the rest of the evaluator's logic (e.g., not producing an erroneous return value).
- **`evaluator_range_test.go`**: The test for `for...range` only asserts that a function call in the range expression was traced. It does not assert that the loop body was executed multiple times.
- **`evaluator_recursion_test.go`**: The tests explicitly verify that the engine correctly distinguishes between valid, state-changing recursion (linked list traversal) and true infinite recursion. It also confirms robustness against recursive data structure definitions.

The tests do not demand "extra" assertions that would only be true for a standard interpreter. They are tailored to verify the specific, intended behavior of the symbolic engine.

## 4. Conclusion

1.  **The `symgo` implementation is not an over-engineered interpreter.** It is a deliberate and precise implementation of the symbolic tracing engine described in the design documents. Its handling of control flow is intentionally simplified to meet the needs of static analysis: discover all possible calls, rather than execute a single path.

2.  **The test suite correctly reinforces the symbolic engine's design.** The assertions validate that function calls are traced and that the engine is robust against complex patterns like recursion, which is exactly what is required.

The implementation is a strong example of a tool built for a specific purpose, correctly avoiding the complexities of building a full language interpreter where they are not needed.

---

## 5. Detailed Test Code Analysis

Based on a review of the test files in `symgo/evaluator/`, the test suite is consistently aligned with the goals of a symbolic engine. The tests prioritize verifying call graph tracing, type propagation, and robustness over simulating concrete execution.

| File | Purpose | Judgment |
|---|---|---|
| `accessor_test.go` | Verifies that helper functions can correctly find field and method definitions on a given type. | **Aligned** |
| `evaluator_assignment_test.go` | Tests the mechanics of simple, multi-value, and complex LHS assignments. | **Aligned** |
| `evaluator_binary_expr_test.go`| Tests evaluation of binary expressions on basic literals. | **Mostly Aligned** |
| `evaluator_call_test.go` | Tests various function call patterns, including intrinsics, method chaining, interface methods, and out-of-policy calls. | **Aligned** |
| `evaluator_channel_test.go` | Verifies that function calls on both sides of a channel send are traced, and that channel types are parsed without error. | **Aligned** |
| `evaluator_channel_concrete_test.go` | Verifies that `make(chan T)` produces a correctly typed channel object and that a receive `<-ch` produces a correctly typed symbolic placeholder. | **Aligned** |
| `evaluator_complex_test.go` | Tests evaluation of arithmetic expressions on complex number literals. | **Mostly Aligned** |
| `evaluator_typeswitch_test.go` | Verifies that the evaluator explores all branches of a `type switch` and correctly handles scoping within each `case`. | **Aligned** |
| `evaluator_test.go` | A general test file. Contains tests confirming that calls in `defer` and `go` statements are traced, and that closures are handled correctly. | **Aligned** |
| `evaluator_variable_test.go` | Tests the basic mechanics of `var` declaration and reassignment. | **Mostly Aligned** |
| `evaluator_nested_block_test.go` | Confirms that function calls inside nested blocks are traced and that variable shadowing and assignment follow correct lexical scoping rules. | **Aligned** |
| `symgo_intramodule_test.go` | Verifies that calls to other packages within the same module are recursively evaluated. | **Mostly Aligned** |
| `symgo_extramodule_test.go` | Verifies that calls to external modules are treated as symbolic placeholders and not evaluated. | **Aligned** |
| `evaluator_interface_method_test.go` | Confirms that interface method calls are not dynamically dispatched, but are traced symbolically, and that all possible concrete types are tracked. | **Aligned** |
| `evaluator_embedded_method_test.go` | Verifies that the engine correctly resolves method calls on embedded structs, following Go's promotion rules. | **Aligned** |
| `evaluator_composite_literal_test.go` | Confirms that function calls within the keys and values of struct/map literals are traced. | **Aligned** |
| `evaluator_unary_expr_test.go` | Verifies that pointer dereferences on symbolic pointers are handled gracefully, and that basic unary operations on literals are evaluated. | **Aligned** |

### Summary of Test Philosophy

- **Aligned Tests**: The majority of tests fall in this category. They assert on symbolic properties: Was a function called? Was an intrinsic triggered? Does the resulting symbolic placeholder have the correct type information? Does the engine correctly handle recursion and out-of-policy resolution? These tests directly validate the engine's fitness for static analysis.
- **Mostly Aligned Tests**: A smaller category of tests asserts on concrete values (e.g., `2 + 2 == 4` or string concatenation). While this is characteristic of a standard interpreter, it is a necessary and acceptable practice here. It ensures the foundational ability to handle constant expressions, which is required for many static analysis tasks (like resolving file paths from string literals). These tests do not involve complex state or path-dependent logic.

The test suite as a whole strongly confirms that `symgo` is being developed and validated as a symbolic tracer, not a general-purpose interpreter.

## 6. Feedback

Based on the comprehensive analysis of the `symgo` source code and its corresponding test suite, the following feedback is provided:

- **Excellent Design Alignment**: The implementation is exceptionally well-aligned with the documented design philosophy. The pragmatic, non-interpreter approach to control flow and state management is consistently applied and is fit-for-purpose for the engine's static analysis goals.

- **Robustness**: The engine and its tests demonstrate a strong focus on robustness. The handling of recursion, out-of-policy types, and complex expressions (like assignments with type assertions) shows a mature design that is resilient to the complexities of real-world Go code.

- **Conclusion**: The investigation confirms that `symgo` is not an over-extended interpreter. It is a well-designed symbolic tracing engine. No fundamental architectural changes are recommended based on this analysis. The design is sound.

### 2.13. `if-ok` Type Assertion Handling (`evalAssignStmt`)

The engine's handling of the `v, ok := i.(T)` idiom is a key feature for enabling precise analysis of type-narrowed code. The implementation correctly preserves the concrete value of the variable `v` after a successful assertion.

-   **Behavior**: When `evalAssignStmt` encounters a two-value type assertion, it does not simply create a `SymbolicPlaceholder` for `v`. Instead, it performs the following steps:
    1.  It evaluates the expression `i` to get the underlying object that the interface holds.
    2.  It uses the newly introduced `object.Object.Clone()` method to create a shallow copy of this underlying object.
    3.  This cloned object is assigned to the new variable `v`. The `ok` variable is assigned the `object.TRUE` singleton.
-   **`Clone()` Method**: To support this, the `Clone() Object` method was added to the `object.Object` interface and implemented for all concrete object types. This method creates a shallow copy of the object, which is crucial for preserving the state (e.g., field values of a struct) of the original object in the new variable `v` without creating shared-state side effects.
-   **Alignment with Design**: This approach is critical for symbolic analysis. By preserving the concrete value, the engine can subsequently trace member access (e.g., `v.ConcreteField`) or method calls (`v.ConcreteMethod()`) on the narrowed variable `v`, even if those members were not part of the original interface `i`. This allows for a much deeper and more accurate analysis of Go's idiomatic type-handling patterns.
