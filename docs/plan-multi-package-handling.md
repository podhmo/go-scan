# 複数パッケージにまたがる型解決の改善計画

## 1. 概要

`go-scan` の現在の実装、特に `examples/convert` ツールでは、単一のパッケージを起点としたスキャンしか想定されていない。しかし、パッケージをまたいだコード生成（例：`source` パッケージの型から `destination` パッケージの型への変換）を行うには、複数のパッケージから型情報を収集し、それらを関連付けて解決する仕組みが必要不可欠である。

`docs/ja/trouble-from-convert.md` に記述されているように、`main.go` で複数の `scanner.PackageInfo` を手動でマージするアプローチは、`parser` や `generator` がその構造を想定していないため、多くの問題を引き起こした。

本計画では、この問題を根本的に解決するため、`go-scan` のコア機能を拡張し、複数のパッケージ情報を一元的に管理・解決する「統合パッケージコンテキスト」の概念を導入する。これにより、`examples/convert` のようなツールは、よりクリーンで堅牢な方法で複数パッケージを扱えるようになる。

## 2. TODOリスト

-   **`go-scan` のコア機能拡張**:
    -   `goscan.Scanner` を、スキャンした全パッケージ情報を保持する「統合コンテキスト」として機能するように強化する。
    -   型解決ロジックを修正し、コンテキスト内の全パッケージを横断して型を検索できるようにする。
-   **`examples/convert` のリファクタリング**:
    -   `parser` が `@derivingconvert` アノテーションを解析し、未知のパッケージを検出した際に、`goscan.Scanner` を通じて動的に（遅延して）スキャンするよう修正する。
-   **`generator` の改善**:
    -   `ImportManager` を活用し、生成コード内で外部パッケージの型名が正しくパッケージプレフィックスで修飾されるよう修正する。
-   **テストの拡充**:
    -   複数パッケージにまたがる型解決のユニットテストを `go-scan` に追加する。
    -   `examples/convert` のインテグレーションテストを更新し、クロスパッケージ変換のシナリオを網羅する。

## 3. 計画と詳細設計

### 3.1. `go-scan` のコア機能拡張: 統合パッケージコンテキスト

**目的**: `goscan.Scanner` が、単一の `Scan` 呼び出しのライフサイクルを超えて、複数のパッケージ情報を一元管理するコンテキストとして機能するようにする。

**現状の課題**: `goscan.Scanner` の `packageCache` は存在するが、型解決（`ResolveType`）は、ある特定の `PackageInfo` のコンテキスト内でのみ行われることが多く、キャッシュされた他のパッケージ情報を横断的に利用する明確な仕組みがない。

**変更点**:

1.  **`goscan.Scanner` の役割の明確化**:
    -   `Scanner` インスタンスを、スキャンされた全パッケージの型情報を保持する唯一の信頼できる情報源（Single Source of Truth）として位置づける。
    -   `packageCache` (`map[string]*scanner.PackageInfo`) をこのコンテキストの基盤とし、`ScanPackage` や `ScanPackageByImport` は常にこのキャッシュを更新・利用する。

2.  **型解決ロジックの修正**:
    -   `scanner.Scanner.ResolveType`（内部スキャナ）が、型を解決する際に、まず自身のパッケージ内で型を探し、見つからない場合は親の `goscan.Scanner` の `packageCache` 全体を検索するよう修正する。
    -   このため、`scanner.Scanner` は、自身を生成した `goscan.Scanner` への参照を保持する必要がある。`goscan.Scanner.ScanFiles` を呼び出す際に、`s` (自分自身) を渡すことでこれを実現する。

    ```go
    // in goscan.go

    // scanner.Scannerに、親のgoscan.Scannerへのポインタを追加
    type Scanner struct {
        // ... existing fields
        parent *goscan.Scanner
    }

    // goscan.Scannerが内部スキャナを呼び出す際に自身を渡す
    func (s *Scanner) ScanFiles(ctx context.Context, filePaths []string) (*scanner.PackageInfo, error) {
        // ...
        // 内部スキャナに自分自身(s)への参照を渡す
        pkgInfo, err := s.scanner.ScanFiles(ctx, filesToParse, pkgDirAbs, s)
        // ...
    }

    // in scanner/scanner.go

    // 内部スキャナのResolveTypeが親のキャッシュを参照する
    func (s *Scanner) ResolveType(ctx context.Context, fieldType *FieldType) (*TypeInfo, error) {
        // ...
        // 自パッケージで見つからない場合...
        if s.parent != nil {
            // 親のキャッシュ全体から型を探すロジックを追加
            pkg, err := s.parent.ScanPackageByImport(ctx, fieldType.PkgPath)
            if err == nil && pkg != nil {
                 // pkg.Typesから型を探す
            }
        }
        // ...
    }
    ```

### 3.2. `examples/convert` のリファクタリング: 遅延パッケージスキャン

**目的**: `main.go` で事前にすべてのパッケージをスキャンするのではなく、`parser` がアノテーションを解釈する過程で、必要になったパッケージをその場でスキャンする。

**変更点**:

1.  **`convert/main.go` のシンプル化**:
    -   `-pkg` フラグで指定された単一のソースパッケージを `s.ScanPackage()` でスキャンするだけの、シンプルな作りに留める。

2.  **`convert/parser/parser.go` の機能強化**:
    -   `Parse` 関数が `*goscan.Scanner` のインスタンスを引数として受け取るようにする。
    -   `@derivingconvert(pkg.DstType)` のようなアノテーションを解析する。
    -   `pkg` 部分のインポートパスを特定する。
    -   `s.ScanPackageByImport(ctx, destinationImportPath)` を呼び出す。これにより、宛先パッケージの型情報が `goscan.Scanner` の中央キャッシュにロードされる。
    -   キャッシュが更新された後、`DstType` の型情報を `Scanner` に問い合わせて取得する。

    ```go
    // in examples/convert/parser/parser.go

    // Parse関数が*goscan.Scannerを受け取る
    func Parse(ctx context.Context, s *goscan.Scanner, scannedPkg *scanner.PackageInfo) (*Info, error) {
        // ...
        for _, t := range scannedPkg.Types {
            // @derivingconvert アノテーションを解析
            // ...
            // dstPkgPath と dstTypeName を取得
            // ...

            // 宛先パッケージを動的にスキャン
            _, err := s.ScanPackageByImport(ctx, dstPkgPath)
            if err != nil {
                return nil, fmt.Errorf("failed to scan destination package %q: %w", dstPkgPath, err)
            }

            // 改めて型情報を解決する
            dstTypeInfo, err := s.ResolveType(...) // ResolveTypeはgoscan.Scannerのメソッドとして提供する必要があるかもしれない
            // ...
        }
        // ...
    }
    ```

### 3.3. `generator` の改善: 正確な型名生成

**目的**: 生成されるコードにおいて、異なるパッケージの型名を `ImportManager` を用いて正しく修飾する。

**変更点**:

1.  **`generator/generator.go` の `getTypeName` (またはそれに類する関数) の修正**:
    -   `TypeInfo` が持つ `PkgPath`（インポートパス）と、現在コードを生成しているパッケージのインポートパスを比較する。
    -   インポートパスが異なる場合、その `PkgPath` を `ImportManager` に登録し、適切なパッケージエイリアス（またはパッケージ名）を取得する。
    -   `alias.TypeName` の形式で、修飾された型名を返す。
    -   インポートパスが同じ場合は、型名のみを返す。

## 4. テスト計画

-   **`goscan` のユニットテスト**:
    -   `TestResolveType_CrossPackage` のようなテストケースを追加する。
    -   `Scanner` に2つの異なるパッケージ（例：`pkgA`, `pkgB`）をスキャンさせ、`pkgA` の型定義内から `pkgB` の型が正しく解決できることを検証する。
-   **`examples/convert` のインテグレーションテスト**:
    -   `scantest` を使用して、`source` と `destination` が異なるパッケージに存在するテストケースを整備する。
    -   `main_test.go` から `convert` の `run` 関数を呼び出し、生成されたコードがコンパイル可能であり、かつ期待通りに動作することを確認する。
    -   生成されたコードに、正しい `import` 文と、正しく修飾された型名が含まれていることをアサーションで確認する。
