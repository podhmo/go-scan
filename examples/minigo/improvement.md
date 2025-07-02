# minigoにおけるインポートされたGo関数の情報取得

## 1. 目的

minigoスクリプト内で、直接インポートした外部Goパッケージおよびその要素（関数、型、変数、定数など）の詳細情報を取得可能にすることを目的とします。これにより、minigoプログラムがより動的にGoのコードを理解し、利用するための基盤を提供します。

## 2. 背景

minigoはGoで実装されたインタプリタであり、Goのパッケージをインポートしてその中の定数や関数を利用する機能を持っています。現状では、インポートされた関数を呼び出すことはできますが、その関数がどのような引数を期待し、どのような値を返すのか、あるいはどのようなドキュメントコメントが付与されているのかをminigoスクリプト側から知る標準的な方法がありません。また、パッケージ自体がどのような要素（関数リスト、型リストなど）を公開しているかを網羅的に知ることもできません。

このような情報にアクセスできることで、以下のようなユースケースが考えられます。

*   動的な関数呼び出しの際の事前チェックやシグネチャ検証。
*   パッケージ内の利用可能なシンボルの一覧表示や探索。
*   関数情報や型情報を利用したコード生成やアダプタの自動生成。
*   開発者ツール、ドキュメントジェネレータ、デバッガなどでのリッチな情報表示。

## 3. 提案アプローチ: `minigo/inspect` パッケージによる情報取得

Go言語の構文との整合性、LSP等外部ツールとの親和性、そしてminigoの言語仕様への影響を考慮し、専用パッケージ `minigo/inspect` 内のエクスポートされた関数によって、インポートされたGoの関数、型、そしてパッケージ全体の情報を取得するアプローチを提案します。

このパッケージは主に以下の3つの関数を提供します。

1.  `inspect.GetFunctionInfo(fnSymbol interface{}) (FunctionInfo, error)`: 特定の関数の詳細情報を取得します。
2.  `inspect.GetTypeInfo(typeName string) (TypeInfo, error)`: 特定の型名の詳細情報を取得します。
3.  `inspect.GetPackageInfo(pkgPathOrSymbol interface{}) (PackageInfo, error)`: 特定のパッケージの詳細情報を取得します。

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
    // ...
}
```

### 3.2. `minigo/inspect.GetTypeInfo` 関数のシグネチャ（案）

```go
// minigo/inspectパッケージの追加関数としてのイメージ (Go言語側)
package inspect

// TypeKind は型の種類を示します (例: Struct, Interface, Slice, Map, Basic)。
type TypeKind string

// TypeInfo は型の詳細情報を保持します。
type TypeInfo struct {
	Kind     TypeKind     // 型の種類
	Name     string       // 型名 (完全修飾名, 例: "mypkg.MyStruct", "string")
	PkgPath  string       // 型が定義されているパッケージパス (基本型の場合は空など特別な値)
	Doc      string       // 型定義のgodocコメント
	Fields   []FieldInfo  // KindがStructの場合のフィールド情報
	Methods  []MethodInfo // KindがInterfaceやStructの場合のメソッド情報 (公開メソッド)
	// ElemType *TypeInfo  // KindがSlice, Ptr, Array, Mapの場合の要素の型情報 (遅延評価または型名文字列)
	// ... その他、型に応じた情報 (例: underlying type for defined types)
}

// FieldInfo はstructのフィールド情報を保持します。
type FieldInfo struct {
	Name string // フィールド名
	Type string // 型名
	Doc  string // フィールドのgodocコメント
	Tag  string // structタグ
}

// MethodInfo はメソッドの情報を保持します (FunctionInfoと類似の構造)。
type MethodInfo FunctionInfo // FunctionInfoを再利用

// GetTypeInfo は指定された型名の詳細情報を返します。
func GetTypeInfo(typeName string) (TypeInfo, error) {
	// ... 実装 ...
}
```

### 3.3. `minigo/inspect.GetPackageInfo` 関数のシグネチャ（案）

```go
// minigo/inspectパッケージの追加関数としてのイメージ (Go言語側)
package inspect

// PackageInfo はパッケージの詳細情報を保持します。
type PackageInfo struct {
	Name        string         // パッケージ名 (e.g., "os")
	Path        string         // パッケージのフルパス (e.g., "os", "github.com/user/mypkg")
	Doc         string         // パッケージのgodocコメント
	Imports     []string       // このパッケージがインポートしているパッケージパスのリスト
	Constants   []ValueInfo    // パッケージレベルの定数情報
	Variables   []ValueInfo    // パッケージレベルの変数情報
	Functions   []string       // パッケージレベルの関数名のリスト
	Types       []string       // パッケージで定義されている型名 (struct, interface, etc.)のリスト
}

// ValueInfo は定数や変数の情報を保持します。
type ValueInfo struct {
	Name string // 定数名/変数名
	Type string // 型名
	Doc  string // godocコメント
	// Value string // (可能であれば) 定数の文字列表現
}

// GetPackageInfo は指定されたパッケージの情報を返します。
// pkgPathOrSymbol はパッケージパス文字列またはそのパッケージに属するシンボル。
func GetPackageInfo(pkgPathOrSymbol interface{}) (PackageInfo, error) {
	// ... 実装 ...
}
```

### 3.4. なぜパッケージ関数か

*   **LSP・外部ツールとの親和性**: `minigo/inspect` パッケージとその関数は、GoのLSP等から認識されやすくなります。
*   **名前空間の明確化**: 機能が `minigo/inspect` という明確な名前空間に属します。
*   **Goの慣習との一致**: Goの標準ライブラリの多くがこの形式で機能を提供しています。
*   **モジュールとしての管理**: 関連機能が増えた場合でも `minigo/inspect` パッケージ内で管理できます。

## 4. 代替アプローチとその検討

本提案である `minigo/inspect` パッケージによる情報取得アプローチに至るまでに、いくつかの代替アプローチを検討しました。以下にその概要と、今回は採用を見送った理由を記します。

### 4.1. グローバルな組み込み関数 (不採用)

*   **検討内容**: minigoのグローバルスコープに `get_function_info()` のような組み込み関数を直接定義する案です。
*   **不採用理由**: LSP（Language Server Protocol）などの外部ツールとの親和性が低く、エディタでの補完や型チェックの恩恵を受けにくい可能性があります。また、グローバル名前空間を汚染する可能性があり、Goの標準的なライブラリ提供方法であるパッケージ経由のアクセスとも異なります。最終的に、よりGoらしいアプローチとしてパッケージ関数を選択しました。

### 4.2. 特殊な構文の導入 (不採用)

*   **検討内容**: `info(pkg.Function)` のような、情報取得専用の新しい構文キーワードをminigoに導入する案です。
*   **不採用理由**: Goの標準的な構文から逸脱するため、minigoパーサーの複雑化を招き、ユーザーの学習コストも増加させます。「Goらしさ」を損なう可能性も考慮し、採用を見送りました。

### 4.3. オブジェクトのプロパティ/メソッドアクセス (現時点では不採用)

*   **検討内容**: インポートされた関数オブジェクト自体が情報取得のためのプロパティやメソッド（例: `pkg.Function.info` や `pkg.Function.getInfo()`）を持つ形を検討しました。
*   **検討結果**: Goの構造体フィールドアクセス (`foo.Bar`) に似せることは可能ですが、minigoの現在のオブジェクトシステムでは、これを汎用的に実現するには `evalSelectorExpr` の大幅な拡張が必要です。特に、`FunctionInfo` のような動的に取得・生成される情報を「フィールドのように」見せることは、Goの静的なフィールドアクセスとは意味合いが異なります。minigoに実質的なメソッド呼び出しやプロパティアクセスの概念を本格導入することになり、現時点では過剰な複雑化を招く可能性があるため、採用を見送りました。

### 4.4. インターフェースと型アサーションの導入 (不採用)

*   **検討内容**: minigoにインターフェースと型アサーションの仕組みを導入し、それらを使って関数情報オブジェクトから詳細を引き出す方法を検討しました。
*   **不採用理由**: Goにはこれらの強力な概念がありますが、現在のminigoにこれらを本格的に導入するのは非常に大規模な変更となり、minigoの設計のシンプルさとはかけ離れてしまいます。実装コストと複雑性が非常に高いため、採用を見送りました。

## 5. 実装が必要となる主な要素 (`minigo/inspect` アプローチの場合)

このアプローチを採用する場合、minigoのコア機能および新規パッケージに以下の追加・修正が必要となります。

### 5.1. `ImportedFunction` オブジェクト型

*   **場所**: `examples/minigo/object.go`
*   **役割**: インポートされたGo関数の情報を保持するための専用オブジェクト型。minigoインタープリタが `inspect.GetFunctionInfo` に渡す内部的な表現となります。
*   **内部データ**: `go-scan/scanner.FunctionInfo` から得られる情報、またはそれを参照するためのキーを格納します。
*   **インターフェース**: `Object` インターフェースを実装 (`Type()`, `Inspect()`)。
    *   `Type()`: `IMPORTED_FUNCTION_OBJ` のような新しいオブジェクトタイプを返します。
    *   `Inspect()`: `<imported function mypkg.MyFunc>` のような文字列を返します。
*   **特性**: このオブジェクトはminigoスクリプト内で直接呼び出すことはできません。呼び出そうとした場合はエラーとなります。

### 5.2. `evalSelectorExpr` 関数の修正

*   **場所**: `examples/minigo/interpreter.go`
*   **修正内容**: `go-scan` を用いて外部パッケージの関数シンボルを解決する際、`UserDefinedFunction` の代わりに上記の `ImportedFunction` オブジェクトを生成し、minigoの実行環境に登録するように変更します。これにより、`inspect.GetFunctionInfo` にそのシンボルを渡せるようになります。

### 5.3. `evalCallExpr` 関数の修正

*   **場所**: `examples/minigo/interpreter.go`
*   **修正内容**: 呼び出そうとしている関数オブジェクトが `ImportedFunction` 型であった場合、呼び出しはエラーとして処理します（例: 「imported function mypkg.MyFunc cannot be called directly」）。

### 5.4. `minigo/inspect` パッケージ及び関数の実装

*   **新規パッケージ**: `minigo/inspect` をGoで実装します。
    *   **利用可能にする方法の検討**:
        *   案1: 他のGoパッケージと同様に、minigoの `import` 文で解決できるようにする（`GOPATH`やモジュール依存関係で解決）。この場合、`minigo/inspect` は独立したGoモジュールとして提供されるか、minigo本体と同じモジュール内に配置される。
        *   案2: インタプリタに「組み込みパッケージ」として特別に登録する。この場合、`import "minigo/inspect"` はインタープリタによって内部的に処理される。LSP等との連携を考えると、案1の方が望ましい可能性があります。
*   **`GetFunctionInfo`, `GetTypeInfo`, `GetPackageInfo` 関数**:
    *   **Go実装**: `minigo/inspect/inspect.go` (仮) にGoの関数として実装します。これらの関数は、minigoから渡される引数（`ImportedFunction`オブジェクトや型名文字列、パッケージパス文字列など）を解釈し、`go-scan` を利用して情報を収集し、定義された `FunctionInfo`, `TypeInfo`, `PackageInfo` 構造体に詰めて返します。
    *   **minigoへの公開**: minigoインタープリタがこれらのGo関数を呼び出し、結果をminigoのオブジェクト（専用のstruct様オブジェクトまたはマップ）に変換してスクリプトに返す仕組みが必要です。
*   **各種 `xxxInfo` struct (Go側)**:
    *   `FunctionInfo`, `ParamInfo`, `ReturnInfo`, `TypeInfo`, `FieldInfo`, `MethodInfo`, `PackageInfo`, `ValueInfo` などの構造体をGoで定義します。これらがminigoに公開される情報のスキーマとなります。

## 6. minigo上で取得可能にすべき情報とその表現

`minigo/inspect` パッケージの各関数が返す情報構造について説明します。

### 6.1. `inspect.FunctionInfo` 構造体

`inspect.GetFunctionInfo` によって返される、個々の関数の詳細情報です。
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

### 6.2. `inspect.TypeInfo` 構造体

`inspect.GetTypeInfo` によって返される、特定の型の詳細情報です。
*   `Kind TypeKind`: 型の種類を示します (例: `Struct`, `Interface`, `Slice`, `Map`, `Basic`)。
*   `Name string`: 型名 (完全修飾名, 例: `"mypkg.MyStruct"`, `"string"`)。
*   `PkgPath string`: 型が定義されているパッケージパス (基本型の場合は空など特別な値)。
*   `Doc string`: 型定義のgodocコメント。
*   `Fields []FieldInfo`: `Kind`が`Struct`の場合のフィールド情報。
    *   `FieldInfo` struct: `Name string`, `Type string`, `Doc string`, `Tag string`。
*   `Methods []MethodInfo`: `Kind`が`Interface`や`Struct`の場合のメソッド情報 (公開メソッド)。
    *   `MethodInfo` struct: `FunctionInfo` と同様の構造を想定。
*   `ElemType *TypeInfo`: `Kind`が`Slice`, `Ptr`, `Array`, `Map`の場合の要素の型情報。遅延評価されるか、型名文字列を保持し、必要に応じて再度 `GetTypeInfo` で解決します。
*   その他、型の種類に応じた情報（例: defined typeの underlying typeなど）。

### 6.3. `inspect.PackageInfo` 構造体

`inspect.GetPackageInfo`関数で返される、パッケージ全体の情報です。
*   `Name string`: パッケージ名。
*   `Path string`: パッケージのフルインポートパス。
*   `Doc string`: パッケージのgodocコメント。
*   `Imports []string`: このパッケージが直接インポートしているパッケージのパスのリスト。
*   `Constants []ValueInfo`: パッケージレベルでエクスポートされている定数の情報リスト。
    *   `ValueInfo` struct: `Name string`, `Type string`, `Doc string`。
*   `Variables []ValueInfo`: パッケージレベルでエクスポートされている変数の情報リスト。
*   `Functions []string`: パッケージレベルでエクスポートされている関数名のリスト。詳細情報は別途 `inspect.GetFunctionInfo(packagePath + "." + functionName)` で取得することを推奨します。
*   `Types []string`: パッケージレベルで定義・エクスポートされている型名のリスト。詳細情報は別途 `inspect.GetTypeInfo(packagePath + "." + typeName)` で取得することを推奨します。

**Lazy Loadingのコンセプト**: `GetTypeInfo` や `GetPackageInfo` (特にその内部のシンボル詳細) は、呼び出された時点で初めて `go-scan` を利用して情報をスキャン・解析します。これにより、不要な情報まで先んじて大量にロードすることを防ぎます。一度取得した情報はキャッシュすることも考えられます。

## 7. 考慮事項・懸念事項

*   **`go-scan` の機能への依存**: 本機能の実現は、`go-scan` が `scanner.FunctionInfo` や型情報、パッケージ情報をどれだけ詳細かつ正確に提供できるかに強く依存します。特に、型名、引数名、ドキュメントコメント、可変長引数フラグ、structのフィールド、メソッド、パッケージ内のシンボルリストなどの情報が正確に取得できることが前提となります。
*   **型情報の詳細度とパース**: `go-scan` が提供する型名を基本的にそのままminigo文字列として提供することを想定します。minigo側でこれらの型文字列をさらにパースして構造的な型オブジェクトにするのは、現時点ではスコープ外とします（将来的な拡張可能性はあり）。`mypkg.MyStruct` のようにパッケージプレフィックスが付く型名の場合、そのプレフィックスの扱いも `go-scan` の出力に準じます。
*   **再帰的情報取得と循環参照**: `GetTypeInfo` で型情報を再帰的に辿る際、型定義が互いに参照し合っている場合（例: `type A struct { B *B }; type B struct { A *A }`）に無限ループに陥らないよう、`go-scan` および `minigo/inspect` の実装で検出・対処が必要です（例: 既に処理中の型であればプレースホルダを返す、深さ制限を設けるなど）。
*   **Lazy Loadingの実装**: `TypeInfo` 内の `ElemType` のような再帰的になる可能性のあるフィールドをどのように遅延評価させるか。関数型フィールドとして持つ、あるいは型名文字列だけを保持し都度 `GetTypeInfo` を呼び出すなどの方法が考えられます。キャッシュ戦略（一度取得した型情報をどの程度の期間・範囲でキャッシュするか）も重要です。
*   **エラーハンドリング**: シンボルが見つからない場合、シンボルが期待する型（例: `ImportedFunction`）でない場合、`go-scan` から期待した情報が得られなかった場合など、様々なエラーケースに対応し、`inspect.GetFunctionInfo`, `inspect.GetTypeInfo`, `inspect.GetPackageInfo` は適切なエラーオブジェクトを返す必要があります。
*   **ドキュメントコメントの取得**: `go-scan` が関数宣言、型定義、パッケージ宣言に直接関連付けられたgodocコメントを正確に抽出できることが前提です。
*   **`minigo/inspect` パッケージの提供方法**: minigoユーザーが特別な設定なしに `import "minigo/inspect"` を利用できるように、パッケージの配置場所やビルド方法を考慮する必要があります。minigo本体に同梱する形か、別途 `go get` 可能にするかなどが考えられます。
*   **minigoオブジェクトへの変換**: Goの `FunctionInfo`, `TypeInfo`, `PackageInfo` structを、minigoスクリプト側で扱いやすいオブジェクト（専用のstruct様オブジェクトまたはマップ）にどのように変換するか。特に `TypeInfo` のようにフィールドが可変になる構造（例: `Fields`, `Methods` スライス）や、`ElemType` のような再帰的構造を持つ場合、minigo側での表現方法が課題となります。
*   **パッケージ全体の情報スキャンのパフォーマンス**: `GetPackageInfo` はパッケージ内の多数の要素をスキャンする可能性があるため、特に大規模なパッケージに対して呼び出された場合のパフォーマンス影響を考慮する必要があります。`go-scan` の効率に大きく依存します。
*   **シンボルからのパッケージ特定**: `GetPackageInfo` にパッケージ内のシンボル (例: `os.Getenv`) を渡した場合、そのシンボルから所属パッケージ (例: `"os"`) を特定する内部メカニズムが必要です。

## 8. 利用例 (minigoコード)

```minigo
import "os"
import "strings"
import "minigo/inspect"

// GetFunctionInfo の利用例
funcInfo, err = inspect.GetFunctionInfo(os.Getenv)
if err == nil {
    fmt.Println("Function:", funcInfo.Name, ", Package:", funcInfo.PkgName)
    fmt.Println("  Doc:", funcInfo.Doc)
    fmt.Println("  Params:")
    for _, p = range funcInfo.Params {
        fmt.Println("    -", p.Name, p.Type)
    }
    // ... (Returnsなども同様にアクセス)
}

// GetTypeInfo の利用例
fileInfoType, err = inspect.GetTypeInfo("os.FileInfo") // os.FileInfoはインターフェース
if err == nil {
    fmt.Println("\nType:", fileInfoType.Name, ", Kind:", fileInfoType.Kind)
    fmt.Println("  Doc:", fileInfoType.Doc)
    if fileInfoType.Kind == "Interface" {
        fmt.Println("  Methods:")
        for _, m = range fileInfoType.Methods {
            fmt.Println("    -", m.Name) // MethodInfoはFunctionInfoと同じ構造
        }
    }
}

// GetPackageInfo の利用例
pkgInfo, err = inspect.GetPackageInfo("strings") // パッケージパスで指定
// または pkgInfo, err = inspect.GetPackageInfo(strings.Join) // パッケージ内のシンボルで指定
if err == nil {
    fmt.Println("\nPackage:", pkgInfo.Name, "(Path:", pkgInfo.Path, ")")
    fmt.Println("  Doc:", pkgInfo.Doc)
    fmt.Println("  Imports:", pkgInfo.Imports)

    fmt.Println("  Constants:")
    for _, c = range pkgInfo.Constants {
        fmt.Println("    -", c.Name, ":", c.Type)
    }
    fmt.Println("  Variables:")
    for _, v = range pkgInfo.Variables {
        fmt.Println("    -", v.Name, ":", v.Type)
    }
    fmt.Println("  Functions:")
    for _, fName = range pkgInfo.Functions {
        fmt.Println("    -", fName)
        // fInfo, _ := inspect.GetFunctionInfo(pkgInfo.Path + "." + fName) // 詳細取得
    }
    fmt.Println("  Types:")
    for _, tName = range pkgInfo.Types {
        fmt.Println("    -", tName)
        // tInfo, _ := inspect.GetTypeInfo(pkgInfo.Path + "." + tName) // 詳細取得
    }
}
```

## 9. 将来的な拡張可能性

この `minigo/inspect` パッケージは、将来的に以下のような方向へ拡張できる可能性があります。

*   **minigoユーザー定義関数のイントロスペクション**: 現在はインポートされたGo関数が主な対象ですが、minigoスクリプト内で定義された関数 (`UserDefinedFunction`) の情報も同様の仕組みで取得できるように拡張できます。
*   **より詳細な型情報**: 型名だけでなく、型の詳細情報（structのフィールドのタグ、埋め込みフィールド、インターフェースが埋め込んでいる他のインターフェースなど）をより深く、かつminigoの型システムと連携した形で取得・操作するための機能。これにはminigoの型システム自体の大きな拡張が必要になる可能性があります。
*   **ジェネリクス対応**: Go本体のジェネリクスサポートが安定し、`go-scan`がジェネリクスで定義された関数や型の情報を適切に提供できるようになった場合、`minigo/inspect` もこれに対応することが期待されます。
*   **コード位置情報**: 各情報（関数定義、型定義、フィールド定義など）がソースコード上のどこで定義されているか（ファイルパス、行番号）といった情報を追加することも有用です。
*   **ASTノードへのアクセス**: （上級者向け機能として）`go-scan` が提供するASTノードへの限定的なアクセスを提供し、より低レベルなコード解析をminigoから行えるようにする可能性も考えられます。

以上が、minigoでインポートされたGo関数の情報を取得するための機能提案です。
ご意見や懸念事項があれば、ぜひお寄せください。
