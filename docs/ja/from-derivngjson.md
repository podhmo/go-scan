# `derivingjson` の実装から学ぶ `go-scan` の改善点

`examples/derivingjson` は、`go-scan` を利用して、JSON Schema の `oneOf` のような構造を持つ Go の構造体に対して `UnmarshalJSON` メソッドを自動生成するツールです。このツールの実装を分析することで、`go-scan` ライブラリがコードジェネレータにとってさらに便利になるための改善点が見えてきます。

以下に、`derivingjson` の実装を簡略化するために `go-scan` に搭載が検討されるべき機能を提案します。

## 1. アノテーション/コメントベースの型フィルタリング

*   **現状の `derivingjson`**:
    `go-scan` でパッケージ内の全ての型情報を取得した後、ループ処理で各 `TypeInfo` の `Doc` 文字列を調べ、特定のマーカーコメント（`@deriving:unmarshall`）が含まれているかを確認することで、処理対象の構造体をフィルタリングしています。

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
    // pkgInfo, err := gscn.ScanPackage(pkgPath, goscan.WithDocAnnotation(unmarshalAnnotation))
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

*   **`go-scan` への提案機能**:
    `scanner.FieldInfo` に、パース済みのタグ情報を保持する構造（例: `map[string]string`）や、特定のタグキーに対する値（オプション部分を除いたもの）を簡単に取得できるヘルパーメソッドを提供します。さらに、特定のタグキーと値を持つフィールドを `TypeInfo` から直接検索できる機能も有用です。

    ```go
    // 提案するAPIのイメージ
    // jsonTagValue := field.TagValue("json") // "name" (omitempty等は除外)
    // allJsonOptions := field.TagOptions("json") // []string{"omitempty"}
    //
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
    さらに、型が定義されているパッケージの正確なインポートパスを取得するために、`TypeInfo.FilePath` からディレクトリパスを算出し、再度 `ScanPackage` を呼び出すといった間接的な方法を取る必要がありました。

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
    //     tempInterfacePkgInfo, _ := gscn.ScanPackage(interfaceDir)
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

これらの機能が `go-scan` に備わることで、`derivingjson` のようなコードジェネレータは、型の解析、フィルタリング、情報の抽出といった汎用的な処理にかかる手間を大幅に削減し、本来の目的であるコード生成ロジックの開発により集中できるようになるでしょう。
