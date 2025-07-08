# TODO for convert2 Generator

## High-Level Goals

*   [P] Implement full field mapping logic in the generator. (Basic `using` tag support added)
*   [X] Implement robust type resolution and import management ( 기본적인仕組みは実装、さらなる改善が必要).
*   [P] Add comprehensive tests for parser and generator. (Generator unit tests for pointers and basic `using` added)
*   [ ] Refine `README.md` with detailed usage, examples, and installation instructions.

## Appendix: 懸念事項と将来の検討事項 (from specification document)

1.  **パフォーマンス:**
    *   **懸念:** `errorCollector` を介したエラー収集やパスの動的な組み立ては、大量のデータ変換時にオーバーヘッドとなる可能性がある。特に `fmt.Sprintf` をループ内で多用すると影響が出やすい。
    *   **検討事項:** 高性能モード（例: Fail Fast戦略、パス追跡なし）をオプションとして提供することを検討する。

2.  **AST解析の限界 (`go/types` 不使用):**
    *   **懸念:** 外部パッケージで定義された型のアンダーライングタイプを特定できない。また、`type T1 T2; type T2 T3;` のような多段の型定義の解決は複雑になる。
    *   **検討事項:** 解析対象を同一モジュール内のソースコードに限定する、という明確な制約をドキュメント化する。もし将来的に必要性が高まれば、`go/packages` への移行を検討する。 (`go-scan`の利用も視野に)

3.  **カスタム関数のシグネチャ検証:**
    *   **懸念:** `using` で指定された関数が規約通りのシグネチャを持っていない場合、生成されたコードがコンパイルエラーとなる。このエラーはツール実行時ではなく、`go build` の段階で初めて発覚する。
    *   **検討事項:** 限定的ながらも、`go/ast` を使って指定された関数のシグネチャを簡易的にチェックし、ミスマッチがあればツール実行時に警告を出す仕組みを検討する。

4.  **循環参照:**
    *   **懸念:** `type A struct { B *B }; type B struct { A *A }` のような循環参照する構造体が含まれている場合、コード生成が無限ループに陥る可能性がある。
    *   **検討事項:** ヘルパー関数の生成計画を立てる際に、既知の型ペアのスタックを保持し、循環を検出した場合はエラーとして報告するロジックを実装する必要がある。

5.  **アノテーションの複雑化:**
    *   **懸念:** 機能追加に伴い、アノテーションのオプションが増え、学習コストが上昇する可能性がある。
    *   **検討事項:** 機能追加時には、既存のルールの組み合わせで実現できないかをまず検討する。ドキュメントと豊富な用例を整備し、ユーザーをサポートする。

## Detailed Implementation Todos

### Parser (`parser/parser.go`)
*   [X] **Type Resolution (Basic Implementation)**:
    *   [X] Resolve `ast.Expr` for types (e.g., field types, `convert:pair` types) to `model.TypeInfo` including package path and basic kind. (Further robustness needed for complex selectors, dot imports, and forward declarations).
    *   [X] Handle `type NewType BaseType` definitions: identify and store in `ParsedInfo.NamedTypes`; if `BaseType` is a struct, treat `NewType` as an alias in `ParsedInfo.Structs`. (Underlying type chain resolution for `type T1 T2; type T2 T3;` is still basic).
*   [ ] **Type Resolution (Advanced)**:
    *   [ ] Improve resolution of types from dot (`.`) imports.
    *   [ ] Handle forward declarations more gracefully (may require multiple parsing passes or deferred resolution).
    *   [ ] Accurately determine `TypeInfo.FullName` and `TypeInfo.PackageName` (as import alias) for all scenarios.
*   [ ] **Annotation Parsing Robustness**:
    *   [ ] Add more robust error handling for malformed annotations (e.g., report line numbers, specific parsing errors).
    *   [ ] Ensure correct parsing of complex quoted type names in `convert:rule` (e.g., `"[]*pkg.MyType"`).
*   [ ] **AST Node Storage**: Review if more `ast.Node` instances need to be stored in `model.ParsedInfo` for richer context during generation (e.g., for `using` function signature checks).

### Generator (`generator/generator.go`)
*   [P] **Field Mapping Logic (Core)** in `generateHelperFunction`: (Basic `using` support added)
    *   [X] **Skipping fields**: If `convert` tag is `"-"`, skip the source field. (Implemented)
    *   [X] **Identical Types & Name Mapping**: Direct assignment (`d.Field = s.Field`) if types and names (or tag-specified name) match. (Basic implementation for `TypeInfo.FullName` match).
    *   [X] **Pointer Conversions**: (Implemented for `T` <-> `*T` where element types match)
        *   [X] `T` to `*T`: `valT := s.Field; d.PtrField = &valT`.
        *   [X] `*T` to `T`:
            *   [X] Default: `if s.PtrField != nil { d.Field = *s.PtrField }`.
            *   [X] `required` tag: If `s.PtrField == nil`, add error to `ec`.
        *   [X] `*T` to `*T`: `d.PtrField = s.PtrField` (Covered by identical type direct assignment).
    *   [X] **Priority 1: Field Tag `using=<funcName>`**: Generate call to the custom function. (Basic implementation, assumes `func(ec, val)` signature)
        *   [ ] Validate function signature (best effort with `go/ast`).
        *   [ ] Handle context parameter if `using` function expects it (e.g. via naming convention or tag option).
    *   [X] **Priority 2: Global Rule `convert:rule "<SrcT>" -> "<DstT>", using=<funcName>`**: Generate call to the custom function. (Basic implementation, assumes `func(ec, val)` signature)
        *   [X] Match source and destination field types against global rules using resolved `TypeInfo`.
    *   [ ] **Priority 3: Automatic Conversion (Advanced)**:
        *   [ ] **Underlying Type Match**: If `type T1 T2` and `type D1 T2` (and T2 is compatible), convert via cast (`D1(s.T1)`). Needs careful check of compatibility based on `TypeInfo.Underlying`.
    *   [ ] **Priority 4: Automatic Field Name Mapping (Normalized)**:
        *   [ ] If `convert` tag's `DstFieldName` is empty, normalize source field name and match with normalized destination field names.
*   [ ] **Recursive Conversions**:
    *   [X] **Nested Structs**: If a field is a struct, generate a call to the corresponding helper function (e.g., `d.Nested = srcNestedToDstNested(ec, s.Nested)`). Ensure helper functions for nested structs are generated.
    *   [ ] **Slices**:
        *   Iterate over the source slice.
        *   For each element, call the appropriate helper function for the element type.
        *   Handle `nil` slices.
        *   Example: `d.Items = make([]DstItem, len(s.SrcItems)); for i, item := range s.SrcItems { d.Items[i] = srcItemToDstItem(ec, item) }`.
*   [ ] **`convert:rule "<DstType>", validator=<funcName>`**:
    *   After a destination struct (or field of that type) is populated, generate a call to the validator function: `ValidateMyType(ec, d.MyField)` or `ValidateMyType(ec, dst)`.
*   [X] **Import Management (Basic Implementation)**:
    *   [X] Accurately track types used from external packages based on `TypeInfo.PackagePath`.
    *   [X] Generate import statements. (Alias collision handling is basic, needs improvement for robustness. `using` function import handling is also basic).
*   [ ] **Import Management (Advanced)**:
    *   [ ] Robust import alias generation to avoid conflicts.
    *   [ ] Consider using `golang.org/x/tools/imports` for final import list cleanup and organization.
*   [ ] **Code Structure and Formatting**:
    *   [ ] Ensure generated helper functions are ordered logically if it impacts compilation (e.g. for mutually recursive types, though direct recursion is the main concern).
    *   [ ] Add `//go:generate ...` comment in generated file if specified by user or as a convention.
*   [ ] **Error Reporting in Generator**: Improve error messages if generation logic fails for a specific construct (e.g. "cannot convert type X to Y for field Z").
*   [ ] **Context Propagation**: Ensure `ctx context.Context` is passed down to `using` functions that require it and to recursive helper calls for nested structs/slices.

### CLI (`main.go`)
*   [ ] Consider adding a `-version` flag.
*   [ ] Improve output logging, perhaps with different verbosity levels.

### General / Project
*   [X] **Testing (Preparation & Basic Pointer/Using Tests)**:
    *   [X] Define sample source structs and annotations in `testdata/simple`.
    *   [X] Manually/logically run generator and visually/logically inspect output.
    *   [X] Updated `simple_test.go` for implemented pointer conversions.
    *   [X] Added unit tests to `generator_test.go` for pointer and basic `using` logic.
*   [P] **Testing (Implementation - Broader Coverage)**: (Unit tests for generator's pointer and basic `using` logic added)
    *   [ ] **Parser Tests**: Unit tests for `parser.ParseDirectory` with various valid and invalid annotation examples.
    *   [P] **Generator Tests (Automated)**: (Unit tests for pointer and basic `using` logic added)
        *   [ ] Compile the generated code from `testdata/simple`.
        *   [ ] Run Go test functions (`_test.go`) in `testdata/simple` that use the generated converters to verify correctness for all implemented features.
        *   [ ] Test edge cases (nil pointers, empty slices).
    *   [ ] **Integration Tests**: Test the CLI tool as a whole with different input directories.
*   [ ] **Documentation (`README.md`)**:
    *   [ ] Add detailed installation instructions.
    *   [ ] Provide comprehensive examples of all annotation types and options.
    *   [ ] Explain generated code structure.
    *   [ ] Document limitations (e.g., `go/ast` vs `go/types`).
*   [ ] **Example Usage**: Create a dedicated `examples/` subdirectory within `convert2` (e.g., `examples/convert2/example/`) with sample model definitions and a `doc.go` or `conversions.go` showing usage of annotations, to test the generator itself.

### Future Enhancements (Post v1.0 - from Spec Appendix)
*   [ ] **Performance Mode**: Option for "Fail Fast" error strategy.
*   [ ] **`go/packages` or `go-scan`**: For more robust cross-package type analysis if `go/ast` limitations become too restrictive.
*   [ ] **Advanced Custom Function Signature Validation**: More sophisticated checks for `using`/`validator` functions.
*   [ ] **Circular Dependency Detection**: Implement detection during generation planning.
*   [ ] **Annotation Simplification**: Continuously review annotation design for clarity and ease of use.

**Legend:**
* `[X]` - Done
* `[P]` - Partially Done / In Progress
* `[ ]` - To Do
