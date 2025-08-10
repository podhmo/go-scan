# Plan: IDE-Friendly Configuration for `convert` with `minigo`

## 1. Introduction

The `convert` tool is a powerful code generator that automates the creation of type-safe conversion functions based on annotations in Go source code. While this annotation-based system (`@derivingconvert`, `// convert:rule`, etc.) is highly flexible, it has a significant drawback: it lacks IDE support. Developers writing conversion rules do not benefit from features like autocompletion, type checking, or "go to definition" for the types and functions referenced in the annotations. This can make complex mapping rules difficult to write, debug, and maintain.

This document proposes a new, complementary approach for configuring the `convert` tool, leveraging the `minigo` interpreter infrastructure. The goal is to allow developers to define conversion rules in a standard Go file (`.go`), which is fully supported by Go language servers (`gopls`) and other development tools.

This new method will provide a superior developer experience by making the process of defining conversions as natural as writing regular Go code, while still generating the same highly-optimized boilerplate.

## 2. Core Concept: The Declarative Mapping File

The foundation of this new approach is the **declarative mapping file**. Instead of embedding rules in comments, developers will create a dedicated Go file (e.g., `converter.go`) to define all conversion logic for a package.

This file will have two key characteristics:

1.  **It is a valid Go program**: The file will use standard Go syntax, including `package` and `import` statements. This ensures that `gopls` can parse and analyze it, providing full IDE features. Developers can import the actual source and destination types, enabling autocompletion and compile-time checks for type names.

2.  **It is excluded from the build**: The file will be marked with a build tag, such as `//go:build ignore` or a custom tag like `//go:build codegen`. This prevents the Go compiler from including it in the final application binary, as its sole purpose is to serve as a structured, declarative input for the `convert` tool.

Inside this file, developers will call "declarative functions" from a new, special-purpose `generate` package. These functions (e.g., `generate.Convert()`) are essentially stubs; they don't perform any action when executed directly. Their purpose is to act as parseable markers that declaratively capture the user's intent for the code generator.

## 3. The `generate` API (The "Verbs")

To enable the declarative mapping, we will introduce a new package, `generate`. This package will provide a set of functions that serve as the "verbs" for defining conversion rules. The `minigo`-based parser will be specifically designed to find and interpret calls to these functions.

```go
package generate

// Option is a marker interface for configuration options.
type Option interface {
	// IsOption is a marker method.
	IsOption()
}

// Convert defines a top-level conversion from a source type to a destination type.
// The src and dst arguments are zero values used to capture type information.
func Convert(src any, dst any, options ...Option) {
	// This is a stub function. It does nothing at runtime.
}

// Field maps a field from the source struct to a field with a different name
// in the destination struct.
func Field(dstFieldName string, srcFieldName string) Option { return nil }

// Ignore skips a field from the destination struct, preventing it from being populated.
func Ignore(dstFieldName string) Option { return nil }

// Use specifies a custom function for converting a single field.
// The converterFunc must have a compatible signature.
func Use(dstFieldName string, converterFunc any) Option { return nil }

// Rule defines a global conversion rule between two types.
// The converterFunc must have a compatible signature (e.g., func(T) S).
func Rule(srcType any, dstType any, converterFunc any) Option { return nil }

// Validate specifies a global validator function for a destination type.
// The validatorFunc must have a compatible signature (e.g., func(S) error).
func Validate(dstType any, validatorFunc any) Option { return nil }

// Var declares a local variable within the generated converter function,
// replicating the behavior of the `// convert:variable` annotation.
// The typeName is a string representing the variable's type (e.g., "strings.Builder").
func Var(name string, typeName string) Option { return nil }
```

## 4. Example Mapping File

Here is an example of what a complete `converter.go` mapping file would look like. This file is easy to read, write, and, most importantly, fully supported by the Go language server.

```go
//go:build ignore
// +build ignore

package main

import (
	"strings"
	"time"

	"github.com/podhmo/go-scan/examples/convert/sampledata/destination"
	"github.com/podhmo/go-scan/examples/convert/sampledata/source"

	"github.com/podhmo/go-scan/tools/generate" // The new package with declarative verbs
)

func main() {
	// This main function is the entry point for the generator tool.
	// It will not be executed by the Go compiler.

	generate.Convert(source.User{}, destination.User{},
		// Option 1: Ignore a field completely.
		generate.Ignore("Password"),

		// Option 2: Map fields with different names.
		generate.Field("FullName", "Name"),

		// Option 3: Use a custom function for a specific field conversion.
		generate.Use("CreatedAt", timeToString),

		// Option 4: Define a global rule for converting between two types.
		// This will apply to `source.Profile.Sub` -> `destination.Profile.Sub`.
		generate.Rule(source.SubProfile{}, destination.SubProfile{}, convertSubProfile),
	)

	generate.Convert(source.Order{}, destination.Order{},
		// Option 5: Declare a variable to be used by custom functions.
		generate.Var("ob", "*strings.Builder"),
		generate.Use("Description", buildDescription),
	)
}

// timeToString is a custom converter function. Its signature is understood by the generator.
func timeToString(t time.Time) string {
	return t.Format(time.RFC3339)
}

// convertSubProfile is another custom converter.
func convertSubProfile(src source.SubProfile) destination.SubProfile {
	return destination.SubProfile{
		Value: src.Value,
	}
}

// buildDescription uses the declared variable "ob".
func buildDescription(ob *strings.Builder, src source.Order) string {
	ob.WriteString("order:")
	ob.WriteString(src.Name)
	return ob.String()
}
```

## 5. The `minigo`-based Parser Architecture

This new configuration method is powered by a new tool that uses the same underlying components as the `minigo` interpreter. It is not `minigo` itself, but a purpose-built CLI tool that uses `go-scan` and AST analysis to understand the declarative mapping file. This tool replaces the existing annotation parser but reuses the entire existing code generation backend.

The process works as follows:

1.  **Parse the Mapping File**: The tool takes the path to the declarative `converter.go` file as an input. It uses `go/parser` to parse this file into a standard Go AST.

2.  **AST Traversal**: The tool walks the AST, specifically looking for `CallExpr` nodes (function calls). It identifies calls made to functions within the `generate` package (e.g., `generate.Convert`, `generate.Field`).

3.  **Argument Analysis**: When a call to a `generate` function is found, the tool analyzes its arguments:
    *   **Type Resolution**: For arguments that are zero-value struct literals (e.g., `source.User{}`), the tool uses `go-scan` to resolve these expressions back to their fully-qualified `scanner.TypeInfo`. This is the most critical step, as it provides rich type information about the user's models.
    *   **Option Processing**: For `options` arguments (e.g., `generate.Field(...)`, `generate.Use(...)`), the tool inspects the function being called and its literal arguments (e.g., the string `"FullName"`) to understand the specific configuration.
    *   **Function Resolution**: For arguments that are function identifiers (e.g., `timeToString` in `generate.Use`), `go-scan` is used to find the function's declaration and analyze its signature to ensure it's compatible.

4.  **Build the Intermediate Representation (IR)**: The information gathered from the AST is used to construct the *exact same* `model.ParsedInfo` struct that the current annotation-based parser produces. This IR contains all the `ConversionPair`, `TypeRule`, and `StructInfo` objects that the generator needs.

5.  **Code Generation**: The fully populated `ParsedInfo` object is passed to the *existing* `generator.Generate()` function. From this point forward, the process is identical to the current `convert` tool. The generator iterates over the IR and writes the `_generated.go` file.

### Architectural Diagram

This diagram illustrates how the new approach fits into the existing architecture, primarily replacing the parser component.

**Old Workflow (Annotation-based):**
```
[Annotations in .go files] -> [Annotation Parser] -> [model.ParsedInfo] -> [Generator] -> [_generated.go]
```

**New Workflow (Script-based):**
```
[Declarative converter.go] -> [Minigo-based Parser] -> [model.ParsedInfo] -> [Generator] -> [_generated.go]
                                                          ^
                                                          |
                                           (The only new component)
```

This architecture maximizes code reuse and minimizes risk. The complex and battle-tested code generation logic does not need to be changed.

## 6. Implementation and Migration Plan

The development of this feature can be broken down into manageable phases. The existing annotation-based system should be preserved for backward compatibility.

### Phase 1: Create the `generate` Package
- **Task**: Create the new `tools/generate` package.
- **Details**: Define the stub functions (`Convert`, `Field`, `Ignore`, etc.) and the `Option` interface. These functions will have empty bodies as they are only markers for the parser. Add clear documentation comments explaining the purpose of each function.

### Phase 2: Develop the `minigo`-based Parser
- **Task**: Create a new command or a subcommand (e.g., `convert-script`) that contains the new parser logic.
- **Details**: This is the core of the implementation. The new command will:
    1. Accept a path to a `converter.go` file.
    2. Use `go-scan` to parse the file and its imports.
    3. Walk the AST to find calls to the `generate` functions.
    4. For each call, analyze the arguments to resolve types and extract configuration details.
    5. Build a complete `model.ParsedInfo` struct from this analysis.

### Phase 3: Integrate with the Generator
- **Task**: Plumb the new parser into the existing generator.
- **Details**: The `ParsedInfo` struct created by the new parser will be passed to the existing `generator.Generate()` function. A new CLI entrypoint will be created to orchestrate this.

### Phase 4: Documentation and Examples
- **Task**: Update documentation and add a full example.
- **Details**:
    1. Update the main `README.md` for the `convert` tool to explain the new script-based approach.
    2. Create a new example directory (e.g., `examples/convert-script`) to provide a working demonstration of the new feature.
    3. Ensure `AGENTS.md` is updated if any new development workflows are introduced.

### Migration and Coexistence
The new script-based method should not replace the annotation-based one immediately. Both systems can coexist. The `convert` tool could have different subcommands or flags to select the parsing mode:
- `go run ./... -pkg <path>`: Runs the existing annotation-based parser.
- `go run ./... -script <path-to-converter.go>`: Runs the new script-based parser.

This allows existing projects to continue using the current system without any changes, while new projects or those willing to migrate can adopt the new, more powerful script-based configuration.
