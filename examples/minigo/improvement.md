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

// FunctionDetails は関数の詳細情報を保持します。
type FunctionDetails struct {
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
	// TypeDetails TypeDetails // (将来的に) 型の詳細情報への遅延アクセス用
}

// ReturnInfo は戻り値の情報を保持します。
type ReturnInfo struct {
	Name string // 戻り値名
	Type string // 型名
	// TypeDetails TypeDetails // (将来的に) 型の詳細情報への遅延アクセス用
}

// GetFunctionInfo は指定された関数の詳細情報を返します。
// fn_symbol はminigoの ImportedFunction オブジェクトから変換されたものを想定。
func GetFunctionInfo(fn_symbol interface{}) (FunctionDetails, error) {
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
*   戻り値: 関数の情報を格納した `inspect.FunctionDetails` 構造体のminigoオブジェクトと、エラーオブジェクト。

### 3.2. なぜパッケージ関数か (再掲)

*   **LSP・外部ツールとの親和性**
*   **名前空間の明確化**
*   **Goの慣習との一致**
*   **モジュールとしての管理**

## 4. 代替アプローチとその検討 (内容は変更なし)

(前回のドキュメントのセクション4をそのまま流用)

### 4.1. グローバルな組み込み関数 (不採用)
### 4.2. 特殊な構文の導入 (不採用)
### 4.3. オブジェクトのプロパティ/メソッドアクセス (現時点では不採用)
### 4.4. インターフェースと型アサーションの導入 (不採用)

## 5. 実装が必要となる主な要素 (`minigo/inspect` アプローチの場合)

(前回のドキュメントのセクション5をベースに、戻り値の型変更を反映)

### 5.1. `ImportedFunction` オブジェクト型 (新規) (変更なし)
### 5.2. `evalSelectorExpr` 関数の修正 (変更なし)
### 5.3. `evalCallExpr` 関数の修正 (変更なし)

### 5.4. `minigo/inspect` パッケージ及び関数の実装 (新規・修正)

*   **新規パッケージ**: `minigo/inspect`
    *   利用可能にする方法の検討 (変更なし)
*   **`GetFunctionInfo` 関数**:
    *   **Go実装**: `minigo/inspect/inspect.go` (仮) にGoの関数として実装。
    *   **戻り値**: `inspect.FunctionDetails` 構造体 (Goで定義) と `error`。minigoインタープリタは `FunctionDetails` をminigoのオブジェクト (おそらく専用のstruct様オブジェクトまたはマップ) に変換してスクリプトに返します。
    *   機能 (変更なし、ただし戻り値の型構造が変わる点に注意)
*   **`FunctionDetails`, `ParamInfo`, `ReturnInfo` struct (Go側)**:
    *   上記シグネチャ案で示した構造をGoで定義します。これらのstructはminigoに公開される情報コンテナとなります。

## 6. minigo上で取得可能にすべき情報とその表現

`inspect.GetFunctionInfo` は、`inspect.FunctionDetails` 構造体のインスタンス (minigoオブジェクトに変換されたもの) を返します。

### 6.1. `inspect.FunctionDetails` 構造体

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

// TypeDetails は型の詳細情報を保持します。
type TypeDetails struct {
	Kind     TypeKind    // 型の種類
	Name     string      // 型名 (完全修飾名)
	PkgPath  string      // 型が定義されているパッケージパス
	Doc      string      // 型定義のgodocコメント
	Fields   []FieldInfo // KindがStructの場合のフィールド情報
	Methods  []MethodInfo// KindがInterfaceやStructの場合のメソッド情報 (公開メソッド)
	// ElemType TypeDetails // KindがSlice, Ptr, Array, Mapの場合の要素の型情報 (遅延評価)
	// ... その他、型に応じた情報
}

// FieldInfo はstructのフィールド情報を保持します。
type FieldInfo struct {
	Name string // フィールド名
	Type string // 型名
	Doc  string // フィールドのgodocコメント
	Tag  string // structタグ
	// TypeDetails TypeDetails // (将来的に) 型の詳細情報への遅延アクセス用
}

// MethodInfo はメソッドの情報を保持します (FunctionDetailsと類似の構造)。
type MethodInfo FunctionDetails // 簡単のため FunctionDetails を再利用する案

// GetTypeInfo は指定された型名の詳細情報を返します。
// typeName は "mypkg.MyStruct", "string", "[]int" のような文字列。
func GetTypeInfo(typeName string) (TypeDetails, error) {
	// 実装:
	// 1. typeNameを解析 (go-scanを利用)
	// 2. 型情報をスキャンし、TypeDetails構造体に詰める
	// 3. ElemTypeのような再帰的な部分は、実際にアクセスされるまで評価しない (Lazy Loading)
	//    または、型名だけを保持しておき、再度GetTypeInfoを呼んでもらう形でも良い。
}
```

**Lazy Loadingのコンセプト**: `GetTypeInfo` が呼び出された時点で初めて、`go-scan` を利用して該当の型の詳細情報をスキャン・解析します。これにより、不要な型情報まで先んじて大量にロードすることを防ぎます。一度取得した型情報はキャッシュすることも考えられます。

## 7. 考慮事項・懸念事項 (項目追加・修正)

*   **`go-scan` の機能への依存**: (変更なし)
*   **型情報の詳細度とパース**: (変更なし、ただし `GetTypeInfo` の導入でより詳細な情報取得が可能になる)
*   **再帰的情報取得と循環参照**:
    *   `GetTypeInfo` で型情報を再帰的に辿る際、型定義が互いに参照し合っている場合（例: `type A struct { B *B }; type B struct { A *A }`）に無限ループに陥らないよう、`go-scan` および `minigo/inspect` の実装で検出・対処が必要です（例: 既に処理中の型であればプレースホルダを返す、深さ制限を設けるなど）。
*   **Lazy Loadingの実装**:
    *   `TypeDetails` 内の `ElemType` のような再帰的になる可能性のあるフィールドをどのように遅延評価させるか。関数型フィールドとして持つ、あるいは型名文字列だけを保持し都度 `GetTypeInfo` を呼び出すなどの方法が考えられます。
    *   キャッシュ戦略: 一度取得した型情報をどの程度の期間・範囲でキャッシュするか。
*   **エラーハンドリング**: (変更なし)
*   **ドキュメントコメントの取得**: (変更なし)
*   **`minigo/inspect` パッケージの提供方法**: (変更なし)
*   **minigoオブジェクトへの変換**: Goの `FunctionDetails` や `TypeDetails` structを、minigoスクリプト側で扱いやすいオブジェクト（専用のstruct様オブジェクトまたはマップ）にどのように変換するか。特に `TypeDetails` のようにフィールドが可変になる構造の場合、minigo側での表現方法が課題となります。

## 8. 利用例 (minigoコード) (修正・追加)

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
                fmt.Println("    TypeDetails for", p.Type, ": Kind=", fileInfoType.Kind)
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
    // ...
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
    fmt.Println("\nDetails for type:", myStructInfo.Name)
    fmt.Println("Kind:", myStructInfo.Kind) // "Struct"
    fmt.Println("Fields:")
    for _, f = range myStructInfo.Fields {
        fmt.Println("  Name:", f.Name, ", Type:", f.Type, ", Tag:", f.Tag)
    }
}
```

## 9. 将来的な拡張可能性 (変更なし)

以上が、minigoでインポートされたGo関数の情報を取得するための機能提案です。
ご意見や懸念事項があれば、ぜひお寄せください。
