# 複数パッケージにまたがる型情報解決の改善計画

## 1. 概要

現在の `go-scan` は、単一のGoパッケージをスキャン対象として設計されている。しかし、`examples/convert` のようなコード生成ツールでは、複数のパッケージ（例：変換元の `source` パッケージと変換先の `destination` パッケージ）にまたがる型情報を同時に解決する必要がある。

現状の `goscan.Scanner` は、一度に一つのパッケージの情報 (`scanner.PackageInfo`) しか扱えず、これが複雑なユースケースへの対応を困難にしている。`main.go` で複数のパッケージを個別にスキャンし、その結果を無理にマージするアプローチは、場当たり的であり、多くの問題を引き起こすことが `docs/ja/trouble-from-convert.md` で報告されている。

このドキュメントは、`go-scan` のコア機能を拡張し、複数パッケージを統一的に、かつ効率的に扱えるようにするための設計と計画を定義する。

## 2. TODOリスト

- [ ] **`Session` (または `Repository`) オブジェクトの導入:**
    - 複数パッケージの情報を一元管理する新しい中心的なオブジェクトを設計・実装する。
- [ ] **`goscan.Scanner` の役割変更:**
    - `Scanner` を `Session` のコンテキスト内で動作する、より軽量なパーサー実行役に位置づける。
- [ ] **APIの刷新:**
    - 複数パッケージのロード、およびロード済みパッケージ情報へのアクセスを容易にするための新しいAPIを設計する。
- [ ] **パーサー (`parser`) との連携強化:**
    - パーサーがアノテーションなどを解析中に、未知のパッケージに遭遇した場合に、動的に（lazyに）パッケージをロードする仕組みを導入する。
- [ ] **`examples/convert` のリファクタリング:**
    - 新しいAPIを利用するように `examples/convert/main.go` を全面的に書き換える。
- [ ] **テストの整備:**
    - 複数パッケージを扱うシナリオに特化した単体テストおよびインテグレーションテストを `scantest` を用いて整備する。

## 3. 計画と詳細設計

### 3.1. 中核概念: `Session` オブジェクト

複数パッケージの情報を横断的に管理するための新しいオブジェクト `goscan.Session` を導入する。これは、一つのコード生成タスクにおけるすべての状態を保持する。

**`Session` の責務:**
- `token.FileSet` の一元管理。
- ロード済みの全パッケージ情報 (`map[string]*scanner.PackageInfo`) の保持（パッケージキャッシュ）。
- 読み込み済みの全ファイル (`map[string]struct{}`) の追跡。
- `locator` インスタンスの保持。
- `Session` のコンテキストで動作する `Scanner` の生成。

```go
// goscan/session.go (new file)
package goscan

import (
    "context"
    "go/token"

    "github.com/podhmo/go-scan/locator"
    "github.com/podhmo/go-scan/scanner"
)

// Session は、複数のパッケージにまたがるスキャン操作全体の状態を管理する。
type Session struct {
    Fset         *token.FileSet
    Locator      *locator.Locator
    PackageCache map[string]*scanner.PackageInfo // Key: import path
    VisitedFiles map[string]struct{}           // Key: absolute file path
    // 他にも overlay や symbol cache の管理も担う
}

// NewSession は新しいセッションを作成する。
func NewSession(workdir string) (*Session, error) {
    // ... locatorやfsetの初期化 ...
}

// LoadPackages は指定されたインポートパスのパッケージをロードし、セッションにキャッシュする。
// すでにロード済みの場合はキャッシュを返す。
func (s *Session) LoadPackages(ctx context.Context, importPaths ...string) error {
    // ... 内部で scanner を使い、未ロードのパッケージをスキャンして PackageCache を更新 ...
}

// LookupType は、セッションにロードされたすべてのパッケージから、
// "example.com/foo/bar.MyType" のような完全修飾名で型情報を検索する。
func (s *Session) LookupType(qualifiedName string) (*scanner.TypeInfo, error) {
    // ... PackageCache を横断的に検索 ...
}
```

### 3.2. `goscan.Scanner` の役割変更

`Scanner` は状態を持つオブジェクトから、`Session` のコンテキスト内で特定のファイル群をパースする、より軽量な実行エンジンへと役割を変える。

- `Scanner` は `Session` から `Fset` や `VisitedFiles` などの状態を渡されて動作する。
- `ScanPackage` や `ScanFiles` といったメソッドは `Session` 側に移設、または `Session` を介して呼び出される形になる。

### 3.3. 動的なパッケージロード (Lazy Loading)

パーサーが型情報を必要としたタイミングで、動的にパッケージをロードする仕組みを導入する。これにより、最初にすべてのパッケージを指定する必要がなくなり、より柔軟で効率的なスキャンが可能になる。

**実装案:**
1. `parser.Parse` 関数は、単一の `PackageInfo` ではなく、`Session` オブジェクトを受け取るように変更する。
2. `parser` は、`@derivingconvert` のようなアノテーションを解析し、`destination` に指定された型（例: `github.com/foo/dto.UserDTO`）を見つける。
3. `parser` は、その型のインポートパス (`github.com/foo/dto`) を抽出し、`session.LoadPackages("github.com/foo/dto")` を呼び出す。
4. `Session` は、そのパッケージが未ロードであればスキャンを実行し、結果をキャッシュする。
5. これで `parser` は `session.LookupType("github.com/foo/dto.UserDTO")` を使って、必要な型情報を安全に取得できる。

**`parser.Parse` のシグネチャ変更:**

```go
// examples/convert/parser/parser.go

// 変更前
func Parse(ctx context.Context, pkg *scanner.PackageInfo) (*Info, error)

// 変更後
// Session を受け取り、必要なパッケージを自身でロードする
type Parser struct {
    Session *goscan.Session
}

func NewParser(s *goscan.Session) *Parser {
    return &Parser{Session: s}
}

func (p *Parser) Parse(ctx context.Context, rootPkg *scanner.PackageInfo) (*Info, error) {
    // ...
    // @derivingconvert(destination="...") を見つけたら
    // importPath := ... // "..." から抽出
    // err := p.Session.LoadPackages(ctx, importPath)
    // ...
}
```

### 3.4. `examples/convert` の修正

上記の新しいAPIと設計思想に基づき、`examples/convert/main.go` を全面的にリファクタリングする。

**`run` 関数の新しい実装イメージ:**

```go
// examples/convert/main.go

func run(ctx context.Context, pkgpath, workdir, output, pkgname string) error {
    // 1. Sessionを作成
    s, err := goscan.NewSession(workdir)
    if err != nil {
        return fmt.Errorf("failed to create session: %w", err)
    }

    // 2. 起点となるパッケージをロード
    if err := s.LoadPackages(ctx, pkgpath); err != nil {
        return fmt.Errorf("failed to load root package: %w", err)
    }
    // Get root package info from session cache
    rootPkg := s.PackageCache[pkgpath]


    // 3. ParserにSessionを渡してインスタンス化
    p := parser.NewParser(s)

    // 4. 解析を実行（内部で追加のパッケージがロードされる）
    info, err := p.Parse(ctx, rootPkg)
    if err != nil {
        return fmt.Errorf("failed to parse package info: %w", err)
    }

    // ... (以降の generator, writer の処理は同様) ...

    return nil
}
```

この変更により、`main.go` はどのパッケージをスキャンすべきかを事前にすべて知る必要がなくなり、責務が明確になる。複雑なパッケージ依存関係の解決は `go-scan` の `Session` と `parser` が協調して担当する。
