# `examples/convert` における型変換と `go-scan` の活用

`examples/convert` は、Goの構造体間で型を変換するコード（コンバータ）を自動生成するジェネレータの試作例です。このジェネレータは `go-scan` を利用してソースコードから型情報を取得し、それに基づいて変換ロジックを構築します。

## 1. `go-scan` に求められる機能と現状の評価

`examples/convert` のような型変換ジェネレータを開発する上で、`go-scan` には以下の機能が求められます。

### 1.1. 型情報の詳細取得

ジェネレータは、変換元（Source）と変換先（Destination）の型について詳細な情報を必要とします。

*   **構造体のフィールド情報**:
    *   フィールド名、型、タグ、ドキュメントコメント。
    *   **現状**: `go-scan` はこれらの情報を `scanner.FieldInfo` や `scanner.TypeInfo` を通じて提供しており、充足しています。
*   **型の種類**:
    *   型が構造体、エイリアス、プリミティブ、関数型など、どのような種類であるかの識別。
    *   **現状**: `scanner.TypeInfo.Kind` （`StructKind`, `AliasKind`, `FuncKind` など）で提供されており、充足しています。
*   **複合型（ポインタ、スライス、マップ）の詳細**:
    *   ポインタ型であるか、スライスやマップの要素型は何か、といった情報。
    *   **現状**: `scanner.FieldType` の `IsPointer`, `IsSlice`, `IsMap`, `Elem`, `MapKey` といったフィールドで提供されており、充足しています。
*   **Embeddedフィールドの識別**:
    *   フィールドがEmbeddedであるかどうかの識別。
    *   **現状**: `scanner.FieldInfo.Embedded` で提供されており、充足しています。
*   **フィールド名の正規化サポート**:
    *   変換元と変換先のフィールド名を対応付ける際、大文字・小文字の違いや `_` (アンダースコア) の有無などを吸収するための正規化処理が必要になります。
    *   **現状の不足/検討点**: `go-scan` 自体が正規化関数を提供するか、あるいはジェネレータ側で実装すべき汎用ロジックか。`go-scan` がAST情報を提供するため、ジェネレータ側での実装は可能ですが、一般的な正規化ルール（例: `snake_case` から `CamelCase`）のユーティリティがあれば便利かもしれません。

### 1.2. 型変換ルールの外部定義サポート

型が完全一致しないフィールド間の変換（例: `time.Time` から `string` へ）には、カスタム変換関数を適用する必要があります。

*   **カスタム変換関数のマッピング**:
    *   特定の型ペア（例: `Src:time.Time`, `Dst:string`）に対して、どの変換関数を使用するかを外部からジェネレータに指示できる仕組み。
    *   **現状の不足/検討点**: `go-scan` は型情報を提供しますが、このマッピングルール自体を解釈する機能は持ちません。ジェネレータがこのマッピングをどのように受け取り、解釈するかが課題です。
*   **minigo風DSLによるルール記述**:
    *   ユーザーが変換ルールをminigoのようなドメイン固有言語（DSL）で記述し、ジェネレータがそれをパースしてGoの変換関数呼び出しを生成するアイデアがあります。
    *   **現状の不足/検討点**: `go-scan` はDSLのパーサーやインタープリタを提供するわけではありません。しかし、DSL内で参照される型名やフィールド名を `go-scan` が提供する情報と照合し、検証する際に役立つ可能性があります。

### 1.3. ASTノードへのアクセス

ジェネレータが複雑な変換ロジックやヘルパー関数を生成する際に、元の型定義や関数定義のASTノード情報が必要になることがあります。

*   **現状**: `scanner.TypeInfo.Node` や `scanner.FunctionInfo.AstDecl` などでASTノードへのアクセスが提供されており、充足しています。

## 2. `examples/convert` の実装計画

`examples/convert` ジェネレータは、以下のステップで実装を進めることを計画しています。

### 2.1. 基本方針

`go-scan` を用いて、変換元（Source）と変換先（Destination）のパッケージから型情報を読み取ります。読み取った情報に基づき、一方の型の値をもう一方の型の値に変換するGoの関数群（例: `func ConvertUser(src models.SrcUser) models.DstUser`) を自動生成します。

### 2.2. フィールドマッピング戦略

変換元のフィールドと変換先のフィールドを対応付けるためのルールは以下の通りです。

1.  **フィールド名の正規化**:
    *   まず、変換元と変換先のフィールド名を正規化します。正規化処理として、全て小文字にし、アンダースコア (`_`) を除去する処理を想定します。
    *   例: `FirstName` -> `firstname`, `USER_ID` -> `userid`
    *   正規化後の名前が一致すれば、それらを対応するフィールドとみなします。
2.  **タグによる明示的な指定**:
    *   変換元構造体のフィールドタグに、変換先フィールドの（正規化後の）名前を明示的に指定できるオプションを提供します。
    *   例: `type Src struct { MyField string \`convert:"target_field_name"\` }`
    *   このタグが存在する場合、フィールド名の正規化ルールよりも優先されます。

### 2.3. 型変換戦略

フィールド間で型が異なる場合の変換戦略は以下の通りです。

1.  **型一致**: フィールドの型が完全に一致する場合、単純な代入文を生成します。
2.  **カスタム型変換関数**:
    *   型が異なる場合（例: `time.Time` を `string` に、`*int` を `int` に）、あらかじめ定義された型ペア変換関数を呼び出すコードを生成します。
    *   これらの変換関数は、以下のいずれかの方法で提供されることを想定します。
        *   **ユーザーによるGo関数提供**: ジェネレータの利用者が、特定の型ペア間の変換を行うGo関数を別途記述し、ジェネレータにその情報を（例えば設定ファイルやアノテーション経由で）伝えます。
        *   **minigo風DSLからの生成**: ユーザーが変換ルールをminigoのようなDSLで記述し、ジェネレータがそのDSLを解釈して対応するGoの変換関数（または関数呼び出し部分）を生成します。このDSLは、単純な型キャストから、より複雑なロジック記述までサポートすることを目指します。
            ```minigo
            // Conceptual minigo DSL for type conversion rules
            package converter

            import (
                "example.com/convert/models"
                "time"
                "fmt"
            )

            // Convert time.Time to string using RFC3339 format
            define_converter TimeToString(src time.Time) string {
                return src.Format(time.RFC3339)
            }

            // Convert *time.Time to string, handling nil
            define_converter PtrTimeToString(src *time.Time) string {
                if src == nil {
                    return ""
                }
                return src.Format(time.RFC3339)
            }

            // Convert int64 to string for UserID
            define_converter UserIDToString(src int64) string {
                return fmt.Sprintf("user-%d", src)
            }
            ```
            ジェネレータは、このDSLを読み込み、`TimeToString` や `PtrTimeToString` といった名前のGo関数を生成するか、あるいは直接変換処理のGoコードスニペットをインラインで生成します。

### 2.4. Embeddedフィールドの扱い

変換元または変換先の構造体にEmbeddedフィールドが存在する場合、それらは展開され、フラットなフィールドとして扱われます。`go-scan` から提供される `scanner.FieldInfo.Embedded` フラグを利用して、Embeddedフィールドを正しく識別し処理します。

### 2.5. 生成されるコードの構造

ジェネレータは、以下のような構造のGoの変換関数を生成します。

```go
package converter // Or a user-specified package

import (
	"example.com/convert/models" // Source/Destination models
	"time"                       // If time conversions are needed
	"fmt"                        // If formatting is needed
	// Other necessary imports based on conversion functions
)

// ConvertSrcUserToDstUser converts models.SrcUser to models.DstUser
func ConvertSrcUserToDstUser(ctx context.Context, src models.SrcUser) models.DstUser {
	dst := models.DstUser{}

	// Example: Direct assignment if types match and names (after normalization) match
	// dst.FullName = src.FirstName + " " + src.LastName // (Manual or rule-based combination)

	// Example: Using a custom converter for ID (int64 to string)
	// dst.UserID = UserIDToString(src.ID) // Assuming UserIDToString is generated/defined

	// Example: Handling embedded struct
	// dst.Address = ConvertSrcAddressToDstAddress(ctx, src.Address) // Recursive call

	// Example: Handling slice of structs
	// if src.Details != nil {
	//    dst.Details = make([]models.DstInternalDetail, len(src.Details))
	//    for i, sDetail := range src.Details {
	//        dst.Details[i] = ConvertSrcInternalDetailToDstInternalDetail(ctx, sDetail)
	//    }
	// }

	// ... other field conversions ...

	return dst
}

// Other converters like ConvertSrcAddressToDstAddress, etc.
```

*   **コンテキストの受け渡し**: 各変換関数は `context.Context` を第一引数として受け取ることを推奨します。これにより、タイムアウト制御やリクエストスコープの値の受け渡しなど、高度なユースケースに対応できます。
*   **ネストされた構造体の変換**: ネストされた構造体（例: `SrcUser` 内の `SrcAddress`）は、対応する別の変換関数（例: `ConvertSrcAddressToDstAddress`）を再帰的に呼び出す形で処理されます。
*   **スライスやマップの変換**: スライスやマップの要素も、要素の型に対応する変換処理（プリミティブな代入、または別の変換関数の呼び出し）をループ内で実行することで変換します。

## 3. 懸念事項と今後の課題

*   **フィールド名正規化ルールの複雑性**: 多様な命名規則（camelCase, PascalCase, snake_case, SCREAMING_SNAKE_CASEなど）に対応するための正規化ロジックは複雑になる可能性があります。どこまでを共通ルールとし、どこからをユーザー設定可能にするかのバランスが重要です。
*   **型変換DSLの設計と実装コスト**: minigo風DSLは柔軟性が高い反面、その構文設計、パーサー、ジェネレータへの組み込みには相応の開発コストがかかります。初期段階では、よりシンプルな変換関数指定方法（例: 関数名文字列のマッピング）から始めることも検討できます。
*   **エラーハンドリング**:
    *   変換不可能な型ペアが指定された場合。
    *   マッピングルールに曖昧さや不備があった場合。
    *   実行時の変換エラー（例: 文字列から数値へのパース失敗）。
    これらのエラーをどのように検出し、ユーザーに報告し、生成コード内で処理するかが課題となります。
*   **生成コードのパフォーマンス**: 大量のデータを変換する場合、リフレクションを多用するような наивな実装ではパフォーマンスが問題になる可能性があります。可能な限り静的な型情報に基づいて最適化されたコードを生成することが望ましいです。
*   **循環参照**: 相互に参照し合うような型構造（例: `User` が `Order` を持ち、`Order` が `User` を持つ）の変換では、無限ループに陥らないような工夫が必要です（例: 変換済みのオブジェクトを追跡する）。

これらの計画と懸念事項を踏まえ、`examples/convert` の実装を進めていきます。初期は手動でのコンバータ作成から始め、段階的にジェネレータの機能を拡充していくアプローチを取ります。
