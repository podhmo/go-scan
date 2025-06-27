# 複数のジェネレータによるコード生成と状態共有: `go-scan` の進化

## 1. はじめに: 課題意識と目指す姿

Go言語におけるコードジェネレーションは、ボイラープレートコードの削減や型安全性の向上に貢献する強力なテクニックです。`go-scan` ライブラリは、Goのソースコードを静的解析し、型情報を抽出することで、これらのジェネレータ開発の基盤を提供します。

本ドキュメントは、ユーザーとの一連の対話を通じて形成された、`go-scan` を用いた高度なコード生成シナリオに関する考察をまとめたものです。出発点となったのは、以下のテーマでした。

> 「複数のマーカー（アノテーション）を1つのstructに指定し、それぞれに関連するコードを生成したい。例えば、JSON処理用、HTTPリクエストバインディング用など、複数の異なる"関心事"のコードを一度に導出するイメージです。この際、`go-scan` は内部状態（スキャン結果）をどう扱い、どのような機能を提供すれば、各ジェネレータが効率的に動作できるでしょうか？ 各ジェネレータが個別にパッケージ全体を再スキャンするのは避けたいのです。」

この要求は、単一の型定義に対して複数の役割や振る舞いを付与する、Haskellの `deriving` キーワードのコンセプトに強くインスパイアされています。`deriving (Eq, Show, Generic)` のように、複数の型クラスインスタンスを宣言的に導出するかのごとく、Goの型にも複数のコード生成ルールを適用したいというニーズです。

現在の `go-scan` のサンプル (`examples/derivingjson`, `examples/derivingbind`) は、それぞれ単一の目的に特化しています。これらを統合し、あるいは新しいジェネレータを追加して連携させるためには、`go-scan` 自身、およびその利用パターンを進化させる必要があります。

本ドキュメントでは、この「複数マーカー（アノテーション）に基づく複数ジェネレータ」シナリオを実現するための設計思想、状態共有メカニズム、具体的な `main` 関数の構成（逐次実行および並行実行の考慮を含む）、そして `go-scan` に求められる具体的な機能改善について、詳細に論じます。

## 2. 背景と関連技術

### 2.1. 既存ドキュメント (`docs/ja/from-*.md`) からの洞察

リポジトリ内の `docs/ja/from-derivingbind.md` および `docs/ja/from-derivngjson.md` は、個別のジェネレータ開発経験に基づき、`go-scan` への多くの貴重な改善提案を含んでいます。これらの提案は、複数ジェネレータが協調するシナリオにおいて、その重要性が一層高まります。

*   **アノテーション/コメントベースのフィルタリング・抽出**: ジェネレータが処理対象の型を特定するための主要な手段です。現状、各ジェネレータは `TypeInfo.Doc` を手動でパースしていますが、これを `go-scan` がサポートすることで、ロジックの共通化と堅牢性の向上が期待できます。
*   **フィールドタグの高度な解析**: 構造体フィールドのタグ（例: `json:"name,omitempty"`）は、ジェネレータの挙動を細かく制御するためのメタデータです。`reflect.StructTag` を用いた手動パースからの脱却が望まれます。
*   **インターフェース実装者の効率的な検索**: `oneOf` のようなポリモーフィックな型を扱うジェネレータや、特定のインターフェースを実装する型に対して共通処理を行いたい場合に不可欠です。
*   **型解決の強化と詳細情報**: 型の完全修飾名、定義元のインポートパス、ポインタ・スライス・マップ等の複合型の構造に関する正確かつ容易な情報アクセスは、複雑なコード生成ロジックの記述を助けます。
*   **生成コードのインポート管理支援**: 生成コードが必要とするパッケージのインポート文を自動的に収集・管理する機能は、ジェネレータ開発の煩雑さを大幅に軽減します。

これらの機能が `go-scan` に組み込まれることで、ジェネレータ開発者は型情報の「解析」という共通的で複雑なタスクから解放され、本来の「コード生成」ロジックに集中できるようになります。

### 2.2. `go-scan` のコア機能と現状のアーキテクチャ

`scanner/scanner.go` と `scanner/models.go` のコード調査に基づき、`go-scan` の現在の主要なコンポーネントとデータフローは以下のように理解されます。

*   **`Scanner`**: Goソースコードをパースし、型情報を抽出する中心的役割を担います。`token.FileSet` を保持し、`ScanPackage` や `ScanFiles` メソッドを通じて、パッケージ内の型、関数、定数などの情報を集約した `PackageInfo` オブジェクトを生成します。
*   **`PackageInfo`**: 単一パッケージのスキャン結果のコンテナです。複数ジェネレータ間で共有されるべき主要な情報単位となります。
*   **`TypeInfo`, `FieldInfo`, `FieldType`**: それぞれ型定義、構造体フィールド（または関数のパラメータ/リザルト）、フィールドの型に関する詳細情報を格納するモデルです。特に `FieldType` は、`IsPointer`, `IsSlice`, `Elem` などの属性を持ち、`Resolve(ctx context.Context)` メソッドを通じて、`PackageResolver` を介した外部パッケージ型の遅延解決をサポートします。
*   **`PackageResolver`**: `Scanner` がインポート宣言を解決し、他のパッケージの `PackageInfo` を取得（スキャンまたはキャッシュから）するために用いるインターフェースです。

現状の `go-scan` は、指定されたパッケージを一度スキャンし、その結果を `PackageInfo` として提供する能力を持っています。この `PackageInfo` を複数のジェネレータで共有すること自体は可能です。しかし、各ジェネレータがその中から自身が必要とする情報を効率的に抽出し（例えば、特定のアノテーションを持つ型のみを対象とする）、かつ依存関係にある他のパッケージの情報を透過的に解決するためには、より高度なサポート機能が望まれます。

### 2.3. `go vet` / `go/analysis` との比較: 「入り口」の概念

`go vet` や `go/analysis` パッケージのアーキテクチャは、静的解析ツールのフレームワークとして非常に洗練されています。これと比較することで、我々が目指す複数ジェネレータモデルの特徴がより明確になります。

*   **`go/analysis` モデル**:
    *   **単一処理パイプライン的**: `go vet` は、指定されたパッケージ群に対し、登録されたアナライザ群を一連のパイプラインのように適用します。
    *   **「パッケージ」と「アナライザ群」が入り口**: フレームワークが全体の流れを制御し、各アナライザに順次処理の機会を与えます。アナライザは `analysis.Pass` オブジェクトを通じて、その時点でのパッケージ情報や先行アナライザの結果にアクセスします。
    *   **構造化された情報共有**: アナライザ間の依存関係 (`Analyzer.Requires`) と結果共有 (`Pass.ResultOf`) は明確に定義されています。

*   **本提案の `go-scan` 複数ジェネレータモデル**:
    *   **初期スキャンによる「情報の海」**: まず `ScanBroker` が起点パッケージ群をスキャンし、`PackageInfo` という型情報の包括的なデータセット（「情報の海」）を構築・キャッシュします。
    *   **ジェネレータごとの独立した「入り口」**: 各ジェネレータは、この「情報の海」に対し、それぞれ独自の関心事（例:特定のアノテーション）を「入り口」として探索を開始します。例えば、JSONジェネレータは「`@deriving:json` を持つ型」を、Bindジェネレータは「`@deriving:binding` を持つ型」を起点とします。
    *   **自由度の高い探索**: 各ジェネレータは、自身の「入り口」から関連情報を比較的自由に辿れます。

この「入り口」の概念の違いは重要です。`go-scan` モデルは、ジェネレータごとに異なる関心事を起点にできる柔軟性を持つ一方、情報源の一元管理と効率的なアクセスを実現する `ScanBroker` の役割が鍵となります。また、ジェネレータ間に依存関係が生じた場合の処理（後述）は、`go/analysis` のようなDAG解決メカニズムを参考に別途考慮する必要があります。

## 3. 設計目標

複数ジェネレータによる効率的なコード生成と状態共有を実現するために、以下の設計目標を設定します。

1.  **効率性**: パッケージのスキャン（ファイルI/O、AST解析）は、対象パッケージごと（理想的にはモジュールごと）に一度だけにします。
2.  **独立性**: 各ジェネレータは、他のジェネレータの処理内容や関心事に影響されず、自身が必要とする情報のみを取得・処理できるようにします。
3.  **柔軟性**: 新しいジェネレータを容易に追加・統合できるようにします。
4.  **シンプルさ**: `go-scan` および関連ツールの利用者が理解しやすく、使いやすいAPIを提供します。

## 4. 提案: 状態共有メカニズム `ScanBroker`

これらの設計目標を達成するため、中心的な役割を果たす `ScanBroker`（または `ScanContext`, `SharedScanner`）というコンポーネントの導入を提案します。これは、`go-scan` ライブラリの一部として、あるいは `go-scan` を利用する上位のコード生成フレームワークが提供するコンポーネントとして想定されます。

### 4.1. `ScanBroker` の責務

*   **`scanner.PackageInfo` のキャッシュ管理**: `importPath` をキーとして `PackageInfo` をキャッシュし、同一パッケージの再スキャンを防止します。
*   **`token.FileSet` の共有**: 全てのスキャン処理で単一の `FileSet` を利用し、位置情報の一貫性とメモリ効率を向上させます。
*   **`scanner.ExternalTypeOverride` の一元管理**: 全スキャンで共通の型オーバーライド設定を適用可能にします。
*   **Broker-Aware `scanner.PackageResolver` の提供**: `scanner.Scanner` が依存パッケージを解決する際に、ブローカーのキャッシュメカニズムを経由させ、依存パッケージのスキャンも効率化します。

### 4.2. `ScanBroker` の構造（概念コード）
*(このセクションのコードは、前回の提示から大きな変更はありませんが、説明との整合性を高めるために`slog`の利用やコメントを調整しています)*
```go
package goscan // or a new subpackage like goscan/broker

import (
	"context"
	"fmt"
	"go/token"
	"log/slog"
	"path/filepath" // For robust path resolution in a real scenario
	"sync"

	"github.com/podhmo/go-scan/scanner" // Assuming scanner models are here
)

// ScanBroker manages and provides access to scanned package information.
type ScanBroker struct {
	fset          *token.FileSet
	resolver      *BrokerPackageResolver
	scanCache     map[string]*scanner.PackageInfo // Key: canonical import path
	scanOverrides scanner.ExternalTypeOverride
	mu            sync.RWMutex
	// Potentially, a reference to go/packages.Config or similar for robust path resolution
}

// NewScanBroker creates a new ScanBroker.
func NewScanBroker(overrides scanner.ExternalTypeOverride) *ScanBroker {
	fset := token.NewFileSet()
	broker := &ScanBroker{
		fset:          fset,
		scanCache:     make(map[string]*scanner.PackageInfo),
		scanOverrides: overrides,
	}
	broker.resolver = &BrokerPackageResolver{broker: broker}
	return broker
}

// GetPackageByDir is the primary method for generators to request package information.
// It resolves dirPath to a canonical import path and then fetches/scans the package.
func (b *ScanBroker) GetPackageByDir(ctx context.Context, dirPath string) (*scanner.PackageInfo, error) {
	importPath, err := b.resolveDirToImportPath(ctx, dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dir %s to import path: %w", dirPath, err)
	}
	return b.getPackageByImportPath(ctx, importPath, dirPath)
}

// getPackageByImportPath is the core caching/scanning logic.
// initialDirPath is an optional hint if scanning is needed for the first time.
func (b *ScanBroker) getPackageByImportPath(ctx context.Context, importPath string, initialDirPath ...string) (*scanner.PackageInfo, error) {
	b.mu.RLock()
	pkgInfo, found := b.scanCache[importPath]
	b.mu.RUnlock()
	if found {
		slog.DebugContext(ctx, "ScanBroker: Cache hit", slog.String("importPath", importPath))
		return pkgInfo, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if pkgInfo, found = b.scanCache[importPath]; found { // Double-check
		slog.DebugContext(ctx, "ScanBroker: Cache hit (after lock)", slog.String("importPath", importPath))
		return pkgInfo, nil
	}

	slog.InfoContext(ctx, "ScanBroker: Cache miss, attempting to scan", slog.String("importPath", importPath))
	var actualDirPath string
	if len(initialDirPath) > 0 {
		actualDirPath = initialDirPath[0]
	} else {
		resolvedDir, err := b.resolveImportPathToDir(ctx, importPath) // Placeholder
		if err != nil {
			return nil, fmt.Errorf("cannot find directory for import path %s (no initialDirPath provided): %w", importPath, err)
		}
		actualDirPath = resolvedDir
	}

	scn, err := scanner.New(b.fset, b.scanOverrides)
	if err != nil {
		return nil, fmt.Errorf("failed to create scanner for %s: %w", importPath, err)
	}

	scannedPkgInfo, err := scn.ScanPackage(ctx, actualDirPath, b.resolver)
	if err != nil {
		return nil, fmt.Errorf("failed to scan package %s (dir %s): %w", importPath, actualDirPath, err)
	}

	if scannedPkgInfo.ImportPath == "" {
		scannedPkgInfo.ImportPath = importPath
	} else if scannedPkgInfo.ImportPath != importPath {
		slog.WarnContext(ctx, "ScanBroker: Scanned package import path mismatch",
			slog.String("requestedImportPath", importPath),
			slog.String("determinedImportPath", scannedPkgInfo.ImportPath))
	}

	b.scanCache[importPath] = scannedPkgInfo
	slog.InfoContext(ctx, "ScanBroker: Package scanned and cached", slog.String("importPath", importPath))
	return scannedPkgInfo, nil
}

// resolveDirToImportPath converts a directory path to a canonical Go import path. (Placeholder)
func (b *ScanBroker) resolveDirToImportPath(ctx context.Context, dirPath string) (string, error) {
	// This is a CRITICAL and COMPLEX step. Real implementation needed.
	// e.g., using `go list -json -C <dirPath> .`
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return "", fmt.Errorf("abs path for %s: %w", dirPath, err)
	}
	slog.WarnContext(ctx, "ScanBroker: resolveDirToImportPath is using a placeholder (absolute path). Robust import path resolution needed.", slog.String("dirPath", dirPath), slog.String("resolvedKeyForCache", absPath))
	// THIS IS A SIMPLIFICATION. The key should be the CANONICAL import path.
	// For example, if dirPath is "./user", it should resolve to "example.com/project/user".
	return absPath, nil
}

// resolveImportPathToDir converts a canonical import path to a directory path. (Placeholder)
func (b *ScanBroker) resolveImportPathToDir(ctx context.Context, importPath string) (string, error) {
	// This is also CRITICAL and COMPLEX. Real implementation needed.
	// e.g., using `go list -json -f {{.Dir}} <importPath>`
	return "", fmt.Errorf("resolveImportPathToDir: not implemented robustly. Import path '%s' cannot be resolved to a directory without external tools/logic", importPath)
}

// BrokerPackageResolver implements scanner.PackageResolver for the ScanBroker.
type BrokerPackageResolver struct {
	broker *ScanBroker
}

// ScanPackageByImport is called by scanner.Scanner for resolving imported packages.
func (r *BrokerPackageResolver) ScanPackageByImport(ctx context.Context, importPath string) (*scanner.PackageInfo, error) {
	slog.DebugContext(ctx, "BrokerPackageResolver: Request to resolve import", slog.String("importPath", importPath))
	return r.broker.getPackageByImportPath(ctx, importPath) // Pass context
}
```

### 4.3. `ScanBroker` の利点と課題

**利点:**

*   **効率性**: `scanCache` により、各パッケージ（およびその依存パッケージ）のスキャンは一度だけ実行されます。
*   **状態の一元管理**: `FileSet`, `ExternalTypeOverride`, キャッシュされた `PackageInfo` が一箇所で管理され、一貫性が保たれます。
*   **透過的な依存関係解決**: ジェネレータが `FieldType.Resolve()` を呼び出すと、内部的に `ScanBroker` のキャッシュ/スキャン機構が利用され、依存パッケージも効率的に処理されます。

**最大の課題: パス解決**

`ScanBroker` の実用性を左右する最大の課題は、ディレクトリパスとGoの正規インポートパス間の相互変換 (`resolveDirToImportPath`, `resolveImportPathToDir`) の堅牢な実装です。Goモジュールの複雑さ（`replace`ディレクティブ、ワークスペース、ローカルパス、バージョン管理など）を正確に扱うには、`go/packages` ライブラリや `go list` コマンドの機能を内部で利用するか、それらと同等のロジックを実装する必要があります。このドキュメントの概念コードでは、この部分をプレースホルダーとしていますが、実用化にはこの解決が不可欠です。

## 5. `main` 関数の設計例とジェネレータの連携

`ScanBroker` を利用した複数ジェネレータ実行の `main` 関数の構成例を示します。ここでは、ジェネレータの並行実行も考慮に入れます。

```go
package main

import (
	"context"
	"fmt"
	"os"
	"strings" // For simplified annotation checking in examples
	"sync"    // For goroutine synchronization

	"log/slog" // For structured logging

	// Assume ScanBroker and scanner types are properly imported or defined.
	// These would typically come from:
	// scanner "github.com/podhmo/go-scan/scanner"
	// goscan "path/to/your/goscan/brokerpackage" // Where ScanBroker is defined
)

// BaseGenerator defines a common interface for all code generators.
type BaseGenerator interface {
	Name() string
	Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error
}

// --- Example Generators (JSONGenerator, BindGenerator - definitions as before) ---
// (Definitions for JSONGenerator and BindGenerator are omitted for brevity,
//  refer to previous versions of this document or section 5 of the prior response)
// --- Assume JSONGenerator and BindGenerator are defined here ---
type JSONGenerator struct{}
func NewJSONGenerator() *JSONGenerator { return &JSONGenerator{} }
func (g *JSONGenerator) Name() string { return "JSONGenerator" }
func (g *JSONGenerator) Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error {
	slog.InfoContext(ctx, "Running generator", slog.String("name", g.Name()), slog.String("package", pkgInfo.Name))
	for _, typeInfo := range pkgInfo.Types {
		if strings.Contains(typeInfo.Doc, "@deriving:json") { // Placeholder for real annotation parsing
			slog.InfoContext(ctx, fmt.Sprintf("  [%s] Found target type: %s", g.Name(), typeInfo.Name))
			// ... (Actual JSON generation logic) ...
		}
	}
	return nil
}

type BindGenerator struct{}
func NewBindGenerator() *BindGenerator { return &BindGenerator{} }
func (g *BindGenerator) Name() string { return "BindGenerator" }
func (g *BindGenerator) Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error {
	slog.InfoContext(ctx, "Running generator", slog.String("name", g.Name()), slog.String("package", pkgInfo.Name))
	for _, typeInfo := range pkgInfo.Types {
		if strings.Contains(typeInfo.Doc, "@deriving:binding") { // Placeholder
			slog.InfoContext(ctx, fmt.Sprintf("  [%s] Found target type: %s", g.Name(), typeInfo.Name))
			// ... (Actual binding generation logic) ...
		}
	}
	return nil
}


func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		slog.Error("Usage: multi-generator <package_directory_path>")
		os.Exit(1)
	}
	targetPackageDir := os.Args[1]
	ctx := context.Background()

	slog.InfoContext(ctx, "Initializing ScanBroker...")
	// broker := goscan.NewScanBroker(nil) // Assuming NewScanBroker is correctly imported/defined
	broker := NewScanBroker(nil) // Using local definition for this example context

	generators := []BaseGenerator{
		NewJSONGenerator(),
		NewBindGenerator(),
		// Add other generators: NewValidateGenerator(), etc.
	}

	slog.InfoContext(ctx, "Fetching package information via ScanBroker", slog.String("targetDir", targetPackageDir))
	pkgInfo, err := broker.GetPackageByDir(ctx, targetPackageDir)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get package info", slog.String("dir", targetPackageDir), slog.Any("error", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Package information retrieved", slog.String("packageName", pkgInfo.Name), slog.String("importPath", pkgInfo.ImportPath))

	// --- Goroutine-based parallel execution of generators ---
	var wg sync.WaitGroup
	errorChan := make(chan error, len(generators)) // Channel to collect errors from goroutines

	for _, genInstance := range generators {
		wg.Add(1)
		go func(g BaseGenerator) { // Launch a goroutine for each generator
			defer wg.Done()
			slog.InfoContext(ctx, "Starting generator goroutine", slog.String("generator", g.Name()))
			if err := g.Generate(ctx, pkgInfo); err != nil {
				slog.ErrorContext(ctx, "Generator failed in goroutine", slog.String("generator", g.Name()), slog.Any("error", err))
				errorChan <- fmt.Errorf("generator %s failed: %w", g.Name(), err)
				return
			}
			slog.InfoContext(ctx, "Generator goroutine finished successfully", slog.String("generator", g.Name()))
		}(genInstance) // Pass the generator instance to the goroutine
	}

	slog.InfoContext(ctx, "Main goroutine: Waiting for all generators to complete...")
	wg.Wait() // Wait for all launched goroutines to finish
	close(errorChan) // Close the error channel as no more errors will be sent

	var encounteredError bool
	for err := range errorChan {
		if err != nil {
			slog.ErrorContext(ctx, "An error occurred during code generation", slog.Any("error", err))
			encounteredError = true
		}
	}

	if encounteredError {
		slog.ErrorContext(ctx, "One or more generators failed. Exiting.")
		os.Exit(1) // Optional: exit with error code if any generator failed
	}

	slog.InfoContext(ctx, "All code generation tasks processed.")
}
```

**並行実行のポイント:**

1.  **`sync.WaitGroup`**: 各ジェネレータのgoroutineが完了するのを待つために使用。
2.  **Goroutine起動**: 各ジェネレータを個別のgoroutineで実行。ループ変数のクロージャ問題に注意し、インスタンスを正しく渡す。
3.  **エラーハンドリング**: チャネル (`errorChan`) を使用して各goroutineからのエラーを集約し、`main` goroutineで一括して報告・処理。
4.  **`ScanBroker` の役割**: 並行実行時も、各ジェネレータは `pkgInfo` を共有し、型解決などで `ScanBroker` のキャッシュとリゾルバの恩恵を透過的に受けます。これにより、スキャン処理の重複は避けられます。

## 6. `go-scan` への具体的なAPI改善提案

この複数ジェネレータ・状態共有アプローチを効果的に実現するため、`go-scan` に以下の具体的なAPI改善を提案します。これらの提案は、前述の「既存ドキュメントからの洞察」や「`go/analysis`との比較」から得られたニーズに基づいています。

### 6.1. アノテーション処理の強化

*   **`TypeInfo.GetAnnotations(prefix string) (map[string]string, error)`**: Docコメントから指定プレフィックスのアノテーションをキー・バリュー形式で抽出。値のパース（例: `key:"value" subkey:"othervalue"`）もある程度サポートできると理想的。
    *   **動機**: ジェネレータがマーカーとそのパラメータを容易に取得できるようにするため。現状の手動文字列パースを置き換える。
*   **`FieldInfo.GetAnnotations(prefix string) (map[string]string, error)`**: フィールドのDocコメントに対しても同様の機能を提供。
*   **`PackageInfo.FilterTypesByAnnotation(predicate func(typeName string, annotations map[string]string) bool, annotationPrefix string) []*TypeInfo`**: アノテーションに基づいてパッケージ内の型を効率的にフィルタリング。
    *   **動機**: 各ジェネレータが関心のある型（「入り口」）を迅速に見つけられるようにするため。

### 6.2. フィールドタグ解析の強化 (`FieldInfo` メソッド)

*   **`TagValue(key string) (string, bool)`**: 指定したタグキーの主要値を取得 (例: `json:"name,omitempty"` から `"name"` を取得)。
*   **`TagOptions(key string) ([]string, bool)`**: 指定したタグキーのオプション部分を取得 (例: `json:"name,omitempty"` から `["omitempty"]` を取得)。
*   **`TagSubFields(key string) (map[string]string, bool)`**: タグ値が `key:value;key2:value2` のような形式（例: `validate:"required;len:5-50"`）の場合、それをパースしてマップで返す。
    *   **動機**: タグはジェネレータの挙動を指示する重要なメタデータであり、その解析を共通化・単純化するため。

### 6.3. 型解決と型情報アクセスの強化

*   **`FieldType.QualifiedName() string`**: パッケージエイリアスを考慮した完全修飾型名 (例: `pkgalias.MyType`, `string`, `[]*myapp.User`) を返す。
*   **`FieldType.SimpleKind() TypeKind`**: 型の基本的な種類（enum: `Primitive`, `NamedStruct`, `NamedInterface`, `Pointer`, `Slice`, `Map`, `Chan`, `Func`, `Invalid` など）を返す。
*   **`TypeInfo.CanonicalImportPath() string`**: 型が定義されているパッケージの正規インポートパスを返す。
    *   **動機**: ジェネレータが型情報を正確に把握し、生成コード内で型名を正しく参照できるようにするため。
*   **`FieldType.IsStruct() bool`, `IsInterface() bool`, `IsPrimitive() bool` など**: 型が特定のカテゴリに属するかを判定するヘルパーメソッド群。

### 6.4. インポート管理の支援

*   **`ImportTracker` (ユーティリティ型)**
    *   `NewImportTracker(currentPackageImportPath string) *ImportTracker`
    *   `AddType(ft *scanner.FieldType)`: `FieldType` を解析し、必要なインポートパスとパッケージ名を内部で記録（現在のパッケージ自身へのインポートは避ける）。ポインタ、スライス、マップの要素型も再帰的に考慮。
    *   `AddImport(importPath string, alias string)`: 手動でインポートを追加。
    *   `RequiredImports() map[string]string`: 収集したインポートパスと推奨エイリアスのマップを返す (`path -> alias`)。
    *   `RenderBlock() string`: `import (...)` のブロック文字列を生成する。
    *   **動機**: ジェネレータが生成するGoコードに必要なインポート文の管理は煩雑であり、これを自動化することで開発効率を大幅に向上させるため。

### 6.5. `ScanBroker` / `Scanner` による高度な検索機能

*   **`ScanBroker.FindImplementers(ctx context.Context, interfaceTypeInfo *scanner.TypeInfo, searchScope []string) ([]*scanner.TypeInfo, error)`**: 指定されたインターフェース型 (`interfaceTypeInfo`) を実装する型を検索する。`searchScope` は検索対象とするパッケージのインポートパスのリスト（空の場合はスキャン済みの全パッケージなど、範囲を指定可能）。
*   **`ScanBroker.FindTypesWithAnnotation(ctx context.Context, annotationPrefix string, predicate func(typeName string, annotations map[string]string) bool, searchScope []string) ([]*scanner.TypeInfo, error)`**: 指定されたアノテーション条件に合致する型を、指定スコープから検索する。
    *   **動機**: 大規模プロジェクトや多数の依存関係を持つ場合に、ジェネレータが必要な型を効率的に見つけ出せるようにするため。`go/analysis` のように、より広範囲な情報を活用した処理をサポート。

これらの改善提案は、`go-scan` を利用するコードジェネレータの開発者が直面するであろう共通の課題を解決し、ジェネレータの実装をより宣言的かつ堅牢にすることを目指しています。

## 7. より高度なシナリオ: ジェネレータ間の依存関係 (DAG)

ここまでの議論は、各ジェネレータが独立して動作できる、あるいは並行して動作できるという前提に立っていました。しかし、より高度なシナリオとして、**あるジェネレータの生成結果（生成されたGoソースコードやメタデータ）を、別のジェネレータが入力として利用する**ケースが考えられます。

> 「やばいのは生成したものに依存した生成が出てきたときですよねDAGとかになる感じのやつ。」

このご指摘の通り、このような依存関係が存在する場合、単純な並行実行モデルでは対応できません。ジェネレータ間の実行順序を制御し、成果物を適切に受け渡すメカニズムが必要となり、これは**タスク間の依存関係グラフ（DAG: Directed Acyclic Graph）** の解決問題となります。

**考えられるアプローチ:**

1.  **静的な依存関係定義とトポロジカルソート**:
    *   各ジェネレータが、自身が依存する他のジェネレータ（またはそれが生成する特定の「成果物マーカー」）を明示的に宣言します。
    *   実行エンジン（`main`関数や専用のオーケストレータ）は、これらの依存関係からDAGを構築し、トポロジカルソートによって実行可能な順序を決定します。
    *   この順序に基づき、依存関係のないジェネレータ群は並行実行しつつ、依存待ちのジェネレータは先行タスクの完了を待機します。`go/analysis` のアナライザ実行順序決定メカニズムがこれに近いです。

2.  **複数フェーズ実行と再スキャン**:
    *   コード生成プロセス全体を複数のフェーズに分割します。
        *   **フェーズ1**: 依存関係のない、あるいは基本的なコード（例: 型定義、インターフェース定義など）を生成するジェネレータ群を実行。
        *   **フェーズ2**: フェーズ1で生成されたGoソースコードを `ScanBroker` を通じて**再度スキャン**し、型情報を更新・拡充します。この際、`ScanBroker` は新しい情報でキャッシュを更新するか、あるいはフェーズごとのスナップショットを管理する能力が必要になるかもしれません。
        *   **フェーズ3**: 更新・拡充された型情報に基づいて、次の依存ジェネレータ群を実行。
    *   このアプローチは、生成コードがさらなるコード生成の入力となる場合に有効ですが、全体のビルド時間が長くなる可能性や、キャッシュ管理の複雑化が課題となります。

**`go-scan` と `ScanBroker` への影響:**

*   **キャッシュの動的な更新・バージョニング**: 生成コードによる型情報の変更を反映するために、`ScanBroker` のキャッシュが単純な追記だけでなく、更新や無効化、あるいはスナップショットに対応できると理想的です。
*   **成果物の共有メカニズム**: ジェネレータがファイル以外のメタデータ（特定の型に関する構造化情報など）を生成し、それを他のジェネレータが `ScanBroker` や共有コンテキストを通じて安全に取得・利用できる仕組み。

ジェネレータ間の依存関係は、`multi-project` のコンセプトをさらに発展させる上で避けて通れない、しかし非常に強力な機能拡張に繋がるテーマです。

## 8. まとめと今後の展望

`ScanBroker` のような状態共有メカニズムの導入と、本ドキュメントで提案した具体的なAPI群による `go-scan` の機能強化は、Go言語におけるコードジェネレーションの新たな可能性を拓きます。複数のジェネレータが効率的かつ協調的に動作する環境は、Haskellの `deriving` のような強力で宣言的なコード生成体験に近づくための一歩となるでしょう。

初期の「複数のグラフを効率的に探索したい」という問題意識から始まり、`go/analysis` との比較を通じて「入り口」の概念の違いを明確にし、ジェネレータの並行実行、そして最終的にはジェネレータ間の依存関係（DAG）という高度な課題に至るまで、一連の対話は `go-scan` の進化の方向性を具体的に示唆しています。

今後の展望としては、以下の点が挙げられます。

*   **`ScanBroker` のパス解決ロジックの堅牢化**: `go/packages` との連携など、実用的なパス解決機能の実装。
*   **ジェネレータ依存関係解決エンジンの検討**: 上述のDAG解決メカニズムの具体的な設計と実装。
*   **より高度な型システムクエリ言語**: ジェネレータが `PackageInfo` の「情報の海」から必要な情報をより柔軟かつ強力に問い合わせるためのDSLやAPIの提供。
*   **エコシステムの育成**: `go-scan` を基盤とした多様なジェネレータが開発され、共有されるようなコミュニティの形成。

これらの進化を通じて、`go-scan` がGoエコシステムにおける型駆動開発とコードジェネレーションの、さらに強力で不可欠な基盤となることを期待します。
