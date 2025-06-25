# TODO List

This document tracks planned features and improvements for the Go Type Scanner project.

## Must-have Features

These are critical features for the library to be broadly useful in real-world projects.

- **Generics Support**: Add parsing logic for Go 1.18+ generics (e.g., `type Page[T] struct { ... }`). Without this, the scanner cannot be used in modern projects that leverage generics for reusable structures like API responses or data containers.

## Nice-to-have / Advanced Features

These features would expand the library's capabilities for more advanced use cases.

- **Interface Parsing**: Fully parse `interface` definitions, including their method sets. This would be valuable for tools like DI containers or mock generators that operate on interface contracts.
- **`iota` Evaluation**: Implement logic to correctly evaluate the integer values of constants defined using `iota`. This is useful for documentation generation where displaying the actual value of an enum is desired (e.g., `StatusActive (value: 1)`).
- **Annotation Parsing**: Support for structured comments (annotations) like `// @validate:"required,min=0"`. This would allow tools to extract rich metadata beyond what standard Go field tags provide, useful for validation, OpenAPI extensions, etc.
- **External Dependency Resolution**: Add an option to scan packages from external dependencies listed in `go.mod`. This would help in resolving common types like `uuid.UUID` to their underlying kinds (e.g., `string`), enabling more accurate schema generation. This should likely be an opt-in feature to manage performance.