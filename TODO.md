# TODO

## Enum-like Constant Scanning (`plan-scan-enum.md`)

- [ ] Modify `scanner/models.go`: Add `IsEnum bool` and `EnumMembers []*ConstantInfo` to `TypeInfo`.
- [ ] Implement Package-Level Discovery (Strategy 1): Implement the linking logic (e.g., `resolveEnums()`) and call it at the end of the existing `scanGoFiles` function in `scanner/scanner.go`.
- [ ] Implement Lazy Symbol-Based Lookup (Strategy 2): Create a new public method on the scanner, e.g., `ScanEnumMembers(ctx context.Context, typeSymbol string) ([]*ConstantInfo, error)`.
- [ ] Add Tests for Both Strategies: Create `goscan_enum_test.go` and add tests for both package-level discovery and lazy lookup.

## Unifying `derivingjson` and `derivingbind` (`plan-walk-once.md`)

- [ ] Step 1: Create the `GeneratedCode` struct.
- [ ] Step 2: Refactor the `derivingjson` generator to return `GeneratedCode`.
- [ ] Step 3: Refactor the `derivingbind` generator to return `GeneratedCode`.
- [ ] Step 4: Implement the initial version of the unified `deriving-all` tool.
- [ ] Step 5: Implement tests for the unified generator using `scantest`.
- [ ] Step 6: Refine and Finalize the unified tool.

## `convert` Tool Enhancements (`plan-neo-convert.md`)

- [ ] Parse `max_errors` from `@derivingconvert` annotation.
- [ ] Handle map key conversion when source and destination key types differ.
- [ ] Expand test coverage for all features, including new import functionality.
- [ ] Complete `README.md` with user-facing documentation.

## `scantest` Library Enhancements (`plan-scantest.md`)

- [ ] Implement file change detection to verify modifications to existing files during tests.
