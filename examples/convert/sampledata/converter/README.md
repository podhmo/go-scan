# Manual Conversion Function Sample

The `converter.go` file in this directory contains a sample of manually implemented conversion functions.

This is intended to serve as a reference for implementing complex conversion processes that cannot be handled by code generation with the `@derivingconvert` annotation, such as:

*   Resolving field name mismatches.
*   Conversions that combine multiple fields.
*   Embedding external functions or custom business logic.
*   `nil` checks and setting default values.

**Note:** The code in this directory is not directly used in the current code generation process. It is provided purely as a sample of manual implementation.
