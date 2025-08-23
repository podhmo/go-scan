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
