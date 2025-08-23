# Analysis of `docgen` and `symgo` for Supporting Other Frameworks

This document provides an analysis of the `docgen` and `symgo` libraries, based on the concepts outlined in `plan-symbolic-execution-like.md`. It assesses the feasibility of supporting other web frameworks like `go-chi` and `echo` and enumerates features that may be missing for use in a real-world web application.

## 1. Supporting `go-chi` and `echo`

**Question: Can `docgen` and `symgo` be adapted to support frameworks like `go-chi` and `echo`?**

Yes, absolutely. The architecture of `symgo` as a generic symbolic execution engine and `docgen` as a specific consumer is explicitly designed to make this possible. The core `symgo` engine is framework-agnostic and does not need to be changed.

Adaptation would involve creating a new `docgen`-like tool (or extending the existing one) with framework-specific knowledge.

---

**Question: Can this be achieved with the current code alone? Or just by specifying patterns?**

No, it cannot be achieved with the current `examples/docgen` code as-is, and it requires more than just defining new patterns.

*   **Why Patterns Are Not Enough:** The "patterns" system as implemented in `examples/docgen/patterns/` is designed for analyzing the *body of a handler function*. For example, it defines how to recognize that `json.NewDecoder(...).Decode(&req)` corresponds to a request body. It does not handle the initial discovery of the route itself (e.g., the HTTP method and path).

*   **What is Required: Intrinsics:** Route discovery is handled by **intrinsics**. In `examples/docgen/analyzer.go`, you can see intrinsics registered for `net/http` functions:
    *   `net/http.NewServeMux`
    *   `(*net/http.ServeMux).HandleFunc`

    These intrinsics teach `symgo` what to do when it "sees" these specific functions being called. To support a new framework, you must provide a new set of intrinsics that teach `symgo` how to interpret that framework's routing API.

---

**Question: What code would be scanned, and what changes are needed?**

The process would be to build a new set of intrinsics tailored to the target framework's API. Let's use `go-chi` as an example.

A typical `go-chi` application looks like this:

```go
import "github.com/go-chi/chi/v5"

func main() {
    r := chi.NewRouter()
    r.Get("/users/{userID}", GetUserHandler) // Method on router
    r.Post("/users", CreateUserHandler)     // Method on router
    // ...
}

func GetUserHandler(w http.ResponseWriter, r *http.Request) {
    userID := chi.URLParam(r, "userID")
    // ...
}
```

The necessary changes would be:

1.  **Create a `chi.NewRouter` Intrinsic:**
    *   **Key:** `"github.com/go-chi/chi/v5.NewRouter"`
    *   **Action:** This intrinsic would be triggered when `symgo` sees `chi.NewRouter()`. It should return a symbolic object representing a Chi router, let's call it a `symgo.Instance` with the type name `chi.Router`.

2.  **Create Intrinsics for Router Methods:**
    *   **Key:** `(*github.com/go-chi/chi/v5.Mux).Get`, `(*github.com/go-chi/chi/v5.Mux).Post`, etc.
    *   **Action:** This intrinsic would be triggered when a method like `.Get()` is called on the symbolic `chi.Router` object. Its handler would:
        *   Extract the path string from the first argument (e.g., `"/users/{userID}"`).
        *   Extract the handler function object from the second argument (e.g., `GetUserHandler`).
        *   **Parse Path Parameters:** Unlike the current `net/http` analyzer, this intrinsic *must* parse the path string for `{...}`-style parameters. It would create a new `openapi.Parameter` for each one found (e.g., a path parameter named `userID`).
        *   Proceed with the analysis of the handler body, similar to how the current `docgen` does.

3.  **Create Patterns/Intrinsics for Chi-Specific Helpers:**
    *   **Key:** `"github.com/go-chi/chi/v5.URLParam"`
    *   **Action:** An intrinsic would be needed to understand that `chi.URLParam` is used to access a path parameter. While this could be defined as a "pattern," it's often simpler to handle it as an intrinsic within the handler analysis scope. This ensures that the documentation for the `userID` parameter can be enriched (e.g., with a type, if discoverable).

The same logic applies directly to **`echo`**, which has a similar API (`echo.New()`, `e.GET(...)`, `e.POST(...)`, `c.Param(...)`).

**Summary:** Supporting a new framework is a matter of writing a new set of `intrinsics` that mirror the framework's routing API. The core `symgo` engine is ready to support this.

### Proposed Directory Structure for Multi-Framework Support

To evolve `docgen` from an example into a robust tool that supports multiple frameworks, its directory structure should be refactored. Here are two potential approaches.

#### Proposal A: The "Natural" Monolithic Approach

This approach involves a single, primary `docgen` tool that contains the logic for all supported frameworks within different sub-packages.

**Structure:**
```
docgen/
├── cmd/docgen/
│   └── main.go        # CLI driver with a `-framework` flag
├── framework/
│   ├── chi/
│   │   └── chi.go     # go-chi intrinsics
│   ├── echo/
│   │   └── echo.go    # echo intrinsics
│   └── http/
│       └── http.go    # net/http intrinsics
├── go.mod             # Depends on chi, echo, etc.
└── ...
```

**How It Works:**
The main CLI tool would accept a flag (e.g., `-framework chi`) to select the target framework. Based on this flag, it would dynamically register the appropriate set of intrinsics from the corresponding package under the `framework/` directory.

*   **Pros:**
    *   Relatively simple to set up and understand.
    *   Adding a new framework is straightforward: just add a new sub-package.
    *   Users only need to install and learn a single binary.

*   **Cons:**
    *   The main `docgen` tool's `go.mod` file becomes bloated. It must list `go-chi`, `echo`, and every other supported framework as a direct dependency. This increases the binary size and dependency tree for all users, even those who only need to analyze one type of application.

#### Proposal B: The "Dependency-Minimized" Adapter Approach

This is a more advanced and architecturally robust approach that prioritizes dependency isolation. It refactors `docgen` into a core library and provides small, framework-specific wrapper binaries.

**Structure:**
```
/
├── docgen-core/          # New core library, no framework dependencies
│   ├── analyzer.go
│   ├── openapi/
│   └── go.mod
└── cmd/
    ├── docgen-http/
    │   ├── main.go       # Imports docgen-core, adds http intrinsics
    │   └── go.mod        # Depends on docgen-core
    └── docgen-chi/
        ├── main.go       # Imports docgen-core, adds chi intrinsics
        └── go.mod        # Depends on docgen-core AND go-chi
```

**How It Works:**
The `docgen` logic is extracted into a `docgen-core` library that contains no framework-specific code. Then, for each supported framework, a separate "adapter" binary is created in the `cmd/` directory. Each adapter has its own `go.mod` file and only depends on `docgen-core` and the specific framework library it supports.

*   **Pros:**
    *   **Perfect Dependency Isolation:** Users only download the dependencies for the framework they are using. The core library remains clean.
    *   **Scalable and Maintainable:** This is a very clean architecture that scales well to many frameworks without creating a complex, monolithic tool. It aligns with the Go philosophy of composing small, focused tools.

*   **Cons:**
    *   **Multiple Binaries:** Users need to know which binary to install and run (e.g., `go install .../docgen-chi`). This is a minor usability trade-off.
    *   **More Boilerplate:** Requires a separate `main.go` and `go.mod` for each supported framework.

For a serious, open-source tool, **Proposal B** is the recommended path.

## 2. Potential Missing Features in a Real-World Application

Here is a list of features and capabilities that would likely be needed to apply this tool to a large, complex, real-world web application.

### OpenAPI Specification Completeness

*   **Configurable Metadata:** The OpenAPI `title` and `version` are currently hardcoded in `analyzer.go`. A mature tool would need a configuration file or flags to set this metadata.
*   **`required` Fields:** The schema generator correctly identifies fields from structs but does not seem to parse validation tags (e.g., `validate:"required"`) to mark fields as `required` in the OpenAPI schema. This is a crucial feature for accurately describing an API.
*   **JSON Schema Composites (`allOf`, `oneOf`, `anyOf`):** These are not supported. Implementing them would require a more sophisticated schema generation system that could interpret specific struct tags or code conventions designed to represent polymorphism.
*   **`additionalProperties: false`:** There is no mechanism to specify that a generated schema should not allow additional properties. This is often desired for stricter API contracts.
*   **Map Responses:** The tool supports `map[string]MyType` which translates correctly to OpenAPI's `additionalProperties`. However, it relies on the map key being a string, which is an OpenAPI requirement. If the Go code used a different key type, the generated schema might be invalid or misleading.

### Scanning and Performance

*   **Is Shallow Exploration Effective?** The user asked if the tool is truly performing a shallow exploration. **Yes, it is.** The distinction between intra-module (recursively evaluated) and extra-module (treated as symbolic placeholders) calls is the core of this efficiency. `symgo` does not waste time analyzing the internals of the Go standard library or third-party dependencies unless explicitly told to via an intrinsic. This is a major strength of the design.
*   **Treating Common Libraries as "Intra-Module":** The user asked about treating a common, shared library as if it were internal to the module being scanned. `symgo`'s evaluation strategy is tied to Go's module system. If a library is in a different Go module, it is considered "extra-module." There is no simple flag to change this behavior. The correct way to handle a shared internal library that contains, for example, custom request/response helpers, is to write **custom patterns or intrinsics** for those helpers. This teaches `docgen` how to understand them, achieving the desired outcome without changing the core evaluation strategy.

### Other Potential Issues and Limitations

*   **Control Flow Heuristics:** The engine's strategy of evaluating all branches of an `if` statement and unrolling `for` loops only once is a practical heuristic. However, in a complex application, API logic might be determined by a runtime value that the symbolic engine cannot resolve. For example:
    ```go
    if user.IsAdmin() {
        json.NewEncoder(w).Encode(AdminResponse{})
    } else {
        json.NewEncoder(w).Encode(UserResponse{})
    }
    ```
    The current engine would likely detect both `Encode` calls and might overwrite the response schema, resulting in only the last one being documented. A more advanced implementation would need to support multiple response definitions for different scenarios.
*   **Path Parameter Discovery:** As noted in the `go-chi` analysis, the current `net/http` analyzer does not parse path parameters (e.g., `/users/{userID}`). This functionality is essential for any modern web framework and would need to be a core part of the intrinsics written for frameworks like `chi` or `echo`.
*   **Type Inference for Parameters:** The default pattern for query parameters (`r.URL.Query().Get("id")`) correctly identifies the parameter `id` but defaults its schema type to `string`. It does not trace the usage of the resulting variable to infer a more precise type (e.g., if it's passed to `strconv.Atoi`, it's an `integer`). This is a complex but valuable potential enhancement.
*   **Global Configuration:** Beyond OpenAPI metadata, a real-world tool would benefit from a global configuration file to define custom patterns, type mappings (e.g., how to represent `time.Time`), and other behaviors without needing to recompile the tool.

## 3. Maturity of the `symgo` Library

This section addresses the maturity of the `symgo` engine by answering specific questions about its capabilities and limitations.

**Question: Are there any Go syntax constructs that `symgo` does not support?**

Yes. `symgo` implements a significant portion of Go's syntax, but it is not exhaustive. The primary focus is on code structures relevant to static analysis of API definitions. Notable unsupported constructs include:

*   **Concurrency:** `go` statements, channels (`chan`), and `select` statements are not supported.
*   **`defer` Statements:** The `defer` keyword is not implemented.
*   **`range` Keyword:** The `for...range` loop construct is not specifically handled. The generic `for` loop handler will traverse the body once, but it won't correctly handle the assignment of keys and values from the range expression.
*   **Pointer Dereferencing:** The `evalUnaryExpr` only handles the address-of operator (`&`). It does not handle dereferencing (`*`), which is a significantly more complex operation in symbolic execution.
*   **Advanced Control Flow:** `fallthrough` in switch cases, `goto`, and other labeled statements are not supported.
*   **Variadic Arguments:** There is no specific handling for variadic arguments (`...`).

**Question: If unsupported syntax is encountered, does the evaluation stop?**

No, the entire analysis does not stop. When the evaluator encounters a node type it does not recognize (e.g., a `go` statement), it returns an `error` object. This error propagates up the evaluation stack for the *current function being analyzed*, effectively stopping the analysis of that function body. However, the overall `docgen` process will continue with other functions. This is a robust design that prevents one unsupported feature from halting the entire analysis.

---

**Question: Can `symgo` handle a function's return value being passed directly as an argument to a pattern-tracked function?**

Yes, this is handled gracefully. The evaluator first evaluates the arguments to a function call before executing the function's intrinsic.

*   **Scenario:** `patternTarget(anotherFunc())`
*   **Evaluation Flow:** `symgo` will first evaluate `anotherFunc()`.
    *   **If `anotherFunc` returns a concrete type** (e.g., `string`, `int`, or a struct), `symgo` will recursively evaluate it (if it's in the same module), determine the return value (e.g., an `object.String`), and pass that concrete object to the `patternTarget` intrinsic. The pattern handler can then inspect the value.
    *   **If `anotherFunc` returns an interface,** `symgo` will determine the return type from the function's signature and create a `SymbolicPlaceholder` object that is tagged with the interface type information. The pattern handler receives this placeholder and can identify the type, though not the underlying concrete value.

**Question: If a literal is passed as an argument to a function and then returned, is that information propagated?**

Yes. For example, if you have `myWrapper("foo")` and `myWrapper` is defined as `func(s string) string { return s }`, `symgo` will evaluate the call to `myWrapper`, see that it returns the `object.String{Value: "foo"}`, and this object will be the result of the expression. This works as expected.

---

**Question: Can `symgo` recognize the value of a global or cross-package constant when used as an argument?**

This is a key limitation in the current implementation.

*   **Scenario:** `GetQuery(r, mypkg.MyConstant)`
*   **Current Behavior:** When `symgo` evaluates `mypkg.MyConstant`, its `evalSelectorExpr` logic attempts to resolve the symbol. However, it currently only searches for **functions** within the external package. It does not look for constants (`const`) or variables (`var`).
*   **Result:** The expression `mypkg.MyConstant` resolves to a generic `SymbolicPlaceholder` with no value attached. The pattern handler for `GetQuery` would not be able to extract the constant's string value.

The underlying `go-scan` library *does* collect information about constants and variables, so the data is available. The `symgo` evaluator would need to be enhanced to look up these symbols in addition to functions.

## 4. Advanced Configuration: Treating External Packages as Internal

**Question: Is it possible to treat some external packages as if they were internal to the module, forcing `symgo` to analyze their code recursively?**

This is an excellent question that addresses a key architectural decision. As noted earlier, `symgo`'s evaluation depth is strictly tied to Go module boundaries. This is a deliberate choice for performance, but it can be limiting when an application relies heavily on a shared library in a separate Go module.

While it is not possible with the current implementation, a feature could be introduced to support this.

### Proposed Solution

A new configuration option could be added to `symgo.Interpreter` to override the default behavior for a specified list of packages.

1.  **Introduce a New Option:**
    A new option, for example `WithForcedIntraModulePackages([]string)`, would be added to `symgo.NewInterpreter`. This option would accept a list of package import path prefixes.

2.  **Modify Evaluator Logic:**
    The core change would be in `symgo`'s evaluator. The logic that decides whether to evaluate a function recursively or treat it as a symbolic placeholder would be updated:

    ```go
    // Current logic in pseudocode
    isSameModule := strings.HasPrefix(targetPackage.ImportPath, currentModule.Path)
    if isSameModule {
        EvaluateRecursively(target)
    } else {
        ReturnSymbolicPlaceholder(target)
    }

    // Proposed new logic in pseudocode
    isSameModule := strings.HasPrefix(targetPackage.ImportPath, currentModule.Path)
    isForcedIntraModule := forcedPackagesList.Contains(targetPackage.ImportPath)
    if isSameModule || isForcedIntraModule {
        EvaluateRecursively(target)
    } else {
        ReturnSymbolicPlaceholder(target)
    }
    ```

### Implementation Impact

*   **`symgo`:** The changes would be concentrated here. It requires adding the new configuration option and updating the evaluator's decision-making logic as described above.
*   **`go-scan` & `locator`:** These underlying libraries are likely already capable of supporting this feature. Their responsibility is to find and parse the source code for a given import path. As long as the target package exists in the Go module cache (`GOPATH/pkg/mod`), the existing `GoModuleResolver` should be able to locate and provide its source files to `symgo` upon request. No significant changes should be needed in these components.

### Use Case

This feature would be extremely valuable for organizations that maintain internal, shared libraries for common tasks (e.g., a package with standardized JSON response helpers). Instead of writing dozens of intrinsics or patterns for every helper function in the shared library, a developer could simply tell `symgo` to treat `github.com/my-org/my-common-library` as an internal package. `symgo` would then automatically trace function calls into that library, providing a much deeper and more accurate analysis with minimal configuration.
