# 計画: docgen設定における関数・メソッド参照の改善

## 1. 概要

現在の `docgen` のカスタムパターン設定は、関数やメソッドを完全修飾名の文字列 (`Key` フィールド) で指定しています。これはタイプミスを誘発しやすく、IDEのサポート（定義ジャンプ、リファクタリングなど）も受けられません。

本計画では、この指定方法を改善し、`minigo` 設定ファイル内で対象の関数やメソッドを直接参照できるようにします。これにより、開発者体験と設定の堅牢性を向上させます。

**変更前:**
```go
// patterns.go (minigo script)
var Patterns = []patterns.PatternConfig{
	{
		Key:  "github.com/my/pkg/api.MyFunction",
		Type: patterns.RequestBody,
		// ...
	},
	{
		Key:  "(*github.com/my/pkg/api.Client).DoRequest",
		Type: patterns.ResponseBody,
		// ...
	},
}
```

**変更後:**
```go
// patterns.go (minigo script)
package main

import (
    "github.com/my/pkg/api"
    "github.com/podhmo/go-scan/examples/docgen/patterns"
)

// メソッド参照のために型付きのnil変数を宣言
var client *api.Client

var Patterns = []patterns.PatternConfig{
	{
		Fn:   api.MyFunction, // 関数を直接参照
		Type: patterns.RequestBody,
		// ...
	},
	{
		Fn:   client.DoRequest, // メソッドを直接参照
		Type: patterns.ResponseBody,
		// ...
	},
}
```

## 2. 調査結果

- **`minigo` のインポート能力**: `minigo` インタプリタは `goscan` ライブラリを内蔵しており、Goのモジュール解決ルール（`go.mod` 内の `replace` ディレクティブを含む）に従って、実際のGoパッケージをインポート・解決する能力を持っています。これにより、設定スクリプト内で `import "github.com/my/pkg/api"` のような記述が可能です。
- **オブジェクト表現**: `minigo` はGoの関数を `object.GoValue` というラッパーオブジェクトで表現します。このオブジェクトは内部で `reflect.Value` を保持しており、リフレクション経由での呼び出しが可能です。
- **課題：メソッド参照**: 現在の `minigo` には、メソッドそのものを値として表現する仕組み（メソッド式）がありません。特に、`nil` ポインタレシーバに対してメソッド（例：`(*T)(nil).Method`）を解決し、それをオブジェクトとして取得する機能が必要です。
- **課題：キーの動的計算**: 新しい `PatternConfig` 構造体から、`symgo` が内部で利用する文字列ベースのキー（例：`"(*github.com/my/pkg/api.Client).DoRequest"`）を動的に計算するロジックが必要です。

## 3. 実装計画

このタスクは、`minigo` の評価器の拡張と、`docgen` の設定読み込み部分の修正に大別されます。

### ステップ1: `minigo` の拡張 - メソッド式への対応

`minigo` が `(nil).Method` 形式のメソッド式を解決し、それを新しいオブジェクト型として表現できるようにします。

- **`minigo/object/object.go` の修正:**
    1. 新しいオブジェクト型 `GoMethodValue` を定義します。これはメソッドのレシーバ型 (`reflect.Type`) とメソッド自体 (`reflect.Method`) を保持します。
        ```go
        const GO_METHOD_VALUE_OBJ ObjectType = "GO_METHOD_VALUE"

        type GoMethodValue struct {
            ReceiverType reflect.Type
            Method       reflect.Method
        }
        ```
    2. `GoMethodValue` に `Type()` と `Inspect()` メソッドを実装します。

- **`minigo/evaluator/evaluator.go` の修正:**
    1. セレクタ式（`.` 演算子）を評価するロジック (おそらく `evalSelectorExpression` のような関数) を修正します。
    2. レシーバが `object.GoValue` で、その値が `nil` のポインタ型である場合を特別に処理します。
    3. 現在の実装ではパニックする可能性がありますが、これを変更し、レシーバの型情報 (`reflect.Type`) を使ってセレクタ（メソッド名）を検索します。
    4. `reflect.Type.MethodByName()` を使ってメソッドを検索し、見つかった場合は新しく作成した `object.GoMethodValue` を返します。
    5. メソッドが見つからない、あるいはレシーバが `nil` でないポインタや非ポインタ型の場合は、既存のフィールドアクセスやエラー処理ロジックにフォールバックします。

### ステップ2: `docgen` の設定構造体と読み込みロジックの変更

`docgen` が新しい設定フォーマットを解釈できるようにします。

- **`examples/docgen/patterns/patterns.go` の修正:**
    1. `PatternConfig` 構造体を変更します。
        - `Key` フィールドを `key` (unexported) に変更します。
        - `Fn any` フィールドを追加します。
        ```go
        type PatternConfig struct {
            Fn          any
            key         string // unexported, will be computed
            Type        PatternType
            ArgIndex    int
            // ... other fields
        }
        ```
    2. `Pattern` 構造体は `Key` を使い続けるので、変更は不要です。

- **`examples/docgen/loader.go` の修正:**
    1. `convertConfigsToPatterns` 関数を修正します。この関数は `[]PatternConfig` を `[]Pattern` に変換します。
    2. `minigo` から受け取った `[]PatternConfig` をループ処理します。各 `config.Fn` は `minigo/object.Object` 型になっています。
    3. `config.Fn` の型を `switch` で判別します。
        - **case `*object.GoValue`:** `Fn` がGoの関数を表す場合。
            - `reflect.Value` から `runtime.FuncForPC()` を使って関数の完全修飾名を取得し、`result[i].Key` に設定します。
        - **case `*object.GoMethodValue`:** `Fn` がGoのメソッドを表す場合。
            - `GoMethodValue` に保持されている `ReceiverType` と `Method.Name` から `(*pkg.Type).Method` 形式のキー文字列を組み立て、`result[i].Key` に設定します。
        - **default:** サポート外の型が指定された場合はエラーを返します。
    4. `config.key` に計算結果を格納するのではなく、直接 `result[i].Key` に設定します。

### ステップ3: テストの追加と修正

変更が正しく機能することを確認し、リグレッションを防ぐためのテストを実装します。

- **`minigo` のテスト:**
    - 型付き `nil` レシーバからのメソッド参照が正しく `GoMethodValue` オブジェクトを返すことを検証する新しいテストケースを `minigo_test.go` (または関連ファイル) に追加します。
    - `nil` 以外のレシーバに対する既存の動作が壊れていないことを確認します。
- **`docgen` のテスト:**
    - `examples/docgen/main_test.go` に新しいテストケースを追加、または既存のテスト (`TestDocgenWithCustomPatterns`) を拡張します。
    - 新しい `Fn` フィールドを使った設定ファイル (`.go` スクリプト) を `testdata` に作成します。この設定ファイルには、関数参照とメソッド参照の両方を含めます。
    - この設定ファイルを読み込んで `docgen` を実行し、期待通りのOpenAPIドキュメントが生成されることを検証します。これにより、キーの計算とパターンの適用が両方とも正しく機能することを確認します。

## 4. タスクリスト

1.  [ ] **`minigo`**: `object.GoMethodValue` 型を `minigo/object/object.go` に定義する。
2.  [ ] **`minigo`**: `minigo/evaluator/evaluator.go` のセレクタ評価ロジックを修正し、型付き `nil` からのメソッド参照をサポートする。
3.  [ ] **`minigo`**: 上記の変更に対する単体テストを追加する。
4.  [ ] **`docgen`**: `examples/docgen/patterns/patterns.go` の `PatternConfig` 構造体を `Fn` フィールドを持つように変更する。
5.  [ ] **`docgen`**: `examples/docgen/loader.go` の `convertConfigsToPatterns` を修正し、`Fn` フィールドから `Key` 文字列を動的に計算するロジックを実装する。
6.  [ ] **`docgen`**: 新しい設定フォーマットを使用する統合テストを `examples/docgen/main_test.go` に追加する。
7.  [ ] 全てのテストがパスすることを確認する。
8.  [ ] `plan-docgen-minigo-omit-string-key.md` を削除、あるいは更新の完了を報告する。
