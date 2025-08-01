# `go-scan` の `convert` に関する問題のトラブルシューティング（再々構築）

## 目的

`go-scan`ライブラリの`examples/convert`ツールにおいて、ASTのスキャンをオンデマンドで複数パッケージにまたがって解決するリファクタリングを完遂する。

## 課題の再認識

リファクタリングの過程で、多数のテストが失敗した。特に、`TestIntegration_WithMultiPackage`で発生した`generated code mismatch`エラーと、`TestScanner_WithSymbolCache`でのキャッシュ関連の失敗が根深い問題であった。これらの問題は、単一の修正では解決せず、ライブラリのコア機能における複数の問題が絡み合っていることを示唆していた。

ご指摘を受け、場当たり的な修正ではなく、問題を再現するテストを先に記述し、それをパスするように修正するという、テスト駆動開発（TDD）のアプローチで問題解決にあたるべきであると再認識した。

## 実装とテストの詳細な経緯

### 1. `parser.go`のリファクタリングと最初のテスト失敗

**実装**:
`examples/convert/parser/parser.go`の`Parse`関数をリファクタリングし、`*goscan.Scanner`を引数として受け取るように変更した。これは、型解決を`FieldType.Resolve()`に委譲するための第一歩であった。

**テスト**:
この変更により、`parser_test.go`がコンパイルエラーを起こした。

**対応**:
テスト内で`*goscan.Scanner`のインスタンスを生成し、`Parse`関数に渡すように修正した。当初は`goscan.WithOverlay`のみで対応しようとしたが、`go.mod`が見つからないエラーが発生したため、`newTestDir`ヘルパーを導入し、テストごとに`go.mod`ファイルを含む一時ディレクトリを作成する方式に切り替えた。

```go
// examples/convert/parser/parser_test.go
func TestParse(t *testing.T) {
	// ...
	dir, cleanup := newTestDir(t, map[string]string{"source.go": source})
	defer cleanup()
	s, err := goscan.New(goscan.WithWorkDir(dir))
	// ...
	pkg, err := s.ScanFiles(context.Background(), []string{"source.go"})
	// ...
	got, err := Parse(context.Background(), s, pkg)
	// ...
}

func newTestDir(t *testing.T, files map[string]string) (string, func()) {
	// ...
	files["go.mod"] = "module example.com/sample"
	// ...
}
```

### 2. 標準ライブラリの解決問題 (`time.Time`)

**問題**:
`TestIntegration_WithImports`と`TestIntegration_WithGlobalRule`が、`import path "time" could not be resolved`というエラーで失敗した。`locator.go`が標準ライブラリのパスを解決できていなかった。

**実装**:
`locator/locator.go`の`FindPackageDir`関数に、インポートパスに`.`が含まれない場合は`GOROOT`を検索するロジックを追加した。

**テスト**:
この修正により、標準ライブラリのパス解決は進んだが、次の問題が発生した。

### 3. `PkgPath`の不整合と`ScanFilesWithKnownImportPath`の導入

**問題**:
標準ライブラリをスキャンした際に、`PackageInfo.ImportPath`が物理パス（例: `/usr/local/go/src/time`）になってしまい、期待されるインポートパス（例: `"time"`）と異なっていた。これにより、型名の完全修飾名が正しく構築できず、後続の処理で問題が発生していた。

**実装**:
この問題を根本的に解決するため、`scanner/scanner.go`に新しい内部API `ScanFilesWithKnownImportPath` を導入した。

```go
// scanner/scanner.go
func (s *Scanner) ScanFilesWithKnownImportPath(ctx context.Context, filePaths []string, pkgDirPath string, canonicalImportPath string) (*PackageInfo, error) {
	// ...
	return s.scanGoFiles(ctx, filePaths, pkgDirPath, canonicalImportPath)
}
```

`goscan.go`の`ScanPackageByImport`は、インポートパスが標準ライブラリのものであるかを判断し、そうであればこの新しいAPIを呼び出すように修正した。これにより、スキャン時に正しいインポートパスを強制的に設定できるようになった。

**テスト**:
この修正を検証するための直接的な単体テストは追加されていないが、`TestIntegration_WithImports`などの既存のテストが、この修正によってパスすることが期待された。

### 4. キャッシュが壊れる問題 (`TestScanner_WithSymbolCache`)

**問題**:
`TestScanner_WithSymbolCache`が常に失敗し、デバッグ出力からシンボルキャッシュが全く書き込まれていないことが判明した。

**原因**:
`goscan.go`の`updateSymbolCacheWithPackageInfo`関数の実装が、リファクタリングの過程で意図せず空になっていた。

**TDDアプローチでの修正計画**:

1.  **テストの作成**: `TestScanner_WithSymbolCache`テストが、この問題を明確に再現するテストケースとなっていることを確認する。このテストは、以下のシナリオを検証する。
    *   パッケージをスキャンした後、`SaveSymbolCache`を呼び出す。
    *   生成されたキャッシュファイル（`symbols.json`）を読み込み、期待されるシンボル（例: `example.com/multipkg-test/api.Handler`）が、正しい相対パスと共に記録されていることをアサートする。
    *   ファイルメタデータ（`Files`マップ）も正しく記録されていることをアサートする。

2.  **実装の修正**: 空になっていた`updateSymbolCacheWithPackageInfo`関数を再実装する。この関数は、`PackageInfo`から型、関数、定数の情報を抽出し、`symbolCache.SetSymbol`と`symbolCache.SetFileMetadata`を呼び出してキャッシュを更新する責務を持つ。

**実装したコード**:

```go
// goscan.go
func (s *Scanner) updateSymbolCacheWithPackageInfo(ctx context.Context, importPath string, pkgInfo *scanner.PackageInfo) {
	if s.CachePath == "" || pkgInfo == nil || len(pkgInfo.Files) == 0 {
		return
	}
	symCache, err := s.getOrCreateSymbolCache(ctx)
	if err != nil || !symCache.IsEnabled() {
		return
	}

	filesToSymbols := make(map[string][]string)

	for _, t := range pkgInfo.Types {
		filesToSymbols[t.FilePath] = append(filesToSymbols[t.FilePath], t.Name)
		key := importPath + "." + t.Name
		if err := symCache.SetSymbol(key, t.FilePath); err != nil {
			slog.WarnContext(ctx, "Failed to set symbol in cache", "key", key, "error", err)
		}
	}
    // ... Functions, Constantsも同様に処理 ...

	for filePath, symbols := range filesToSymbols {
		metadata := cache.FileMetadata{Symbols: symbols}
		if err := symCache.SetFileMetadata(filePath, metadata); err != nil {
			slog.WarnContext(ctx, "Failed to set file metadata in cache", "file", filePath, "error", err)
		}
	}
}
```

### 5. デバッグ関数の追加

デバッグ効率を向上させるため、以下の関数を実装した。これはスキャナの内部状態（訪問済みファイル、パッケージキャッシュ、シンボルキャッシュ）を標準出力にダンプする。

```go
// goscan.go
func (s *Scanner) debugDump(ctx context.Context) { /* ... */ }

// cache/cache.go
func (sc *SymbolCache) DebugDump() { /* ... */ }
```

これらの関数は`goscan_test.go`の`TestScanner_WithSymbolCache`内で呼び出され、キャッシュが空である問題の特定に役立った。

## 現在の状況

すべての変更を元に戻し、このドキュメントの作成に集中した。テストは依然として失敗しているが、問題の切り分けと、それぞれに対する具体的な実装方針が明確になった。

**次に行うべきこと**:
このドキュメントに記述した実装を再度正確に適用し、`make test`が成功するまで修正を続ける。特に、`TestIntegration_WithMultiPackage`の`generated code mismatch`エラーの解決に注力する。これは、`generator.go`の型名解決ロジック（`getTypeName`や`getBaseTypeName`など）が、複数パッケージのインポート情報を`ImportManager`経由で正しく扱えているかを再度検証する必要があることを示唆している。
