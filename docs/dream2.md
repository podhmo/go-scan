# Go-Scan: The Dream List - Beyond the Horizon

This document outlines ambitious, long-term, and "dream-like" features and concepts for the `go-scan` library. These ideas represent a vision for a highly advanced static analysis tool and ecosystem, pushing beyond the current roadmap in `docs/todo.md`. While `docs/todo.md` focuses on concrete, planned improvements, this document explores the "what ifs" and the ultimate potential of `go-scan`.

*This document is an evolution of `docs/todo.md`'s "Broader Vision" and incorporates futuristic ideas from various `docs/ja/*.md` discussions.*

## 1. Hyper-Intelligent Source Code Understanding

This section explores features that would grant `go-scan` a much deeper, almost semantic understanding of Go code, moving beyond simple AST parsing into the realm of advanced code intelligence.

*   **1.1. Advanced Symbol Resolution and Semantic Analysis**
    *   **Vision:** `go-scan` would not just see types and functions as names, but understand their full lifecycle, scope, and relationships.
    *   **Details:**
        *   **Full-fledged Symbol Tables with Scope Analysis:** True understanding of lexical scope, visibility (public/private across packages), and shadowing of variables, types, and functions. This is crucial for accurate analysis in complex codebases. (Inspired by `docs/ja/from-minigo.md`)
        *   **Precise Data Flow Analysis (DFA) and Control Flow Graph (CFG) Generation:** The ability to trace data movement and understand the possible execution paths within functions and across function calls. This could enable advanced static analysis like taint analysis, dead code detection beyond simple reachability, or resource leak detection. (Inspired by `docs/ja/from-minigo.md`)
        *   **Deep Type System Understanding:**
            *   **Type Assignability and Compatibility:** Implementing robust checks like `isTypeAssignableTo(typeA, typeB)` that understand Go's type system rules, including interfaces and type aliases. (Inspired by `docs/ja/from-minigo.md`)
            *   **Method Resolution for Instantiated Generics:** For a generic type like `List[T]`, if `T` is instantiated with `int`, `go-scan` could determine the concrete signature of methods like `Add(int)`. (Inspired by `docs/ja/from-minigo.md`)
        *   **Complex Dependency Resolution:** Going beyond same-module resolution to accurately trace and analyze types from external dependencies, including handling `replace` directives in `go.mod`, vendored modules, and even multiple versions of the same module if the Go ecosystem ever supports it more directly. (Addresses parts of "Considerations/Known Issues" from `docs/todo.md`)

*   **1.2. Rich AST Navigation and Transformation Framework**
    *   **Vision:** Provide developers with powerful, high-level tools to query and manipulate ASTs, making complex code generation and refactoring tasks much simpler.
    *   **Details:**
        *   **XPath-like Querying for AST Nodes:** A declarative way to find specific AST nodes or patterns (e.g., "all struct fields with a specific tag and type"). (Inspired by `docs/ja/from-minigo.md`)
        *   **CSS Selector-style Matching for AST Patterns:** An alternative, possibly more intuitive way for common AST pattern matching. (Inspired by `docs/ja/from-minigo.md`)
        *   **High-Level AST Transformation Utilities:** Helpers for common AST manipulation patterns, such as adding methods to a struct, wrapping function calls, or generating adapter code (e.g., "generate a function to get a type implementing this interface from a context object"). (Inspired by `docs/ja/from-minigo.md` and `docs/ja/from-derivingbind.md`'s path parameter challenge)

*   **1.3. Sophisticated GoDoc and Annotation Metaprogramming**
    *   **Vision:** Elevate comments and GoDoc from passive documentation to active, machine-readable metadata that can drive powerful code generation and analysis.
    *   **Details:**
        *   **Advanced Parsing of GoDoc Tags:** Structurally parse common GoDoc tags (e.g., `@param <name> <description>`, `@return <description>`, `@see <symbol>`) into accessible data structures. (Inspired by `docs/ja/from-minigo.md`)
        *   **Programmable and Extensible Structured Annotation System:**
            *   Allow users or generators to define custom annotation formats (e.g., `// @myGen:option="value"`) and provide `go-scan` with parsers or rules for these.
            *   Support for sophisticated annotations like `// @validate:"required,min=0"` or `// @json:"name,omitempty"`, making them as easy to query as struct tags.
            *   Enable extraction of specific annotation values, for instance, `// @oneOfValue:"cat"` for deriving JSON unmarshalers. (Inspired by `docs/ja/from-derivingjson.md`)
        *   **Annotations Influencing Analysis:** Allow annotations to provide hints or directives to the `go-scan` engine itself, perhaps influencing type resolution strategies or marking code sections for specific analysis passes.

## 2. Next-Generation Code Generation Ecosystem

This section envisions `go-scan` as the core of a powerful and collaborative code generation ecosystem, enabling complex, multi-stage code generation workflows.

*   **2.1. The "ScanBroker" and Multi-Generator Harmony (Elaborated from `docs/ja/multi-project.md`)**
    *   **Vision:** A central `ScanBroker` that manages scanned package information, allowing multiple code generators to collaborate efficiently without redundant parsing, and even build upon each other's output.
    *   **Details:**
        *   **Dynamic Cache Invalidation and Updates:** The `ScanBroker`'s cache would need to be sophisticated enough to handle updates if a generator modifies a package that another generator later depends on. This might involve versioning or snapshotting cached `PackageInfo`.
        *   **Multi-Phase Scanning & Generation:** Support for scenarios where code generated in one phase becomes input for `go-scan` in a subsequent phase. This is crucial for complex transformations or when generators build upon each other's work.
        *   **Inter-Generator Dependency Resolution (DAG):** A mechanism for defining and resolving dependencies between generators. If Generator B needs artifacts from Generator A, the system ensures A runs first. This might involve a task graph similar to `go/analysis`.
        *   **Shared Artifact Repository:** Beyond Go code, generators might produce other artifacts (metadata files, configuration snippets). The ecosystem could provide a way to share these.
        *   **Advanced Scoped Queries:** `ScanBroker` offering powerful query capabilities across all known packages, such as `FindImplementers(interfaceType, scope)` or `FindTypesWithAnnotation(annotation, predicate, scope)`, allowing generators to discover relevant types globally or within specified modules/packages.

*   **2.2. Ultimate Build and Environment Awareness**
    *   **Vision:** `go-scan` fully understands the Go build process, including all nuances that affect which code is active.
    *   **Details:**
        *   **Deep Understanding of Build Tags:** Correctly interpret build tags (`//go:build tag`) and file name conventions (`_os`, `_arch`) to analyze only the code relevant to a specific build configuration.
        *   **`go:generate` Directive Awareness:** Recognize and potentially even trigger (or simulate the effect of) `go:generate` directives to understand code that is itself generated as part of the build process.
        *   **Simulation of Conditional Compilation:** Allow analysis under different build tag combinations to understand how the codebase changes. (Inspired by `docs/ja/from-minigo.md`)

*   **2.3. First-Class Support for Idiomatic Go Patterns**
    *   **Vision:** `go-scan` deeply understands and can accurately model common Go idioms that have semantic meaning beyond their raw syntax.
    *   **Details:**
        *   **True `iota` Evaluation:** Correctly calculate the sequence and values of constants defined using `iota`, even in complex scenarios. (Inspired by `docs/ja/from-minigo.md`)
        *   **Handling Complex Constant Expressions:** Evaluate constant expressions (e.g., `const MaxSize = 1 << 20`) to their actual values.

## 3. Developer Experience and Tooling Utopia

This section focuses on features that would make using `go-scan` and tools built upon it exceptionally powerful and user-friendly, enhancing developer productivity and insight.

*   **3.1. Interactive and Incremental Scanning**
    *   **Vision:** Provide near real-time feedback for developers using `go-scan` powered tools, such as IDE integrations or REPLs.
    *   **Details:**
        *   **Highly Performant Incremental Scanning:** When a file changes, only re-scan what's necessary, intelligently updating the `PackageInfo` and propagating changes. This is vital for language server performance. (Inspired by `docs/ja/from-minigo.md`)
        *   **Partial Scanning Capabilities:** Ability to scan only specific parts of a package (e.g., only public types and functions) or to stop scanning after certain information is found, with robust mechanisms for merging partially scanned information if needed later.

*   **3.2. Source Code Context and Diagnostics Perfected**
    *   **Vision:** `go-scan` provides exceptionally clear and actionable error messages and diagnostic information.
    *   **Details:**
        *   **Rich `GetSourceContext` API:** An API to retrieve not just the line of code for an AST node, but also surrounding lines, syntax highlighting hints, and potentially even inferred type information for variables in that context. (Inspired by `docs/ja/from-minigo.md`)
        *   **Pinpoint Accuracy in Complex Scenarios:** Even with deep type resolution chains or complex generic instantiations, error messages should clearly indicate the source of the issue.

*   **3.3. Beyond Go: Potential for Language Agnostic Concepts**
    *   **Vision:** (Extremely ambitious) The core concepts of AST parsing, symbol resolution, and caching developed for `go-scan` could be abstracted to a point where they form a toolkit applicable to other statically-typed languages.
    *   **Details:** This would involve identifying language-agnostic interfaces for AST nodes, resolvers, and symbol tables, with language-specific implementations. A true "meta-scanner" framework.

## 4. The "Meta-Circular" Go Scanner

The ultimate dream: `go-scan` becomes so advanced that it can significantly contribute to its own development or analyze its own complexities, embodying a form of meta-circular interpretation for static analysis.

*   **4.1. Self-Analysis and Optimization**
    *   **Vision:** `go-scan` can be pointed at its own codebase to identify performance bottlenecks in its parsing logic, complex type interactions that are hard to maintain, or areas where its own rules are inconsistently applied.
*   **4.2. Bootstrapping Advanced Features**
    *   **Vision:** Could `go-scan`'s AST transformation capabilities be used to generate parts of its own more advanced parsing logic? For instance, if a new Go syntax feature is introduced, could `go-scan` assist in generating the boilerplate for its own parser update?

---
*(The primary, up-to-date list of concrete planned features, ongoing tasks, and known issues can be found in [./todo.md](./todo.md). This dream list complements it by exploring longer-term possibilities. Refer to `docs/todo.md` for actionable items.)*
