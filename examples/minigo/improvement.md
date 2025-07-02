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

(セクション内容は変更なし、前回のものをそのまま記載)
### 4.1. グローバルな組み込み関数 (不採用)
### 4.2. 特殊な構文の導入 (不採用)
### 4.3. オブジェクトのプロパティ/メソッドアクセス (現時点では不採用)
### 4.4. インターフェースと型アサーションの導入 (不採用)

## 5. 実装が必要となる主な要素 (`minigo/inspect` アプローチの場合)

(セクション内容は変更なし、前回のものをそのまま記載)
### 5.1. `ImportedFunction` オブジェクト型 (新規)
### 5.2. `evalSelectorExpr` 関数の修正
### 5.3. `evalCallExpr` 関数の修正
### 5.4. `minigo/inspect` パッケージ及び関数の実装 (新規・修正)
    * これには `GetFunctionInfo`, `GetTypeInfo`, `GetPackageInfo` およびそれらが依存するstruct (`FunctionInfo`, `TypeInfo`, `PackageInfo` など) のGo側での定義と、minigoから呼び出すための仕組みが含まれます。

## 6. minigo上で取得可能にすべき情報とその表現

### 6.1. `inspect.FunctionInfo` 構造体 (内容は変更なし)
*   `Name string`, `PkgPath string`, `PkgName string`, `Doc string`, `Params []ParamInfo`, `Returns []ReturnInfo`, `IsVariadic bool`

### 6.2. `inspect.TypeInfo` 構造体 (内容は変更なし、`GetTypeInfo`関数で返される)
*   `Kind TypeKind`, `Name string`, `PkgPath string`, `Doc string`, `Fields []FieldInfo`, `Methods []MethodInfo` など。

### 6.3. `inspect.PackageInfo` 構造体 (`GetPackageInfo`関数で返される)
*   `Name string`: パッケージ名。
*   `Path string`: パッケージのフルインポートパス。
*   `Doc string`: パッケージのgodocコメント。
*   `Imports []string`: このパッケージが直接インポートしているパッケージのパスのリスト。
*   `Constants []ValueInfo`: パッケージレベルでエクスポートされている定数の情報リスト。
    *   `ValueInfo` struct: `Name string`, `Type string`, `Doc string` (Valueは今回は含めない)。
*   `Variables []ValueInfo`: パッケージレベルでエクスポートされている変数の情報リスト。
*   `Functions []string`: パッケージレベルでエクスポートされている関数名のリスト。詳細情報は別途 `inspect.GetFunctionInfo(packagePath + "." + functionName)` で取得。
*   `Types []string`: パッケージレベルで定義・エクスポートされている型名のリスト。詳細情報は別途 `inspect.GetTypeInfo(packagePath + "." + typeName)` で取得。

**Lazy Loadingのコンセプト**: `GetTypeInfo` や `GetPackageInfo` (特にその内部のシンボル詳細) は、呼び出された時点で初めて `go-scan` を利用して情報をスキャン・解析します。これにより、不要な情報まで先んじて大量にロードすることを防ぎます。一度取得した情報はキャッシュすることも考えられます。

## 7. 考慮事項・懸念事項

*   **`go-scan` の機能への依存**: (内容は変更なし)
*   **型情報の詳細度とパース**: (内容は変更なし)
*   **再帰的情報取得と循環参照**: (内容は変更なし)
*   **Lazy Loadingの実装**: (内容は変更なし)
*   **エラーハンドリング**: (内容は変更なし)
*   **ドキュメントコメントの取得**: (内容は変更なし)
*   **`minigo/inspect` パッケージの提供方法**: (内容は変更なし)
*   **minigoオブジェクトへの変換**: (内容は変更なし)
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
    // ...
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

(内容は変更なし)

以上が、minigoでインポートされたGo関数の情報を取得するための機能提案です。
ご意見や懸念事項があれば、ぜひお寄せください。
