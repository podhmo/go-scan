# TODO for convert2 Generator

## High-Level Goals

*   [ ] Implement full field mapping logic in the generator.
*   [ ] Implement robust type resolution and import management.
*   [ ] Add comprehensive tests for parser and generator.
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
*   [ ] **Type Resolution**:
    *   [ ] Resolve `ast.Expr` for types (e.g., `pair.SrcTypeExpr`, `field.Type`) to fully qualified names or `model.TypeInfo` structs that include package path and import alias information. This is crucial for the generator.
    *   [ ] Handle `type NewType BaseType` definitions and determine their underlying types for conversion purposes.
*   [ ] **Annotation Parsing Robustness**:
    *   [ ] Add more robust error handling for malformed annotations (e.g., report line numbers).
    *   [ ] Ensure correct parsing of quoted type names in `convert:rule`, especially those with package selectors (e.g., `"pkg.MyType"`).
*   [ ] **AST Node Storage**: Store relevant `ast.Node` instances (e.g., `ast.TypeSpec`, `ast.FuncDecl` for `using` functions if parsed) in `model.ParsedInfo` for richer context during generation.

### Generator (`generator/generator.go`)
*   [ ] **Field Mapping Logic (Core)** in `generateHelperFunction`:
    *   [ ] **Priority 1: Field Tag `using=<funcName>`**: Generate call to the custom function.
        *   [ ] Validate function signature (best effort with `go/ast`).
        *   [ ] Handle context parameter if `using` function expects it.
    *   [ ] **Priority 2: Global Rule `convert:rule "<SrcT>" -> "<DstT>", using=<funcName>`**: Generate call to the custom function.
        *   [ ] Match source and destination field types against global rules.
    *   [ ] **Priority 3: Automatic Conversion**:
        *   [ ] **Underlying Type Match**: If `type T1 T2` and `type D1 T2`, convert via cast (`D1(s.T1)`).
        *   [ ] **Identical Types**: Direct assignment (`d.Field = s.Field`).
        *   [ ] **Pointer Conversions**:
            *   `T` to `*T`: `ptr := s.Field; d.PtrField = &ptr` (or similar).
            *   `*T` to `T`:
                *   Default: `if s.PtrField != nil { d.Field = *s.PtrField }`.
                *   `required` tag: If `s.PtrField == nil`, add error to `ec`.
            *   `*T` to `*T`: `d.PtrField = s.PtrField`.
    *   [ ] **Priority 4: Automatic Field Name Mapping**:
        *   [ ] If `convert` tag's `DstFieldName` is empty, normalize source field name and match with normalized destination field names.
    *   [ ] **Skipping fields**: If `convert` tag is `"-"`, skip the source field.
*   [ ] **Recursive Conversions**:
    *   [ ] **Nested Structs**: If a field is a struct, generate a call to the corresponding helper function (e.g., `d.Nested = srcNestedToDstNested(ec, s.Nested)`).
    *   [ ] **Slices**:
        *   Iterate over the source slice.
        *   For each element, call the appropriate helper function for the element type.
        *   Handle `nil` slices.
        *   Example: `d.Items = make([]DstItem, len(s.SrcItems)); for i, item := range s.SrcItems { d.Items[i] = srcItemToDstItem(ec, item) }`.
*   [ ] **`convert:rule "<DstType>", validator=<funcName>`**:
    *   After a destination struct (or field of that type) is populated, generate a call to the validator function: `ValidateMyType(ec, d.MyField)` or `ValidateMyType(ec, dst)`.
*   [ ] **Import Management**:
    *   [ ] Accurately track all types used from external packages.
    *   [ ] Generate correct import statements with aliases if necessary. Consider using `golang.org/x/tools/imports`.
*   [ ] **Code Structure and Formatting**:
    *   [ ] Ensure generated helper functions are ordered logically (e.g., dependencies first, though not strictly necessary if all in one file).
    *   [ ] Add `//go:generate ...` comment if specified by user or as a convention.
*   [ ] **Error Reporting in Generator**: Improve error messages if generation logic fails for a specific construct.
*   [ ] **Context Propagation**: Ensure `ctx context.Context` is passed down to `using` functions that require it and to recursive helper calls.

### CLI (`main.go`)
*   [ ] Consider adding a `-version` flag.
*   [ ] Improve output logging, perhaps with different verbosity levels.

### General / Project
*   [ ] **Testing**:
    *   [ ] **Parser Tests**: Unit tests for `parser.ParseDirectory` with various valid and invalid annotation examples.
    *   [ ] **Generator Tests**:
        *   Define sample source structs and annotations.
        *   Run the generator.
        *   Compile the generated code.
        *   Run test functions that use the generated converters to verify correctness.
        *   Test edge cases (nil pointers, empty slices, required fields, etc.).
    *   [ ] **Integration Tests**: Test the CLI tool as a whole.
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
