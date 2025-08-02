# TODO

This file tracks the implementation status of features outlined in the `docs/plan-*.md` documents.

## On-Demand, Multi-Package AST Scanning (`plan-multi-package-handling.md`)
- [x] Test Harness Enhancements (`scantest`)
- [x] Unit Tests for `go-scan`
- [x] Integration Tests for `examples/convert`

## `convert` Tool Re-implementation (`plan-neo-convert.md`)
- [x] Develop the Core Tool
- [x] Replicate `examples/convert` Logic
- [x] Generate New Converters
- [x] Update Tests
- [x] Remove Prototype Code
- [ ] **Future:** Expand Test Coverage
- [ ] **Future:** Complete `README.md`
- [ ] **Future:** Parse `max_errors` from Annotation
- [ ] **Future:** Handle Map Key Conversion

## Overlay Feature for `go-scan` (`plan-overlay.md`)
- [x] Add `scanner.Overlay` type to `scanner/models.go`
- [x] Update `locator.Locator` to accept an `Overlay`
- [x] Update `scanner.Scanner` to accept an `Overlay` and resolve keys

## Enum-like Constant Scanning (`plan-scan-enum.md`)
- [ ] Modify `scanner/models.go` to support enum members
- [ ] Implement Package-Level Discovery for enums
- [ ] Implement Lazy Symbol-Based Lookup for enums
- [ ] Add Tests for Both Enum Scanning Strategies

## File and Directory Scanning Improvement (`plan-scan-improvement.md`)
- [x] Group arguments by package
- [x] Implement file-based scanning for specified files
- [x] Preserve directory-based scanning for full packages

## `scantest` Library (`plan-scantest.md`)
- [ ] **Future:** Implement file change detection in `scantest`

## Recursive Type Definition Handling (`plan-support-recursive-definition.md`)
- [x] Implement resolution history tracking
- [x] Implement cycle detection in `FieldType.Resolve()`
- [x] Provide a clean public entry point for resolution

## Unified Single-Pass Generator (`plan-walk-once.md`)
- [ ] Step 1: Create the `GeneratedCode` struct.
- [ ] Step 2: Refactor the `derivingjson` generator.
- [ ] Step 3: Refactor the `derivingbind` generator.
- [ ] Step 4: Implement the initial version of the unified `deriving-all` tool.
- [ ] Step 5: Implement tests for the unified generator using `scantest`.
- [ ] Step 6: Refine and Finalize the unified tool.
