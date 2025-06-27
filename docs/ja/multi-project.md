# 複数のジェネレータによるコード生成と状態共有

`go-scan` を利用して、単一の型定義やパッケージに対して複数の異なるコード生成処理（例えば、JSONマーシャラ、HTTPリクエストバインダ、バリデータなど）を一度に実行したい場合があります。これは Haskell の `deriving` キーワードが複数の型クラスインスタンスを一度に導出するのに似ています。このようなシナリオでは、各ジェネレータが効率的に型情報を共有し、重複したスキャン処理を避けることが重要になります。

このドキュメントでは、複数のジェネレータ機能を統合して実行するための `main` 関数の設計例と、その中で `go-scan` のスキャン結果（状態）を共有する方法、そしてこのアプローチを実現するために `go-scan` に求められる機能について説明します。

## 設計思想

主な設計思想は以下の通りです。

1.  **スキャン処理の共通化**: パッケージのソースコード解析（ファイル読み込み、AST構築など）はコストの高い処理です。この処理は、対象パッケージ（またはモジュール）ごとに一度だけ実行されるべきです。
2.  **状態の共有**: 一度スキャンされたパッケージの情報 (`scanner.PackageInfo`) は、複数のジェネレータ間で共有されるべきです。
3.  **ジェネレータの独立性**: 各ジェネレータは、他のジェネレータの存在や処理内容を意識することなく、共有された型情報から自身が必要とする情報を抽出・処理できるように設計されるべきです。
4.  **拡張性**: 新しいジェネレータをシステムに容易に追加できるようにします。

## 状態共有メカニズム: `ScanBroker`

この設計を実現するための中核的なコンポーネントとして `ScanBroker`（または `ScanContext`, `SharedScanner`）という概念を導入します。`ScanBroker` は以下の責務を持ちます。

*   `scanner.PackageInfo` のキャッシュ管理: 一度スキャンしたパッケージの結果を保持し、再スキャンを防ぎます。
*   `token.FileSet` の共有: 全てのスキャン処理で単一の `FileSet` を利用し、位置情報の一貫性とメモリ効率を確保します。
*   `scanner.PackageResolver` の提供: `scanner.Scanner` が依存パッケージを解決する際に、ブローカーのキャッシュを経由するようにします。

```go
// (ScanBroker の詳細なコード例は前ステップの「状態共有メカニズムの設計」を参照)
// package goscan // or a new subpackage like goscan/broker
//
// type ScanBroker struct {
// 	 fset         *token.FileSet
// 	 resolver     *BrokerPackageResolver
// 	 scanCache    map[string]*scanner.PackageInfo
// 	 scanOverrides scanner.ExternalTypeOverride
// 	 mu           sync.RWMutex
// }
//
// func NewScanBroker(overrides scanner.ExternalTypeOverride) *ScanBroker { /* ... */ }
// func (b *ScanBroker) GetPackageByDir(ctx context.Context, dirPath string) (*scanner.PackageInfo, error) { /* ... */ }
// // ...その他のメソッド...
//
// type BrokerPackageResolver struct { /* ... */ }
// func (r *BrokerPackageResolver) ScanPackageByImport(ctx context.Context, importPath string) (*scanner.PackageInfo, error) { /* ... */ }
```

## `main` 関数の設計例

以下に、`ScanBroker` を利用して、`derivingjson` (JSON処理系) と `derivingbind` (リクエストバインディング処理系) の両方の機能を一度の実行で処理する `main` 関数の例を示します。

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings" // 例としてJSONGenerator内で使用

	"go/token" // ScanBroker内で必要
	"log/slog" // ScanBroker内で必要 (または標準log)


	// goscan "github.com/your_org/your_repo/goscan" // ScanBrokerを定義したパッケージ
	// scanner "github.com/podhmo/go-scan/scanner" // go-scan本体
	// ※上記はScanBrokerがgo-scanライブラリに統合されるか、別パッケージかで変わる
	// この例では、ScanBrokerと必要な型が適切にインポートされていると仮定します。
	// ここでは仮のScanBroker実装をインラインで示す代わりに、
	// ScanBrokerが提供するインターフェースと利用方法に焦点を当てます。
)

// --- ScanBroker とその関連型の仮定義 (実際は別パッケージからインポート) ---
// (前ステップで設計した ScanBroker, BrokerPackageResolver のコードがここにあると仮定)
// scanner.PackageInfo, scanner.ExternalTypeOverride, scanner.New などの型・関数も
// 適切に利用できる状態とします。
// --- ここまで仮定義 ---


// BaseGenerator defines a common interface for all generators
type BaseGenerator interface {
	Name() string
	Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error
}

// JSONGenerator example
type JSONGenerator struct {
	// No broker needed here if Generate receives PackageInfo directly
}

func NewJSONGenerator() *JSONGenerator {
	return &JSONGenerator{}
}

func (g *JSONGenerator) Name() string { return "JSONGenerator" }

func (g *JSONGenerator) Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error {
	fmt.Printf("[%s] Processing package %s (Path: %s)\n", g.Name(), pkgInfo.Name, pkgInfo.Path)
	for _, typeInfo := range pkgInfo.Types {
		if strings.Contains(typeInfo.Doc, "@deriving:json") { // Simplified check
			fmt.Printf("  [%s] Found target type %s for JSON generation\n", g.Name(), typeInfo.Name)
			// ... actual JSON generation logic ...
			// If types from other packages are involved, their PackageInfo
			// would have been resolved by the broker when pkgInfo was built.
			// typeInfo.Fields[...].Type.Resolve(ctx) would work if needed.
		}
	}
	return nil
}

// BindGenerator example
type BindGenerator struct {
	// No broker needed here
}

func NewBindGenerator() *BindGenerator {
	return &BindGenerator{}
}

func (g *BindGenerator) Name() string { return "BindGenerator" }

func (g *BindGenerator) Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error {
	fmt.Printf("[%s] Processing package %s (Path: %s)\n", g.Name(), pkgInfo.Name, pkgInfo.Path)
	for _, typeInfo := range pkgInfo.Types {
		if strings.Contains(typeInfo.Doc, "@deriving:binding") { // Simplified check
			fmt.Printf("  [%s] Found target type %s for binding generation\n", g.Name(), typeInfo.Name)
			// ... actual binding generation logic ...
		}
	}
	return nil
}


func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: multi-generator <package_directory_path>")
	}
	targetPackageDir := os.Args[1]
	ctx := context.Background()

	// 1. Initialize the ScanBroker
	//    ScanBrokerの実際のインポートパスはプロジェクト構成による
	//    ここでは仮に goscan.NewScanBroker のように呼び出す
	broker := NewScanBroker(nil) // (ScanBrokerのコンストラクta)

	// 2. Register all generators
	generators := []BaseGenerator{
		NewJSONGenerator(),
		NewBindGenerator(),
		// Add other generators here: NewValidateGenerator(), etc.
	}

	// 3. Get the initial PackageInfo for the target directory via the broker
	//    This will perform the actual scan if not already cached for this path.
	slog.InfoContext(ctx, "Starting code generation process", slog.String("targetDir", targetPackageDir))
	pkgInfo, err := broker.GetPackageByDir(ctx, targetPackageDir)
	if err != nil {
		log.Fatalf("Failed to get package info for %s: %v", targetPackageDir, err)
	}

	// 4. Execute each generator with the obtained PackageInfo
	//    All generators receive the *same* PackageInfo instance.
	//    If a generator needs to resolve types from imported packages,
	//    the FieldType.Resolve() method will use the broker's cached resolver,
	//    ensuring those packages are also scanned only once.
	for _, gen := range generators {
		slog.InfoContext(ctx, "Running generator", slog.String("generator", gen.Name()), slog.String("package", pkgInfo.Name))
		if err := gen.Generate(ctx, pkgInfo); err != nil {
			log.Fatalf("Generator %s failed: %v", gen.Name(), err)
		}
		slog.InfoContext(ctx, "Generator finished successfully", slog.String("generator", gen.Name()))
	}

	slog.InfoContext(ctx, "All code generation tasks completed successfully.")
}

// NOTE: The ScanBroker, BrokerPackageResolver, scanner.PackageInfo, etc., would be imported
// from their respective packages. The above `main` assumes they are available.
// The key is that `broker.GetPackageByDir` does the heavy lifting of scanning ONCE,
// and all generators use the resulting `pkgInfo`.
// If `pkgInfo` contains fields whose types are from other packages, `field.Type.Resolve(ctx)`
// will trigger the `BrokerPackageResolver`, which then calls `broker.getPackageByImportPath(ctx, importPath)`,
// thus using the broker's caching mechanism for dependent packages as well.
```

**`main` 関数の処理フロー:**

1.  **`ScanBroker` の初期化**: アプリケーション開始時に `ScanBroker` のインスタンスを一つ作成します。このブローカーが、以降のスキャン処理全体の状態（キャッシュなど）を管理します。
2.  **ジェネレータの登録**: 実行したい全てのジェネレータ（`JSONGenerator`, `BindGenerator`など）のインスタンスを作成し、リストに登録します。各ジェネレータは `BaseGenerator` のような共通インターフェースを実装していると管理しやすいです。
3.  **起点パッケージ情報の取得**: `broker.GetPackageByDir(ctx, targetPackageDir)` を呼び出し、コマンドライン引数などで指定された起点となるパッケージの `scanner.PackageInfo` を取得します。ブローカーは、このパッケージがキャッシュにあればそれを返し、なければ実際にスキャン処理（`scanner.New().ScanPackage()`）を実行して結果をキャッシュし、返します。
4.  **各ジェネレータの実行**: 登録された各ジェネレータの `Generate` メソッドを呼び出し、ステップ3で取得した `PackageInfo` を渡します。
    *   各ジェネレータは、渡された `PackageInfo` を元に、自身が関心のあるアノテーション（例: `@deriving:json`）やタグを持つ型をフィルタリングし、コード生成ロジックを実行します。
    *   もしジェネレータが処理対象の型のフィールドを解析し、そのフィールドの型が別の（インポートされた）パッケージで定義されている場合、その型情報 (`FieldType`) の `Resolve(ctx)` メソッドを呼び出すことで詳細な定義 (`TypeInfo`) を取得できます。この `Resolve` メソッドは `ScanBroker` に提供された `PackageResolver` を利用するため、依存パッケージのスキャンもブローカーのキャッシュを経由して効率的に行われます。

## `go-scan` に求められる機能 (再掲と強調)

この複数ジェネレータ・状態共有アプローチをスムーズに実現するためには、前ステップ「不足機能の洗い出し」で挙げた `go-scan` の機能強化が非常に重要になります。特に以下の機能は、ジェネレータの実装を大幅に簡略化し、堅牢性を高めます。

*   **アノテーション/コメントベースの高度なフィルタリング・抽出**: ジェネレータが `@deriving:xxx` のようなマーカーを簡単に識別・解析できるようにする。
    *   例: `TypeInfo.GetAnnotations("deriving") map[string]string`
*   **フィールドタグの高度な解析とクエリ**: `json:"name,omitempty"` のようなタグ情報を容易に扱えるようにする。
    *   例: `FieldInfo.TagValue("json") string`
*   **インターフェース実装者の効率的な検索**: `oneOf` のようなポリモーフィックな型を扱うジェネレータで必須。
    *   例: `ScanBroker.FindImplementers(ctx, interfaceTypeInfo, scope)`
*   **型解決の強化と詳細情報**: 型の完全修飾名、インポートパス、型の種類（ポインタ、スライス等）の正確な取得。
    *   例: `FieldType.QualifiedName() string`, `TypeInfo.ImportPath() string`
*   **生成コードのインポート管理の支援**: ジェネレータが生成するコードに必要なインポート文を自動的に収集・構築できるようにする。
    *   例: `PackageInfo.GetRequiredImports(types []*TypeInfo) map[string]string`

これらの機能が `go-scan` 本体、あるいは `ScanBroker` のような上位レイヤーで提供されることで、各ジェネレータはボイラープレートコードの記述から解放され、本質的なコード生成ロジックに集中できます。

## まとめ

`ScanBroker` のような状態共有メカニズムを導入し、各ジェネレータがそれを介して型情報を取得するように設計することで、複数のコード生成処理を効率的かつ独立して実行できます。これにより、`go-scan` を基盤とした高度なコード生成フレームワークの構築が促進され、Haskell の `deriving` のような開発体験に近づけることができるでしょう。`go-scan` 自身の機能強化は、このエコシステムをさらに発展させる上で鍵となります。
