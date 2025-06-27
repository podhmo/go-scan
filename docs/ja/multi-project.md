# 複数のジェネレータによるコード生成と状態共有: `go-scan` の進化

## 1. はじめに: 課題意識と目指す姿

Go言語におけるコードジェネレーションは、ボイラープレートコードの削減や型安全性の向上に貢献する強力なテクニックです。`go-scan` ライブラリは、Goのソースコードを静的解析し、型情報を抽出することで、これらのジェネレータ開発の基盤を提供します。

本ドキュメントは、ユーザーからの以下の問いかけを出発点としています。

> 「複数のマーカーを1つのstructに指定し辿るような処理があると考えてください。このときのgo-scan内部の状態の扱いや機能について考えてください。2つの大きなグラフが存在するイメージです。それぞれで個別に探索し直すということは避けたいです。」

これは、単一の型定義に対して、複数の異なる目的のコード（例えば、JSONマーシャリング用、HTTPリクエストバインディング用、バリデーション用など）を一度に生成したい、という高度な要求を示唆しています。このイメージは、Haskellの `deriving` キーワードが、一つの型定義から `Eq`, `Show`, `Generic` といった複数の型クラスインスタンスを自動的に導出する機能に類似しています。

現在の `go-scan` のサンプル (`examples/derivingjson`, `examples/derivingbind`) は、それぞれ単一の目的に特化したジェネレータです。これらを統合し、効率的に連携させるためには、`go-scan` 自身、およびその利用パターンを進化させる必要があります。

本ドキュメントでは、この「複数マーカー（アノテーション）に基づく複数ジェネレータ」のシナリオを実現するための設計思想、状態共有メカニズム、具体的な `main` 関数の構成、そして `go-scan` に求められる機能改善について、これまでの対話を通じて深掘りした考察をまとめます。

## 2. 背景: 既存の知見と `go-scan` の現状

### 2.1. 既存ドキュメント (`docs/ja/from-*.md`) からの洞察

リポジトリ内の `docs/ja/from-derivingbind.md` および `docs/ja/from-derivngjson.md` には、それぞれのサンプルジェネレータ開発を通じて得られた `go-scan` への具体的な改善提案が記載されています。これらの提案は、複数ジェネレータ実行の文脈において、さらにその重要性を増します。

*   **アノテーション/コメントベースのフィルタリング・抽出**: ジェネレータが処理対象の型を特定する主要な手段。現状は各ジェネレータが独自に文字列処理を行っており、`go-scan` 側での統一的なサポートが望まれる。
*   **フィールドタグの高度な解析**: 構造体フィールドに付与されたタグ（例: `json:"name,omitempty"`）は、ジェネレータの挙動を細かく制御するための重要なメタデータ。解析ロジックの共通化が必要。
*   **インターフェース実装者の効率的な検索**: 特に `oneOf` のようなポリモーフィックな型を扱う際に不可欠。
*   **型解決の強化と詳細情報**: 型の完全修飾名、インポートパス、ポインタやスライスなどの複合型の詳細情報を正確かつ容易に取得できることが求められる。
*   **生成コードのインポート管理支援**: 生成コードが必要とするパッケージのインポート文を自動的に管理する機能は、ジェネレータ開発の負担を大幅に軽減する。

これらの機能が `go-scan` に備わることで、各ジェネレータは型情報の解析という共通処理から解放され、本来のコード生成ロジックに集中できるようになります。これは、複数のジェネレータが協調する上で不可欠な前提条件です。

### 2.2. `go-scan` のコア機能と現状のアーキテクチャ

`scanner/scanner.go` と `scanner/models.go` の調査から、`go-scan` の現在のコア機能は以下のように理解できます。

*   **`Scanner`**: Goソースコードをパースし、型情報を抽出する中心的なコンポーネント。`FileSet` を共有し、`ScanPackage` や `ScanFiles` メソッドを通じて `PackageInfo` を生成します。
*   **`PackageInfo`**: 単一パッケージのスキャン結果（型、関数、定数、インポート情報など）を保持する構造体。複数ジェネレータ間で共有されるべき主要な情報集約単位です。
*   **`TypeInfo`, `FieldInfo`, `FieldType`**: 型、フィールド、フィールドの型に関する詳細な情報を格納するモデル。特に `FieldType.Resolve()` は、`PackageResolver` を介して外部パッケージの型を遅延解決する重要な機能を持ちます。
*   **`PackageResolver`**: インポートパスから `PackageInfo` を解決するためのインターフェース。これにより、`Scanner` は必要に応じて他のパッケージをスキャン（またはキャッシュから取得）できます。

現状の `go-scan` は、指定されたパッケージを一度スキャンし、その結果を `PackageInfo` として提供します。この `PackageInfo` を複数のジェネレータで共有すること自体は可能です。しかし、各ジェネレータがその中から自身に必要な情報を効率的に抽出し、かつ依存関係にある他のパッケージの情報を透過的に解決するためには、さらなるサポート機能が求められます。

## 3. 設計目標

複数ジェネレータによる効率的なコード生成と状態共有を実現するために、以下の設計目標を設定します。

1.  **効率性**: パッケージのスキャン（ファイルI/O、AST解析）は、対象パッケージごと（理想的にはモジュールごと）に一度だけにします。
2.  **独立性**: 各ジェネレータは、他のジェネレータの処理内容や関心事に影響されず、自身が必要とする情報のみを取得・処理できるようにします。
3.  **柔軟性**: 新しいジェネレータを容易に追加・統合できるようにします。
4.  **シンプルさ**: `go-scan` および関連ツールの利用者が理解しやすく、使いやすいAPIを提供します。

## 4. 提案: 状態共有メカニズム `ScanBroker`

これらの設計目標を達成するため、中心的な役割を果たす `ScanBroker`（または `ScanContext`, `SharedScanner`）というコンポーネントの導入を提案します。

### 4.1. `ScanBroker` の責務

*   **`scanner.PackageInfo` のキャッシュ管理**: `importPath` をキーとして `PackageInfo` をキャッシュし、再スキャンを防止します。
*   **`token.FileSet` の共有**: 全てのスキャン処理で単一の `FileSet` を利用し、位置情報の一貫性とメモリ効率を向上させます。
*   **`scanner.ExternalTypeOverride` の一元管理**: 全スキャンで共通の型オーバーライド設定を適用します。
*   **Broker-Aware `scanner.PackageResolver` の提供**: `scanner.Scanner` が依存パッケージを解決する際に、ブローカーのキャッシュメカニズムを経由させます。

### 4.2. `ScanBroker` の構造（概念コード）

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
	// CRITICAL: Robustly resolve dirPath to a canonical import path.
	// This might involve finding go.mod, using `go list -json`, etc.
	// Placeholder for this complex logic:
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
		// If no dirPath hint, we need to resolve importPath to a directory.
		// This is another point where go/packages or `go list` would be used.
		// For simplicity, this example might fail if no hint and not a GetPackageByDir call.
		resolvedDir, err := b.resolveImportPathToDir(ctx, importPath) // Placeholder
		if err != nil {
			return nil, fmt.Errorf("cannot find directory for import path %s: %w", importPath, err)
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

	// Ensure the PackageInfo stores its canonical import path.
	// This should ideally be set by ScanPackage itself after determining it.
	if scannedPkgInfo.ImportPath == "" {
		scannedPkgInfo.ImportPath = importPath
	} else if scannedPkgInfo.ImportPath != importPath {
		// Log a warning if determined import path differs from the requested one,
		// might indicate issues in path resolution.
		slog.WarnContext(ctx, "ScanBroker: Scanned package import path mismatch",
			slog.String("requestedImportPath", importPath),
			slog.String("determinedImportPath", scannedPkgInfo.ImportPath))
		// Decide on a strategy: use requested, use determined, or error.
		// For caching, consistency is key; using the requested (and hopefully canonical) importPath.
	}

	b.scanCache[importPath] = scannedPkgInfo
	slog.InfoContext(ctx, "ScanBroker: Package scanned and cached", slog.String("importPath", importPath))
	return scannedPkgInfo, nil
}

// resolveDirToImportPath converts a directory path to a canonical Go import path. (Placeholder)
func (b *ScanBroker) resolveDirToImportPath(ctx context.Context, dirPath string) (string, error) {
	// In a real implementation, this would use `go list -json -C <dirPath> .` or similar
	// or parse go.mod files and directory structure.
	// For this example, we'll use a simplified (and not robust) approach.
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return "", fmt.Errorf("abs path for %s: %w", dirPath, err)
	}
	// This is NOT a general solution. A proper solution needs to understand Go modules.
	// Returning the absolute path is a placeholder for a unique key if true import path resolution is complex.
	slog.WarnContext(ctx, "ScanBroker: resolveDirToImportPath is using a placeholder (absolute path). Robust import path resolution needed.", slog.String("dirPath", dirPath), slog.String("resolvedKey", absPath))
	return absPath, nil // This should be the CANONICAL import path.
}

// resolveImportPathToDir converts a canonical import path to a directory path. (Placeholder)
func (b *ScanBroker) resolveImportPathToDir(ctx context.Context, importPath string) (string, error) {
	// In a real implementation, this would use `go list -json -f {{.Dir}} <importPath>`
	return "", fmt.Errorf("resolveImportPathToDir: not implemented robustly. Import path '%s' cannot be resolved to a directory without external tools/logic", importPath)
}

// BrokerPackageResolver implements scanner.PackageResolver for the ScanBroker.
type BrokerPackageResolver struct {
	broker *ScanBroker
}

// ScanPackageByImport is called by scanner.Scanner for resolving imported packages.
func (r *BrokerPackageResolver) ScanPackageByImport(ctx context.Context, importPath string) (*scanner.PackageInfo, error) {
	slog.DebugContext(ctx, "BrokerPackageResolver: Request to resolve import", slog.String("importPath", importPath))
	// This will hit the broker's cache or trigger a scan (if dir can be found for importPath).
	return r.broker.getPackageByImportPath(ctx, importPath)
}
```

### 4.3. `ScanBroker` の利点

*   **効率性**: `scanCache` により、各パッケージのスキャンは一度だけ実行されます。
*   **状態の一元管理**: `FileSet`, `ExternalTypeOverride`, キャッシュされた `PackageInfo` が一箇所で管理されます。
*   **透過的な依存関係解決**: ジェネレータが `FieldType.Resolve()` を呼び出すと、内部的に `ScanBroker` のキャッシュ/スキャン機構が利用され、依存パッケージも効率的に処理されます。

### 4.4. 課題: パス解決

`ScanBroker` の設計における最大の課題は、ディレクトリパスとGoの正規インポートパス間の相互変換 (`resolveDirToImportPath`, `resolveImportPathToDir`) です。Goモジュールの複雑さ（`replace`ディレクティブ、ワークスペースなど）を考慮すると、この解決ロジックは非常に高度になります。`go/packages` ライブラリや `go list` コマンドの機能を利用するのが現実的なアプローチです。`go-scan` がこれらの外部ツールに依存するか、あるいはこの解決を利用者に委ねるかは、設計上の重要な判断点となります。

## 5. `main` 関数の設計例とジェネレータの連携

`ScanBroker` を利用した複数ジェネレータ実行の `main` 関数の構成例です。

```go
package main

import (
	"context"
	"fmt"
	"log" // Standard log for fatal errors
	"os"
	"strings" // For simplified annotation checking in examples

	"log/slog" // For structured logging

	// Assume ScanBroker and scanner types are properly imported or defined
	// For brevity, their full definitions from section 4.2 are omitted here.
	// goscan "path/to/your/goscan/brokerpackage"
	// scanner "github.com/podhmo/go-scan/scanner"
)


// BaseGenerator defines a common interface for all code generators.
type BaseGenerator interface {
	Name() string                                         // Returns the name of the generator.
	Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error // Processes the PackageInfo.
}

// --- Example: JSONGenerator ---
type JSONGenerator struct{}
func NewJSONGenerator() *JSONGenerator { return &JSONGenerator{} }
func (g *JSONGenerator) Name() string { return "JSONGenerator" }
func (g *JSONGenerator) Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error {
	slog.InfoContext(ctx, "Running generator", slog.String("name", g.Name()), slog.String("package", pkgInfo.Name))
	for _, typeInfo := range pkgInfo.Types {
		// In a real generator, use TypeInfo.GetAnnotations() as proposed later.
		if strings.Contains(typeInfo.Doc, "@deriving:json") {
			slog.InfoContext(ctx, fmt.Sprintf("  [%s] Found target type: %s", g.Name(), typeInfo.Name))
			// ... (Actual JSON generation logic) ...
			// Example: If a field type needs to be resolved:
			// for _, field := range typeInfo.Struct.Fields {
			//   if !field.Type.IsBuiltin && field.Type.FullImportPath() != pkgInfo.ImportPath {
			//     slog.DebugContext(ctx, "Resolving external field type", slog.String("field", field.Name), slog.String("type", field.Type.String()))
			//     def, err := field.Type.Resolve(ctx) // Uses broker's resolver
			//     if err != nil {
			//       slog.ErrorContext(ctx, "Failed to resolve field type", slog.Any("error", err))
			//       continue
			//     }
			//     if def != nil { /* Use resolved type 'def' */ }
			//   }
			// }
		}
	}
	return nil
}

// --- Example: BindGenerator ---
type BindGenerator struct{}
func NewBindGenerator() *BindGenerator { return &BindGenerator{} }
func (g *BindGenerator) Name() string { return "BindGenerator" }
func (g *BindGenerator) Generate(ctx context.Context, pkgInfo *scanner.PackageInfo) error {
	slog.InfoContext(ctx, "Running generator", slog.String("name", g.Name()), slog.String("package", pkgInfo.Name))
	for _, typeInfo := range pkgInfo.Types {
		if strings.Contains(typeInfo.Doc, "@deriving:binding") {
			slog.InfoContext(ctx, fmt.Sprintf("  [%s] Found target type: %s", g.Name(), typeInfo.Name))
			// ... (Actual binding generation logic) ...
		}
	}
	return nil
}


func main() {
	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		slog.Error("Usage: multi-generator <package_directory_path>")
		os.Exit(1)
	}
	targetPackageDir := os.Args[1]
	ctx := context.Background()

	slog.InfoContext(ctx, "Initializing ScanBroker...")
	// Assuming NewScanBroker is available from our goscan (broker) package.
	// Real implementation of NewScanBroker needs to be imported.
	// For this doc, we assume it's defined as in section 4.2.
	broker := NewScanBroker(nil) // Pass scanner.ExternalTypeOverride if any.

	// Register all generators
	generators := []BaseGenerator{
		NewJSONGenerator(),
		NewBindGenerator(),
		// Add other generators: NewValidateGenerator(), etc.
	}

	slog.InfoContext(ctx, "Fetching package information via ScanBroker", slog.String("targetDir", targetPackageDir))
	// Get the PackageInfo for the target directory.
	// This performs the scan via the broker, utilizing its cache.
	pkgInfo, err := broker.GetPackageByDir(ctx, targetPackageDir)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get package info", slog.String("dir", targetPackageDir), slog.Any("error", err))
		os.Exit(1)
	}
	slog.InfoContext(ctx, "Package information retrieved", slog.String("packageName", pkgInfo.Name), slog.String("importPath", pkgInfo.ImportPath))

	// Execute each generator with the *same* PackageInfo.
	for _, gen := range generators {
		if err := gen.Generate(ctx, pkgInfo); err != nil {
			slog.ErrorContext(ctx, "Generator failed", slog.String("generator", gen.Name()), slog.Any("error", err))
			// Decide if one generator failure should stop others. For now, continue.
		} else {
			slog.InfoContext(ctx, "Generator completed successfully", slog.String("generator", gen.Name()))
		}
	}

	slog.InfoContext(ctx, "All code generation tasks processed.")
}
```

**処理フローのポイント:**

1.  **`ScanBroker` 初期化**: アプリケーション起動時に一度だけ `ScanBroker` を作成。
2.  **ジェネレータ登録**: 実行したいジェネレータをリスト化。共通インターフェース (`BaseGenerator`) を持つと管理が容易。
3.  **起点パッケージ情報取得**: `broker.GetPackageByDir()` で起点パッケージの `PackageInfo` を取得。スキャンは必要時のみ実行され、結果はキャッシュされる。
4.  **各ジェネレータ実行**: 全ジェネレータが同じ `PackageInfo` インスタンスを利用。依存型解決もブローカー経由で効率的に行われる。

## 6. `go-scan` への具体的なAPI改善提案

この複数ジェネレータ・状態共有アプローチを効果的に実現するため、`go-scan` に以下の具体的なAPI改善を提案します。これらはジェネレータ開発の負担を軽減し、宣言的で堅牢なコード記述を可能にします。

### 6.1. アノテーション処理の強化

*   **`TypeInfo.GetAnnotations(prefix string) (map[string]string, error)`**: Docコメントから指定プレフィックスのアノテーションをキー・バリュー形式で抽出。
    *   例: `ann, _ := typeInfo.GetAnnotations("@deriving:")` で `{"json": "", "bindable": "path:/foo"}` を取得。
*   **`FieldInfo.GetAnnotations(prefix string) (map[string]string, error)`**: フィールドDocコメント用。
*   **`PackageInfo.FilterTypesByAnnotation(predicate func(typeName string, annotations map[string]string) bool, annotationPrefix string) []*TypeInfo`**: アノテーションに基づく型フィルタリング。

### 6.2. フィールドタグ解析の強化 (`FieldInfo` メソッド)

*   **`TagValue(key string) (string, bool)`**: タグキーの主要値を取得 (例: `json:"name,omitempty"` -> `"name"`).
*   **`TagOptions(key string) ([]string, bool)`**: タグキーのオプション部分を取得 (例: `json:"name,omitempty"` -> `["omitempty"]`).
*   **`TagSubFields(key string) (map[string]string, bool)`**: 複合タグ値 (例: `validate:"required;len:5-50"`) をパース。

### 6.3. 型解決と型情報アクセスの強化

*   **`FieldType.QualifiedName() string`**: パッケージエイリアスを考慮した完全修飾型名 (例: `pkgalias.MyType`).
*   **`FieldType.SimpleKind() TypeKind`**: 型の基本種別 (enum: `Primitive`, `NamedStruct`, `Pointer`, etc.).
*   **`TypeInfo.CanonicalImportPath() string`**: 型定義パッケージの正規インポートパス。
*   **`FieldType.IsStruct() bool`, `IsInterface() bool`, etc.**: 型カテゴリ判定ヘルパー。

### 6.4. インポート管理の支援

*   **`ImportTracker` (ユーティリティ型)**
    *   `NewImportTracker(currentPackageImportPath string) *ImportTracker`
    *   `AddType(ft *scanner.FieldType)`: `FieldType` から必要なインポートを自動記録。
    *   `AddImport(importPath string, alias string)`: 手動追加。
    *   `RequiredImports() map[string]string`: 収集結果 (`path -> alias`)。
    *   `RenderBlock() string`: `import (...)` ブロック文字列を生成。

### 6.5. `ScanBroker` / `Scanner` による高度な検索

*   **`ScanBroker.FindImplementers(ctx, interfaceTypeInfo, searchScope) ([]*TypeInfo, error)`**: インターフェース実装型を広範囲に検索。
*   **`ScanBroker.FindTypesWithAnnotation(ctx, annotationPrefix, predicate, searchScope) ([]*TypeInfo, error)`**: アノテーション条件で型を広範囲に検索。

これらの改善により、ジェネレータは型情報の詳細な解析や管理といった共通タスクから解放され、本質的なコード生成ロジックに注力できます。

## 7. まとめと今後の展望

`ScanBroker` のような状態共有メカニズムと、提案されたAPI群による `go-scan` の機能強化は、Go言語におけるコードジェネレーションの新たな可能性を拓きます。複数のジェネレータが効率的かつ協調的に動作する環境は、Haskellの `deriving` のような強力で宣言的なコード生成体験に近づくための一歩となるでしょう。

今後の展望としては、`ScanBroker` のパス解決ロジックの堅牢化（`go/packages` との連携など）、非同期ジェネレータ実行のサポート、より高度な型システムクエリ言語の導入などが考えられます。これらの進化を通じて、`go-scan` がGoエコシステムにおける型駆動開発のさらに強力な基盤となることを期待します。
