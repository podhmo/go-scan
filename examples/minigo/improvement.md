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
// minigo/inspectパッケージの関数としてのイメージ (Go言語側)
package inspect

// FunctionInfo は関数の詳細情報を保持します。
type FunctionInfo struct {
	Name       string      // 関数名
	PkgPath    string      // パッケージのフルパス
	PkgName    string      // パッケージ名
	Doc        string      // godocコメント
	Params     []ParamInfo // 引数情報
	Returns    []ReturnInfo// 戻り値情報
	IsVariadic bool        // 可変長引数か
}

// ParamInfo は引数の情報を保持します。
type ParamInfo struct {
	Name string // 引数名
	Type string // 型名 (例: "string", "mypkg.MyStruct")
}

// ReturnInfo は戻り値の情報を保持します。
type ReturnInfo struct {
	Name string // 戻り値名
	Type string // 型名
}

// GetFunctionInfo は指定された関数の詳細情報を返します。
// fn_symbol はminigoの ImportedFunction オブジェクトから変換されたものを想定。
func GetFunctionInfo(fn_symbol interface{}) (FunctionInfo, error) {
	// ... 実装 ...
}
```

```minigo
// minigoスクリプトからの呼び出しイメージ:
import "minigo/inspect"

info, err = inspect.GetFunctionInfo(mypkg.MyFunction)
if err == nil {
    fmt.Println(info.Name)
    for _, p = range info.Params {
        fmt.Println(p.Name, p.Type)
        // もし p.Type が "mypkg.MyStruct" のような場合、
        // さらなる詳細情報を取得できる (後述の GetTypeInfo)
    }
}
```

*   `fn_symbol`: 情報を取得したいインポート済み関数のシンボル。
*   戻り値: 関数の情報を格納した `inspect.FunctionInfo` 構造体のminigoオブジェクトと、エラーオブジェクト。

### 3.2. なぜパッケージ関数か

*   **LSP・外部ツールとの親和性**: `minigo/inspect` パッケージとその中の `GetFunctionInfo` 関数は、GoのLanguage Server Protocol (LSP) やその他の静的解析ツールから認識されやすくなります。これにより、エディタでのコード補完、シグネチャヘルプ、型チェックなどの恩恵を受けやすくなります。
*   **名前空間の明確化**: 機能が `minigo/inspect` という明確な名前空間に属することで、グローバルな組み込み関数が乱立することを防ぎます。
*   **Goの慣習との一致**: Goの標準ライブラリも、機能の多くをパッケージ内のエクスポートされた関数として提供しており、これに倣う形となります。
*   **モジュールとしての管理**: 将来的に関連機能が増えた場合でも、`minigo/inspect` パッケージ内でまとめて管理できます。

## 4. 代替アプローチとその検討

以下に、情報取得のための他のアプローチと、それらを今回は採用しない理由をまとめます。

### 4.1. グローバルな組み込み関数 (不採用)

*   **検討内容**: 当初案として、minigoのグローバルスコープに `get_function_info()` のような組み込み関数を直接定義することを検討しました。
*   **不採用理由**: LSP（Language Server Protocol）などの外部ツールとの親和性が低く、エディタでの補完や型チェックの恩恵を受けにくい可能性があります。また、グローバル名前空間を汚染する可能性があり、Goの標準的なライブラリ提供方法であるパッケージ経由のアクセスとも異なります。

### 4.2. 特殊な構文の導入 (不採用)

*   **検討内容**: `info(pkg.Function)` のような、情報取得専用の新しい構文キーワードをminigoに導入することを検討しました。
*   **不採用理由**: Goの標準的な構文から逸脱するため、minigoパーサーの複雑化を招き、ユーザーの学習コストも増加させます。「Goらしさ」を損なう可能性も考慮し、採用を見送りました。

### 4.3. オブジェクトのプロパティ/メソッドアクセス (現時点では不採用)

*   **検討内容**: インポートされた関数オブジェクト自体が情報取得のためのプロパティやメソッド（例: `pkg.Function.info` や `pkg.Function.getInfo()`）を持つ形を検討しました。
*   **検討結果**: Goの構造体フィールドアクセス (`foo.Bar`) に似せることは可能ですが、minigoの現在のオブジェクトシステムでは、これを汎用的に実現するには `evalSelectorExpr` の大幅な拡張が必要です。特に、`FunctionInfo` のような動的に取得・生成される情報を「フィールドのように」見せることは、Goの静的なフィールドアクセスとは意味合いが異なります。minigoに実質的なメソッド呼び出しやプロパティアクセスの概念を本格導入することになり、現時点では過剰な複雑化を招く可能性があるため、採用を見送りました。

### 4.4. インターフェースと型アサーションの導入 (不採用)

*   **検討内容**: minigoにインターフェースと型アサーションの仕組みを導入し、それらを使って関数情報オブジェクトから詳細を引き出す方法を検討しました。
*   **不採用理由**: Goにはこれらの強力な概念がありますが、現在のminigoにこれらを本格的に導入するのは非常に大規模な変更となり、minigoの設計のシンプルさとはかけ離れてしまいます。実装コストと複雑性が非常に高いため、採用を見送りました。

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

### 5.4. `minigo/inspect` パッケージ及び関数の実装 (新規・修正)

*   **新規パッケージ**: `minigo/inspect`
    *   **利用可能にする方法の検討**:
        *   案1: 他のGoパッケージと同様に、minigoの `import` 文で解決できるようにする（`GOPATH`やモジュール依存関係で解決）。この場合、`minigo/inspect` は独立したGoモジュールとして提供されるか、minigo本体と同じモジュール内に配置される。
        *   案2: インタプリタに「組み込みパッケージ」として特別に登録する。この場合、`import "minigo/inspect"` はインタープリタによって内部的に処理される。LSP等との連携を考えると、案1の方が望ましい可能性があります。
*   **`GetFunctionInfo` 関数**:
    *   **Go実装**: `minigo/inspect/inspect.go` (仮) にGoの関数として実装します。この関数がminigoの`Object`型を直接扱うか、あるいはminigoの評価器(evaluator)と連携するためのアダプタ層を介して呼ばれる形になります。
    *   **戻り値**: `inspect.FunctionInfo` 構造体 (Goで定義) と `error`。minigoインタープリタは `FunctionInfo` をminigoのオブジェクト (おそらく専用のstruct様オブジェクトまたはマップ) に変換してスクリプトに返します。
    *   **機能詳細**:
        1.  minigoから渡された引数（`ImportedFunction` オブジェクトを期待）を受け取ります。
        2.  引数が期待する型であるか検証します。
        3.  `ImportedFunction` オブジェクトから内部的に保持している `scanner.FunctionInfo` (またはそこから抽出した情報) を取得します。
        4.  取得した情報を、`inspect.FunctionInfo` 構造体（Goで定義）に詰め替えます。
        5.  情報取得に失敗した場合はエラーを返します。
*   **`FunctionInfo`, `ParamInfo`, `ReturnInfo` struct (Go側)**:
    *   上記シグネチャ案で示した構造をGoで定義します。これらのstructはminigoに公開される情報コンテナとなります。

## 6. minigo上で取得可能にすべき情報とその表現

`inspect.GetFunctionInfo` は、`inspect.FunctionInfo` 構造体のインスタンス (minigoオブジェクトに変換されたもの) を返します。

### 6.1. `inspect.FunctionInfo` 構造体

*   `Name string`: 関数名。
*   `PkgPath string`: 関数が属するGoパッケージのフルパス。
*   `PkgName string`: 関数が属するGoパッケージ名。
*   `Doc string`: 関数のgodocコメント。
*   `Params []ParamInfo`: 関数の引数情報のスライス。
    *   `ParamInfo` struct:
        *   `Name string`: 引数名。
        *   `Type string`: 引数の型名。この型名がユーザー定義型(例: `mypkg.MyStruct`)の場合、後述の `inspect.GetTypeInfo` を用いてさらに詳細な型情報を取得できる可能性があります。
*   `Returns []ReturnInfo`: 関数の戻り値情報のスライス。
    *   `ReturnInfo` struct:
        *   `Name string`: 戻り値名。
        *   `Type string`: 戻り値の型名。同様に `inspect.GetTypeInfo` で詳細を取得できる可能性があります。
*   `IsVariadic bool`: 関数が可変長引数を取るかどうか。

### 6.2. 型詳細情報の再帰的・遅延取得: `inspect.GetTypeInfo`

関数の引数や戻り値の型がstructやインターフェースなどの複合型である場合、その型の詳細情報（フィールド、メソッドなど）をさらに取得できると便利です。これを実現するために、`inspect.GetTypeInfo` 関数を提案します。

```go
// minigo/inspectパッケージの追加関数としてのイメージ (Go言語側)
package inspect

// TypeKind は型の種類を示します (例: Struct, Interface, Slice, Map, Basic)。
type TypeKind string

// TypeInfo は型の詳細情報を保持します。
type TypeInfo struct {
	Kind     TypeKind     // 型の種類
	Name     string       // 型名 (完全修飾名)
	PkgPath  string       // 型が定義されているパッケージパス
	Doc      string       // 型定義のgodocコメント
	Fields   []FieldInfo  // KindがStructの場合のフィールド情報
	Methods  []MethodInfo // KindがInterfaceやStructの場合のメソッド情報 (公開メソッド)
	// ElemType *TypeInfo // KindがSlice, Ptr, Array, Mapの場合の要素の型情報 (遅延評価または型名文字列)
	// ... その他、型に応じた情報
}

// FieldInfo はstructのフィールド情報を保持します。
type FieldInfo struct {
	Name string // フィールド名
	Type string // 型名
	Doc  string // フィールドのgodocコメント
	Tag  string // structタグ
}

// MethodInfo はメソッドの情報を保持します (FunctionInfoと類似の構造)。
type MethodInfo FunctionInfo // 簡単のため FunctionInfo を再利用する案

// GetTypeInfo は指定された型名の詳細情報を返します。
// typeName は "mypkg.MyStruct", "string", "[]int" のような文字列。
func GetTypeInfo(typeName string) (TypeInfo, error) {
	// 実装:
	// 1. typeNameを解析 (go-scanを利用)
	// 2. 型情報をスキャンし、TypeInfo構造体に詰める
	// 3. ElemTypeのような再帰的な部分は、実際にアクセスされるまで評価しない (Lazy Loading)
	//    または、型名だけを保持しておき、再度GetTypeInfoを呼んでもらう形でも良い。
}
```

**Lazy Loadingのコンセプト**: `GetTypeInfo` が呼び出された時点で初めて、`go-scan` を利用して該当の型の詳細情報をスキャン・解析します。これにより、不要な型情報まで先んじて大量にロードすることを防ぎます。一度取得した型情報はキャッシュすることも考えられます。

## 7. 考慮事項・懸念事項

*   **`go-scan` の機能への依存**: 本機能の実現は、`go-scan` が `scanner.FunctionInfo` や型情報をどれだけ詳細に提供できるかに強く依存します。特に、型名、引数名、ドキュメントコメント、可変長引数フラグ、structのフィールド、メソッドなどの情報が正確に取得できることが前提となります。
*   **型情報の詳細度とパース**: `go-scan` が提供する型名を基本的にそのままminigo文字列として提供することを想定します。minigo側でこれらの型文字列をさらにパースして構造的な型オブジェクトにするのは、現時点ではスコープ外とします（将来的な拡張可能性はあり）。`mypkg.MyStruct` のようにパッケージプレフィックスが付く型名の場合、そのプレフィックスの扱いも `go-scan` の出力に準じます。
*   **再帰的情報取得と循環参照**: `GetTypeInfo` で型情報を再帰的に辿る際、型定義が互いに参照し合っている場合（例: `type A struct { B *B }; type B struct { A *A }`）に無限ループに陥らないよう、`go-scan` および `minigo/inspect` の実装で検出・対処が必要です（例: 既に処理中の型であればプレースホルダを返す、深さ制限を設けるなど）。
*   **Lazy Loadingの実装**: `TypeInfo` 内の `ElemType` のような再帰的になる可能性のあるフィールドをどのように遅延評価させるか。関数型フィールドとして持つ、あるいは型名文字列だけを保持し都度 `GetTypeInfo` を呼び出すなどの方法が考えられます。キャッシュ戦略（一度取得した型情報をどの程度の期間・範囲でキャッシュするか）も重要です。
*   **エラーハンドリング**: シンボルが見つからない場合、シンボルが期待する型（例: `ImportedFunction`）でない場合、`go-scan` から期待した情報が得られなかった場合など、様々なエラーケースに対応し、`inspect.GetFunctionInfo` や `inspect.GetTypeInfo` は適切なエラーオブジェクトを返す必要があります。
*   **ドキュメントコメントの取得**: `go-scan` が関数宣言や型定義に直接関連付けられたgodocコメントを正確に抽出できることが前提です。
*   **`minigo/inspect` パッケージの提供方法**: minigoユーザーが特別な設定なしに `import "minigo/inspect"` を利用できるように、パッケージの配置場所やビルド方法を考慮する必要があります。minigo本体に同梱する形か、別途 `go get` 可能にするかなどが考えられます。
*   **minigoオブジェクトへの変換**: Goの `FunctionInfo` や `TypeInfo` structを、minigoスクリプト側で扱いやすいオブジェクト（専用のstruct様オブジェクトまたはマップ）にどのように変換するか。特に `TypeInfo` のようにフィールドが可変になる構造（例: `Fields`, `Methods` スライス）や、`ElemType` のような再帰的構造を持つ場合、minigo側での表現方法が課題となります。

## 8. 利用例 (minigoコード)

```minigo
import "os"
import "strings"
import "minigo/inspect"

// os.Getenv 関数の情報を取得
info, err = inspect.GetFunctionInfo(os.Getenv)
if err != nil {
    fmt.Println("Error getting info for os.Getenv:", err)
} else {
    fmt.Println("Function Name:", info.Name)
    fmt.Println("Package Path:", info.PkgPath)
    fmt.Println("Documentation:", info.Doc)
    fmt.Println("Is Variadic:", info.IsVariadic)

    fmt.Println("Parameters:")
    for _, p = range info.Params {
        fmt.Println("  Name:", p.Name, ", Type:", p.Type)
        // p.Type が "mypkg.MyStruct" のような場合、さらに詳細を取得できる
        if p.Type == "os.FileInfo" { // 例: os.FileInfo (実際はインターフェース)
            fileInfoType, errType = inspect.GetTypeInfo(p.Type)
            if errType == nil {
                fmt.Println("    TypeInfo for", p.Type, ": Kind=", fileInfoType.Kind)
                // fileInfoType.Methods などを参照可能
            }
        }
    }

    fmt.Println("Return Values:")
    for _, r = range info.Returns {
        fmt.Println("  Name:", r.Name, ", Type:", r.Type)
    }
}

// strings.Join 関数の情報を取得 (同様に info.FieldName でアクセス)
info2, err2 = inspect.GetFunctionInfo(strings.Join)
if err2 == nil {
    fmt.Println("\nFunction Name:", info2.Name)
    // ... (他のフィールドも同様にアクセス)
}


// 型情報の直接取得の例
// (仮に MyStruct が以下のように定義されているとする)
// package mypkg
// type MyStruct struct {
//     FieldA string
//     FieldB int `json:"field_b"`
// }
myStructInfo, errStruct = inspect.GetTypeInfo("mypkg.MyStruct") // mypkgがスキャン対象にある前提
if errStruct == nil {
    fmt.Println("\nInfo for type:", myStructInfo.Name)
    fmt.Println("Kind:", myStructInfo.Kind) // "Struct"
    fmt.Println("Fields:")
    for _, f = range myStructInfo.Fields {
        fmt.Println("  Name:", f.Name, ", Type:", f.Type, ", Tag:", f.Tag)
    }
}
```

## 9. 将来的な拡張可能性

*   minigoで定義されたユーザー関数 (`UserDefinedFunction`) の情報も同様の仕組みで取得できるようにする。
*   型名だけでなく、型の詳細情報（structのフィールド、インターフェースのメソッドなど）をより深く、かつminigoの型システムと連携した形で取得・操作するための機能。これにはminigoの型システム自体の大きな拡張が必要になる可能性があります。
*   ジェネリクスで定義された関数や型の情報取得への対応（Go本体のジェネリクスサポートが安定し、`go-scan`が対応した場合）。

以上が、minigoでインポートされたGo関数の情報を取得するための機能提案です。
ご意見や懸念事項があれば、ぜひお寄せください。
