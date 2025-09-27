# `derivingjson` の実装から学ぶ `go-scan` の改善点

`examples/derivingjson` は、`go-scan` を利用して、JSON Schema の `oneOf` のような構造を持つ Go の構造体に対して `UnmarshalJSON` メソッドを自動生成するツールです。このツールの実装を分析することで、`go-scan` ライブラリがコードジェネレータにとってさらに便利になるための改善点が見えてきます。

以下に、`derivingjson` の実装を簡略化するために `go-scan` に搭載が検討されるべき機能を提案します。

## 1. アノテーション/コメントベースの型フィルタリング

*   **現状の `derivingjson`**:
    `go-scan` でパッケージ内の全ての型情報を取得した後、ループ処理で各 `TypeInfo` の `Doc` 文字列を調べ、特定のマーカーコメント（`@deriving:unmarshal`）が含まれているかを確認することで、処理対象の構造体をフィルタリングしています。

    ```go
    // examples/derivingjson/generator.go より抜粋
    for _, typeInfo := range pkgInfo.Types {
        if typeInfo.Kind != scanner.StructKind {
            continue
        }
        if typeInfo.Doc == "" || !strings.Contains(typeInfo.Doc, unmarshalAnnotation) {
            continue
        }
        // ... 処理対象の構造体に対するロジック ...
    }
    ```

*   **`go-scan` への提案機能**:
    `go-scan` のスキャン実行時に、特定のGoDocコメントアノテーションを持つ型のみを抽出するオプションを提供します。これにより、クライアント側での手動フィルタリングが不要になり、より効率的に対象の型を取得できます。

    ```go
    // 提案するAPIのイメージ
    // pkgInfo, err := gscn.ScanPackageFromFilePath(pkgPath, goscan.WithDocAnnotation(unmarshalAnnotation))
    // or
    // filteredTypes := pkgInfo.FilterTypes(goscan.WithDocAnnotation(unmarshalAnnotation))
    ```

## 2. フィールドタグの高度な解析とクエリ

*   **現状の `derivingjson`**:
    構造体フィールドのタグ（例: `json:"name,omitempty"`）を解析するために、標準ライブラリの `reflect.StructTag` を利用しています。`json` タグの値を取得し、カンマで区切られたオプション部分を取り除く処理を自前で行っています。また、「識別子フィールド」（例: `json:"type"`）を特定するために、タグのキーと値を直接比較しています。

    ```go
    // examples/derivingjson/generator.go より抜粋
    jsonTag := ""
    if field.Tag != "" {
        var structTag reflect.StructTag = reflect.StructTag(field.Tag)
        jsonTagVal := structTag.Get("json")
        if commaIdx := strings.Index(jsonTagVal, ","); commaIdx != -1 {
            jsonTag = jsonTagVal[:commaIdx]
        } else {
            jsonTag = jsonTagVal
        }
    }
    // ...
    if strings.ToLower(jsonTag) == "type" && isDiscriminatorType {
        // ... 識別子フィールドとしての処理 ...
    }
    ```

*   **`go-scan` への提案機能 (一部実装済み)**:
    `scanner.FieldInfo` に、特定のタグキーに対する値（オプション部分を除いたもの）を簡単に取得できるヘルパーメソッド `TagValue(tagName string) string` が追加されました。これにより、例えば `jsonTagValue := field.TagValue("json")` のようにして `"name"` (omitempty等は除外) を取得できます。
    パース済みのタグ情報を保持する汎用的な構造（例: `map[string]string`）や、特定のタグキーと値を持つフィールドを `TypeInfo` から直接検索する機能は、引き続き有用な提案となります。

    ```go
    // 実装されたAPIの例
    // jsonTagValue := field.TagValue("json") // "name" (omitempty等は除外)

    // 引き続き提案するAPIのイメージ
    // allJsonOptions := field.TagOptions("json") // []string{"omitempty"}
    // discriminatorField := typeInfo.FindFieldWithTagValue("json", "type")
    ```

## 3. インターフェース実装者の効率的な検索

*   **現状の `derivingjson`**:
    `oneOf` の対象となるインターフェース型を特定した後、そのインターフェースを実装している具象型をパッケージ内から見つけるために、再度パッケージ内の全ての `TypeInfo` をループ処理し、`goscan.Implements()` 関数を使用して一つ一つチェックしています。

    ```go
    // examples/derivingjson/generator.go より抜粋
    // oneOfInterfaceDef は事前に特定されたインターフェースの TypeInfo
    for _, structCandidate := range pkgInfo.Types {
        if structCandidate.Kind != scanner.StructKind || structCandidate.Name == typeInfo.Name {
            continue
        }
        if goscan.Implements(structCandidate, oneOfInterfaceDef, pkgInfo) {
            // ... 実装型が見つかった場合の処理 ...
        }
    }
    ```

*   **`go-scan` への提案機能**:
    指定したインターフェースの `TypeInfo` を受け取り、そのインターフェースを実装する型（特に構造体）のリストを、特定のスコープ（例: 同じパッケージ内、またはスキャナが認識している全パッケージ）から効率的に検索するメソッドを `go-scan` または `PackageInfo` に提供します。

    ```go
    // 提案するAPIのイメージ
    // implementers, err := gscn.FindImplementers(oneOfInterfaceDef, pkgInfo)
    // or
    // implementers := pkgInfo.FindImplementers(oneOfInterfaceDef)
    ```

## 4. 型名の正規化/JSONキーへの変換支援

*   **現状の `derivingjson`**:
    具象型名をJSONの識別子の値として使用する際に、`strings.ToLower(structCandidate.Name)` のように単純に小文字に変換しています。

    ```go
    // examples/derivingjson/generator.go より抜粋
    jsonVal := strings.ToLower(structCandidate.Name)
    data.OneOfTypes = append(data.OneOfTypes, OneOfTypeMapping{
        JSONValue: jsonVal,
        GoType:    structCandidate.Name,
    })
    ```

*   **`go-scan` への提案機能**:
    コード生成ツールでは、Goの型名（通常PascalCase）をJSONのキー名（snake_caseやcamelCaseなど）に変換するニーズが頻繁にあります。`go-scan` が、一般的な命名規則への変換ユーティリティを提供するか、`TypeInfo` や `FieldInfo` に一般的な変換結果を保持するプロパティ（例: `NameSnakeCase`, `NameCamelCase`）を含めると便利です。これは必須ではないかもしれませんが、多くのジェネレータで役立つ補助機能となります。

## 5. フィールド型解決の強化と詳細情報

*   **現状の `derivingjson`**:
    フィールドの型情報を扱う際、その型がプリミティブ型か、ローカルパッケージで定義された型か、外部パッケージの型かを判別するために、`field.Type.PkgName == ""` や `field.Type.Resolve()` の結果を組み合わせて複雑な条件分岐を行っています。また、ポインタ、スライス、マップといった複合型の完全な型名を文字列として再構築するロジックも自前で実装しています。
    さらに、型が定義されているパッケージの正確なインポートパスを取得するために、`TypeInfo.FilePath` からディレクトリパスを算出し、再度 `ScanPackageFromFilePath` を呼び出すといった間接的な方法を取る必要がありました。

    ```go
    // examples/derivingjson/generator.go より抜粋
    // 型名の再構築ロジック
    typeNameBuilder := &strings.Builder{}
    currentFieldType := field.Type
    // ... ポインタ、スライス、マップの処理 ...
    actualFieldTypeString := typeNameBuilder.String()

    // ImportPath取得の複雑さ (イメージ)
    // if oneOfInterfaceDef.FilePath != "" {
    //     interfaceDir := filepath.Dir(oneOfInterfaceDef.FilePath)
    //     tempInterfacePkgInfo, _ := gscn.ScanPackageFromFilePath(interfaceDir)
    //     if tempInterfacePkgInfo != nil {
    //          importPath = tempInterfacePkgInfo.ImportPath
    //     }
    // }
    ```

*   **`go-scan` への提案機能**:
    `scanner.FieldType`（または `scanner.TypeRef`）に、より詳細かつ直接的に型情報を取得できるメソッドやプロパティを充実させます。
    *   型の種類を判別するヘルパー: `IsPrimitive()`, `IsPointer()`, `IsSlice()`, `IsMap()`, `IsStruct()`, `IsInterface()` など。
    *   ポインタ、スライス、マップの場合の要素型（`Elem() *TypeRef`）への簡単なアクセス。
    *   型が定義されているパッケージ情報（ローカルか、標準ライブラリか、外部か）を明確に示すフラグやプロパティ。
    *   **型が属するパッケージの完全なインポートパス (`FullImportPath() string`) を `scanner.FieldType` や `scanner.TypeInfo` から直接取得できるメソッド。** (特に重要)
    *   完全な型表現（例: `*[]mypkg.MyType`）を簡単に取得できる `String()` メソッドや `QualifiedName()` メソッドの提供。
    *   `Resolve()` の結果として得られる `TypeInfo` との関係性をより明確にする。`TypeInfo` にも、それが定義されているパッケージの `ImportPath` を保持するフィールドがあると良い。

## 6. 生成コードのインポート管理の支援

*   **現状の `derivingjson`**:
    生成するコードが必要とするパッケージ（例: `encoding/json`, `fmt`、外部パッケージの型）を判断するために、`allFileImports map[string]string` のようなマップを手動で管理し、最終的な出力ファイルにインポート文を自前で構築しています。

    ```go
    // examples/derivingjson/generator.go より抜粋
    // allFileImports[determinedInterfaceImportPath] = interfaceDefiningPkg.Name
    // ...
    // if len(allFileImports) > 0 || needsImportEncodingJson || needsImportFmt {
    //    finalOutput.WriteString("import (\n")
    // ...
    ```

*   **`go-scan` への提案機能**:
    `go-scan` が型情報をスキャン・解決する過程で、登場した型が属するパッケージのインポートパスとその推奨エイリアス（パッケージ名）を収集し、`PackageInfo` や `TypeInfo` を通じてこの情報を提供します。コード生成ツールは、この情報セットを利用して、生成コードに必要なインポート文を自動的かつ正確に構築できます。

    ```go
    // 提案するAPIのイメージ
    // requiredImports := NewImportManager() // or similar utility
    // requiredImports.AddType(field.Type) // field.Type から自動でパッケージパスと名前(エイリアス)を解決
    // generatedImportBlock := requiredImports.Render()
    //
    // または、PackageInfo や TypeInfo が関連するインポートパスのセットを保持する
    // allImports := pkgInfo.GetRequiredImportsForTypes(processedTypes) // map[string]string{ "path": "alias" } のような形式
    ```

## 7. 特定インターフェース実装型の横断的検索 (再掲・強調)

*   **現状の `derivingjson`**:
    `oneOf` の対象となるインターフェース型を特定した後、そのインターフェースを実装している具象型を、関連しそうなパッケージ (`currentSearchPkg`) を推測してスキャンし、その中で `goscan.Implements()` を使ってチェックしています。

*   **`go-scan` への提案機能**: (提案3をより具体的に)
    `goscan.Scanner` に、指定したインターフェースの `TypeInfo` を受け取り、**スキャナがこれまでに解決・スキャンした全てのパッケージの中から**そのインターフェースを実装する型（特に構造体）のリストを効率的に検索するメソッド `FindAllImplementers(interfaceDef *scanner.TypeInfo) []*scanner.TypeInfo` のようなAPIを提供します。これにより、ジェネレータ側は実装型を探すためにパッケージを個別にスキャンしたり、探索範囲を推測したりする必要がなくなります。

## 8. 複数 `oneOf` フィールド対応とジェネレータ設定の強化 (新規)

今回の `derivingjson` の機能拡張（単一構造体内の複数 `oneOf` フィールドサポート）を通じて、さらに以下の点が `go-scan` の改善として考えられました。

*   **現状の `derivingjson`**:
    *   複数のインターフェース型フィールドを処理するために、ジェネレータ内で各フィールドをループし、それぞれに対してインターフェース解決と実装者検索のロジックを繰り返す必要がありました。
    *   JSON内の識別子フィールド名（例: `"type"`）や、各実装型に対応する識別子の値（例: `"dog"`, `"cat"`）の決定ロジックがジェネレータ内にハードコードされているか、単純なルール（型名を小文字化）に依存しています。

*   **`go-scan` への提案機能**:

    *   **より高度なタグ/アノテーション解析の活用 (提案2の拡張)**:
        ジェネレータの振る舞いをフィールドごとに細かく設定できるようなタグ解析機能が求められます。
        例えば、`oneOf` フィールドの識別子フィールド名をタグで指定できるようにします。
        ```go
        // フィールドタグで識別子フィールド名を指定
        // FieldX MyInterface `json:"field_x" oneOf:"discriminatorKey:event_type"`
        // FieldY OtherInterface `json:"field_y" oneOf:"discriminatorKey:payload_kind"`

        // ジェネレータは FieldInfo.TagValue("oneOf", "discriminatorKey") のような形で取得
        ```
        また、実装型側で、自身に対応する識別子の値をタグやメソッドで明示できるようにする支援も考えられます（提案4の拡張）。
        ```go
        // 実装型側での識別子値の明示
        // type Dog struct { ... } `oneOfValue:"doggo"`
        // func (c *Cat) OneOfDiscriminatorValue() string { return "kitty" }
        ```
        `go-scan` がこれらの情報を容易に抽出できるAPI（例: `TypeInfo.TagValue()`, `TypeInfo.MethodValue()`) を提供することで、ジェネレータはより柔軟な設定に対応できます。

    *   **型解決とインポート管理の包括的サポート (提案5, 6の重要性の再確認)**:
        複数の `oneOf` フィールドを持つ構造体を処理する場合、それぞれのインターフェース、その実装型、さらには通常のフィールドの型が、多様な外部パッケージに由来する可能性が高まります。
        ジェネレータがこれらの型の完全修飾名（パッケージエイリアスを含む）を正確に取得し、必要な `import` 文を網羅的かつ重複なく生成するためには、`go-scan` による強力なサポートが不可欠です。
        `TypeInfo.ImportPath()` や `FieldType.QualifiedName()` のようなAPIの充実は、この複雑性を管理する上で極めて重要になります。
        `Scanner.RequiredImportsForTypes(types ...*TypeInfo) map[string]string` のようなユーティリティは、コード生成の最終段階での `import` 文構築を大幅に簡略化します。

    *   **ポインタ型表現の明確化**:
        `oneOf` フィールドに代入される具象型は、通常ポインタ型（例: `s.MyInterfaceField = &MyStructImpl{}`）です。
        `go-scan` が `TypeInfo` や `FieldType` を通じて、型名の文字列表現（例: `*mypkg.MyType`）や、それがポインタであるかどうかの情報を明確に提供できると、ジェネレータは型キャストや変数宣言のコードをより安全かつ正確に生成できます。`FieldType.AsPointerString()` や `FieldType.Elem().QualifiedName()` のようなメソッドが考えられます。

これらの機能が `go-scan` に備わることで、`derivingjson` のようなコードジェネレータは、型の解析、フィルタリング、情報の抽出といった汎用的な処理にかかる手間を大幅に削減し、本来の目的であるコード生成ロジックの開発により集中できるようになるでしょう。
特に、複数の `oneOf` フィールドや、より複雑な設定オプションを持つジェネレータを開発する際に、これらの改善点は大きな助けとなります。
