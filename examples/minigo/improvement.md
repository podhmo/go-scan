# minigoにおけるインポートされたGo関数の情報取得

## 1. 目的

minigoスクリプト内で、直接インポートした外部Go関数の詳細情報（シグネチャ、ドキュメントなど）を取得可能にすることを目的とします。これにより、minigoプログラムがより動的にGoのコードを理解し、利用するための基盤を提供します。

## 2. 背景

minigoはGoで実装されたインタプリタであり、Goのパッケージをインポートしてその中の定数や関数を利用する機能を持っています。現状では、インポートされた関数を呼び出すことはできますが、その関数がどのような引数を期待し、どのような値を返すのか、あるいはどのようなドキュメントコメントが付与されているのかをminigoスクリプト側から知る標準的な方法がありません。

このような情報にアクセスできることで、以下のようなユースケースが考えられます。

*   動的な関数呼び出しの際の事前チェック
*   関数情報を利用したコード生成やアダプタの自動生成
*   開発者ツールやデバッガでの情報表示

## 3. 提案アプローチ: `minigo/inspect` パッケージによる情報取得

Go言語の構文との整合性、LSP等外部ツールとの親和性、そしてminigoの言語仕様への影響を考慮し、専用パッケージ `minigo/inspect` 内のエクスポートされた関数によってインポートされたGo関数の情報を取得するアプローチを提案します。

### 3.1. `minigo/inspect.GetFunctionInfo` 関数のシグネチャ（案）

```go
// minigo/inspectパッケージの関数としてのイメージ
// package inspect
// func GetFunctionInfo(fn_symbol interface{}) (map[string]interface{}, error)

// minigoスクリプトからの呼び出しイメージ:
// import "minigo/inspect"
// info, err = inspect.GetFunctionInfo(mypkg.MyFunction)
```

*   `fn_symbol`: 情報を取得したいインポート済み関数のシンボル（例: `mypkg.MyFunction` を評価した結果のオブジェクト）。
*   戻り値: 関数の情報を格納したminigoのマップオブジェクトと、エラーオブジェクト。
*   エラー: 対象シンボルが関数でない場合、情報取得に失敗した場合などに返されます。

### 3.2. なぜパッケージ関数か

*   **LSP・外部ツールとの親和性**: `minigo/inspect` パッケージとその中の `GetFunctionInfo` 関数は、GoのLanguage Server Protocol (LSP) やその他の静的解析ツールから認識されやすくなります。これにより、エディタでのコード補完、シグネチャヘルプ、型チェックなどの恩恵を受けやすくなります。
*   **名前空間の明確化**: 機能が `minigo/inspect` という明確な名前空間に属することで、グローバルな組み込み関数が乱立することを防ぎます。
*   **Goの慣習との一致**: Goの標準ライブラリも、機能の多くをパッケージ内のエクスポートされた関数として提供しており、これに倣う形となります。
*   **モジュールとしての管理**: 将来的に関連機能が増えた場合でも、`minigo/inspect` パッケージ内でまとめて管理できます。

## 4. 代替アプローチとその検討

### 4.1. グローバルな組み込み関数 (不採用)

*   **検討**: 当初案として検討されました。
*   **不採用理由**: LSP等外部ツールとの親和性が低く、名前空間も汚染する可能性があるため。Goの慣習にもパッケージ関数の方がより適合します。

### 4.2. 特殊な構文の導入 (不採用)

*   例: `info(pkg.Function)` のような専用キーワード。
*   **不採用理由**: Goの標準的な構文から逸脱し、minigoパーサーの複雑化、学習コストの増加を招くため。

### 4.3. オブジェクトのプロパティ/メソッドアクセス (現時点では不採用)

*   例: `pkg.Function.info` や `pkg.Function.getInfo()`。
*   **検討**: Goの構造体フィールドアクセス (`foo.Bar`) に似せることは可能ですが、minigoの現在のオブジェクトシステムでは、これを汎用的に実現するには `evalSelectorExpr` の大幅な拡張が必要です。`FunctionInfo` のような動的に取得される情報を「フィールドのように」見せることは、Goの静的なフィールドアクセスとは意味合いが異なり、minigoに実質的なメソッド呼び出しやプロパティアクセスの概念を導入することになり、複雑性が増します。
*   **現時点不採用理由**: minigoの言語仕様と実装への影響が大きく、現時点では過剰な複雑化を招く可能性があるため。

### 4.4. インターフェースと型アサーションの導入 (不採用)

*   **検討**: Goにはこれらの概念がありますが、minigoにこれらを本格的に導入するのは非常に大規模な変更となり、現在のminigoのシンプルさとはかけ離れてしまいます。
*   **不採用理由**: 実装コストと複雑性が非常に高いため。

## 5. 実装が必要となる主な要素 (`minigo/inspect` アプローチの場合)

このアプローチを採用する場合、minigoのコア機能および新規パッケージに以下の追加・修正が必要となります。

### 5.1. `ImportedFunction` オブジェクト型 (新規)

*   **場所**: `examples/minigo/object.go`
*   **役割**: インポートされたGo関数の情報を保持するための専用オブジェクト型。
*   **内部データ**: `go-scan/scanner.FunctionInfo` から得られる情報（関数名、パッケージパス、型情報、ドキュメントコメントなど）を格納します。
*   **インターフェース**: `Object` インターフェースを実装 (`Type()`, `Inspect()`)。
    *   `Type()`: `IMPORTED_FUNCTION_OBJ` のような新しいオブジェクトタイプを返します。
    *   `Inspect()`: `<imported function mypkg.MyFunc>` のような文字列を返します。
*   **特性**: このオブジェクトはminigoスクリプト内で直接呼び出すことはできません。呼び出そうとした場合はエラーとなります。

### 5.2. `evalSelectorExpr` 関数の修正

*   **場所**: `examples/minigo/interpreter.go`
*   **修正内容**: `go-scan` を用いて外部パッケージの関数シンボルを解決する際、`UserDefinedFunction` の代わりに上記の `ImportedFunction` オブジェクトを生成し、minigoの実行環境に登録するように変更します。

### 5.3. `evalCallExpr` 関数の修正

*   **場所**: `examples/minigo/interpreter.go`
*   **修正内容**: 呼び出そうとしている関数オブジェクトが `ImportedFunction` 型であった場合、呼び出しはエラーとして処理します（例: 「imported function mypkg.MyFunc cannot be called directly」）。

### 5.4. `minigo/inspect` パッケージ及び `GetFunctionInfo` 関数の実装 (新規)

*   **新規パッケージ**: `minigo/inspect`
    *   このパッケージはGoで実装され、minigoインタープリタから利用可能になる必要があります。
    *   **利用可能にする方法の検討**:
        *   案1: 他のGoパッケージと同様に、minigoの `import` 文で解決できるようにする（`GOPATH`やモジュール依存関係で解決）。この場合、`minigo/inspect` は独立したGoモジュールとして提供されるか、minigo本体と同じモジュール内に配置される。
        *   案2: インタプリタに「組み込みパッケージ」として特別に登録する。この場合、`import "minigo/inspect"` はインタープリタによって内部的に処理される。LSP等との連携を考えると、案1の方が望ましい可能性があります。
*   **`GetFunctionInfo` 関数**:
    *   **Go実装**: `minigo/inspect/inspect.go` (仮) にGoの関数として実装します。この関数がminigoの`Object`型を直接扱うか、あるいはminigoの評価器(evaluator)と連携するためのアダプタ層を介して呼ばれる形になります。
    *   **機能**:
        1.  minigoから渡された引数（`ImportedFunction` オブジェクトを期待）を受け取ります。
        2.  引数が期待する型であるか検証します。
        3.  `ImportedFunction` オブジェクトから内部的に保持している `scanner.FunctionInfo` (またはそこから抽出した情報) を取得します。
        4.  取得した情報を、minigoのマップオブジェクトに相当するGoのデータ構造（例: `map[string]interface{}` で、値はminigoのオブジェクト型に変換可能なもの）に変換して返します。
        5.  情報取得に失敗した場合はエラーを返します。

## 6. minigo上で取得可能にすべき情報とその表現

`inspect.GetFunctionInfo` が返すマップオブジェクトには、以下の情報が含まれることを想定します。

*   **`"name"`**: 関数名 (minigo文字列型)。例: `"MyFunction"`
*   **`"pkgPath"`**: 関数が属するGoパッケージのフルパス (minigo文字列型)。例: `"github.com/user/mypkg"`
*   **`"pkgName"`**: 関数が属するGoパッケージ名 (minigo文字列型)。例: `"mypkg"`
*   **`"doc"`**: 関数のgodocコメント (minigo文字列型)。複数行の場合は改行を含む単一の文字列。コメントがない場合は空文字列。
*   **`"params"`**: 関数の引数に関する情報のリスト (minigoリスト型)。リストの各要素は引数一つ分を表すマップオブジェクトで、以下のキーを持つことを想定します。
    *   `"name"`: 引数名 (minigo文字列型)。名前がない場合（`_`など）は空文字列。
    *   `"type"`: 引数の型名 (minigo文字列型)。例: `"string"`, `"int"`, `"mypkg.MyStruct"`, `"[]*mypkg.OtherType"`, `"map[string]interface{}"`。型名は `go-scan` が提供する形式に基づきます。
*   **`"returns"`**: 関数の戻り値に関する情報のリスト (minigoリスト型)。リストの各要素は戻り値一つ分を表すマップオブジェクトで、`"params"` と同様に以下のキーを持つことを想定します。
    *   `"name"`: 戻り値の名前 (minigo文字列型)。名前がない場合は空文字列。
    *   `"type"`: 戻り値の型名 (minigo文字列型)。
*   **`"isVariadic"`**: 関数が可変長引数を取るかどうか (minigoブール型)。最後の引数が `...T` の形式の場合 `true`。

**マップオブジェクトの例:**

```json
// inspect.GetFunctionInfo(mypkg.SampleFunc) の戻り値のイメージ
{
    "name": "SampleFunc",
    "pkgPath": "github.com/user/mypkg",
    "pkgName": "mypkg",
    "doc": "This is a sample function.\nIt demonstrates variadic arguments and multiple return values.",
    "params": [
        {"name": "count", "type": "int"},
        {"name": "prefix", "type": "string"},
        {"name": "values", "type": "...string"} // or "[]string" and isVariadic: true
    ],
    "returns": [
        {"name": "result", "type": "string"},
        {"name": "", "type": "error"}
    ],
    "isVariadic": true
}
```

## 7. 考慮事項・懸念事項

*   **`go-scan` の `FunctionInfo` への依存**:
    *   本機能の実現は、`go-scan` が `scanner.FunctionInfo` としてどれだけ詳細な情報（型名、引数名、ドキュメントコメント、可変長引数フラグなど）を提供できるかに強く依存します。
    *   特に、struct名、ポインタ、スライス、マップ、関数型、インターフェース型といった複雑な型情報を、`go-scan` がどのような文字列形式で提供するかの確認が不可欠です。
    *   `go-scan` がドキュメントコメントを正確に抽出できるかどうかも重要です。
*   **型情報の詳細度とパース**:
    *   `go-scan` が提供する型名を基本的にそのままminigo文字列として提供することを想定します。minigo側でこれらの型文字列をさらにパースして構造的な型オブジェクトにするのは、現時点ではスコープ外とします（将来的な拡張可能性はあり）。
    *   `mypkg.MyStruct` のようにパッケージプレフィックスが付く型名の場合、そのプレフィックスの扱いも `go-scan` の出力に準じます。
*   **型の循環参照や深いネスト**:
    *   主に `go-scan` 側で対応すべき問題ですが、minigo側でこれらの情報を扱う際にも、無限ループや極端なパフォーマンス低下を招かないよう注意が必要です（今回は文字列ベースなので大きな問題にはなりにくいと予想）。
*   **エラーハンドリング**:
    *   シンボルが見つからない場合。
    *   シンボルが `ImportedFunction` オブジェクトではない場合。
    *   `go-scan` から期待した情報が得られなかった場合。
    *   これらの場合に、`inspect.GetFunctionInfo` は適切なエラーオブジェクトを返す必要があります。
*   **ドキュメントコメントの取得**:
    *   `go-scan` が関数宣言に直接関連付けられたgodocコメントを抽出できることが前提です。
*   **`minigo/inspect` パッケージの提供方法**:
    *   minigoユーザーが特別な設定なしに `import "minigo/inspect"` を利用できるように、パッケージの配置場所やビルド方法を考慮する必要があります。

## 8. 利用例 (minigoコード)

```minigo
import "os" // 例として標準パッケージ "os" をインポート
import "strings"
import "minigo/inspect" // 新しく提案するパッケージ

// os.Getenv 関数の情報を取得
info, err = inspect.GetFunctionInfo(os.Getenv)
if err != nil {
    fmt.Println("Error getting info for os.Getenv:", err)
} else {
    fmt.Println("Function Name:", info["name"])
    fmt.Println("Package Path:", info["pkgPath"])
    fmt.Println("Documentation:", info["doc"])
    fmt.Println("Is Variadic:", info["isVariadic"])

    fmt.Println("Parameters:")
    for _, p = range info["params"] {
        fmt.Println("  Name:", p["name"], ", Type:", p["type"])
    }

    fmt.Println("Return Values:")
    for _, r = range info["returns"] {
        fmt.Println("  Name:", r["name"], ", Type:", r["type"])
    }
}

// strings.Join 関数の情報を取得
info2, err2 = inspect.GetFunctionInfo(strings.Join)
if err2 != nil {
    fmt.Println("Error getting info for strings.Join:", err2)
} else {
    fmt.Println("\nFunction Name:", info2["name"])
    fmt.Println("Documentation:", info2["doc"])
    fmt.Println("Parameters:")
    for _, p = range info2["params"] {
        fmt.Println("  Name:", p["name"], ", Type:", p["type"])
    }
    fmt.Println("Is Variadic:", info2["isVariadic"])
}

// 存在しない関数や、関数でないものを渡した場合 (エラーになる想定)
_, err3 = inspect.GetFunctionInfo(os.PathSeparator) // 定数なのでエラー
if err3 != nil {
    fmt.Println("\nError (as expected) for os.PathSeparator:", err3)
}

// ユーザー定義関数 (現状の提案ではインポートされたGo関数のみ対象)
func myMiniGoFunc(a, b) { return a + b }
// _, err4 = inspect.GetFunctionInfo(myMiniGoFunc) // これはエラーになるか、別途検討
// 現状の提案では ImportedFunction オブジェクトを期待するためエラーになる
```

## 9. 将来的な拡張可能性

*   minigoで定義されたユーザー関数 (`UserDefinedFunction`) の情報も同様の仕組みで取得できるようにする。
*   型名だけでなく、型の詳細情報（structのフィールドなど）を取得するための機能。これにはminigoの型システムの大きな拡張が必要になります。

以上が、minigoでインポートされたGo関数の情報を取得するための機能提案です。
ご意見や懸念事項があれば、ぜひお寄せください。
