# `examples/minigo` から探る `go-scan` の活用と発展可能性

`examples/minigo` は、`github.com/podhmo/go-scan` ライブラリの能力を実証し、テストするために設計された小規模なGoインタープリタです。この `minigo` の実装や将来的な拡張を考察することを通じて、`go-scan` パッケージが持つ現状の有用性と、さらなる発展によってどのような価値を提供しうるかを探ります。この文書は、`minigo` という具体的なアプリケーションの視点から `go-scan` への期待をまとめたものです。

## `go-scan` の現状の強みと `minigo` での活かし方

`minigo` のようなコード解析や実行を行うツールにとって、`go-scan` が現在提供している機能は多くのメリットをもたらします。

1.  **ASTとソースコード位置情報へのアクセス**:
    *   `scanner.PackageInfo` は `Fset (*token.FileSet)` を保持し、各スキャン結果 (`TypeInfo`, `FunctionInfo` など) は関連する `Node (ast.Node)` や `FilePath (string)` を含みます。これにより、`minigo` はエラー報告やデバッグ情報の表示に必要なソースコード上の正確な位置情報を容易に取得できます。
    *   さらに、`PackageInfo.AstFiles (map[string]*ast.File)` により、スキャンされた各ファイルの完全な `*ast.File` に直接アクセス可能です。これは、`minigo` の評価ロジック (`eval` 関数群) が引き続き `ast.Node` を直接操作する上で非常に重要です。

2.  **型情報とシグネチャの詳細な解析**:
    *   `scanner.FunctionInfo` は、関数名、レシーバ、型パラメータ、パラメータ (`[]*FieldInfo`)、リターン値 (`[]*FieldInfo`)、そしてGoDocコメントといった詳細な関数シグネチャ情報を提供します。
    *   `scanner.TypeInfo` は、型名、種類 (struct, interface, alias, func)、型パラメータ、GoDocコメント、そして構造体であればフィールド (`StructInfo.Fields`)、インターフェースであればメソッド (`InterfaceInfo.Methods`) などの情報を提供します。
    *   これらの情報は、`minigo` がGoの関数や型をより深く理解し、例えば `examples/minigo/improvement.md` で提案されている `minigo/inspect` パッケージのような実行時リフレクション機能を実現する際の強力な基盤となります。

3.  **パッケージ間の型解決と柔軟な型解釈**:
    *   `scanner.FieldType.Resolve()` メソッドと `PackageResolver` インターフェースにより、パッケージを跨いだ型定義の遅延読み込みと解決が可能です。これにより、`minigo` は自身が解釈するコードが依存する外部パッケージの型情報を動的に取得できます。
    *   ジェネリクス型 (`TypeParams`, `TypeArgs`) のパースにも対応しており、Go 1.18以降のコードも扱えます。
    *   `ExternalTypeOverride` 機能により、特定の外部パッケージの型（例: `uuid.UUID`）を、`minigo` が扱いやすい別の型（例: `string`）として解釈するよう指定できるため、柔軟性が向上します。

## `minigo` の発展に向けた `go-scan` へのさらなる期待

`minigo` の機能をさらに強化し、より高度なインタープリタや静的解析ツールへと進化させることを考えると、`go-scan` には以下のような機能拡張やサポートが期待されます。

1.  **ソースコードコンテキスト取得の高レベルAPI**:
    *   現状でも位置情報は取得できますが、ASTノードや `token.Pos` から、該当行のソースコードスニペットやその周辺数行を簡単に取得できるAPI（例: `scanner.GetSourceContext(pos token.Pos, window int) SourceContext`）があれば、`minigo` のエラーメッセージやデバッグ出力が格段に分かりやすくなります。

2.  **シンボルテーブル構築とスコープ解析の支援**:
    *   識別子（変数名、関数名、型名など）の定義箇所と参照箇所を効率的に抽出し、それらのスコープ関係（可視性、シャドーイングなど）を分析する機能。これは、`minigo` が独自に持つ `Environment` のようなスコープ管理ロジックの実装を簡略化し、より堅牢なものにします。

3.  **GoDocコメントの高度な構造化解析**:
    *   現状の `TypeInfo.Annotation(name string)` のようなアノテーション抽出に加え、`@param <name> <description>`、`@return <description>`、`@see <symbol>` といった一般的なGoDocタグや、ユーザー定義の構造化コメント（例: `@minigo:export(name="foo")`）をパースし、構造化されたデータとして取得できるAPI。これにより、`minigo/inspect` がGoのドキュメントから豊富なメタデータを読み取り、`minigo` ユーザーに提示することが容易になります。

4.  **型システム関連の高度なユーティリティ**:
    *   ある型が特定のインターフェースを実装しているかどうかの判定。
    *   ジェネリック型が具体型でインスタンス化された際のメソッドシグネチャ等の解決。
    *   型の互換性チェック（代入可能かなど）。
    *   `TypeInfo` から直接メソッド一覧（`FunctionInfo` のスライスとして）やフィールド一覧（`FieldInfo` のスライスとして）を、より簡単に取得できるアクセサ。例えば、structのメソッドも `InterfaceInfo.Methods` と同様の形で `TypeInfo` から直接取得できると、`minigo/inspect` の実装が簡素化されます。

5.  **AST探索・変換ユーティリティの拡充**:
    *   XPath for AST、CSSセレクタライクなクエリ、特定のパターンに一致するノードの抽出、安全なAST変換ヘルパーなど、ASTをより宣言的に操作するためのユーティリティ。

6.  **補助的な解析機能**:
    *   **制御フローグラフ (CFG) / データフロー分析 (DFA) の基礎情報提供**: 未使用変数検出や到達不能コード解析など、より高度な静的解析機能の実現を支援。
    *   **インクリメンタルスキャン / パーシャルスキャン**: REPL環境や大規模プロジェクトにおける再スキャン効率の向上。
    *   **`go:generate` やビルドタグ等のディレクティブ認識**: コード生成プロセスや条件付きコンパイルのシミュレーションへの活用。

## `minigo` における `go-scan` 導入戦略：ハイブリッドアプローチ

`minigo` に `go-scan` を導入する際は、`go-scan` を主要な**パッケージ/ファイル構造解析フェーズ**で活用し、`minigo` のコアとなる**評価フェーズ** (`eval` 関数群) は引き続き `go/ast` ノードを直接扱う形を維持する**ハイブリッドアプローチ**が現実的です。`go-scan` が `PackageInfo.AstFiles` を提供することで、このアプローチはスムーズに実現できます。

### 主な統合ステップ

1.  **初期化**: `minigo` の `Interpreter` が `go-scan` の `Scanner` インスタンスを生成（`token.FileSet` を共有）。
2.  **スキャン**: `minigo` のソースファイルを `Scanner.ScanFiles()` または `Scanner.ScanPackageFromFilePath()` でスキャンし、`PackageInfo` を取得。
3.  **シンボル登録**:
    *   `PackageInfo.Functions` を元に、各 `FunctionInfo.AstDecl (*ast.FuncDecl)` を `minigo` の環境に登録。
    *   `PackageInfo.AstFiles` から各 `*ast.File` を取得し、トップレベルの `ast.GenDecl` (VAR) を見つけてグローバル変数を評価・登録。`PackageInfo.Constants` も活用。
4.  **実行**: エントリーポイント関数（例: `main`）の `AstDecl` を用いて評価を開始。
5.  **インポート処理**: `minigo` の `import` 文で外部Goパッケージを参照する場合、`scanner.PackageResolver` を実装し、`go-scan` を用いて外部パッケージの情報を遅延ロード。これにより、`minigo/inspect` のような機能で外部Go関数のシグネチャ情報などを取得・利用可能になります。

### 期待される効果と今後の展望

この統合により、`minigo` は以下の恩恵を受けることが期待されます。

*   **解析処理の責務分離**: `go-scan` が構造解析とAST提供、`minigo` が評価に集中。
*   **一貫した位置情報とASTアクセス**: `go-scan` の `FileSet` と `AstFiles` の活用。
*   **将来の拡張性**: `go-scan` の進化（型解決、ジェネリクス対応強化など）を `minigo` に取り込みやすくなり、`minigo/inspect` のような高度なリフレクション機能の実装が現実的になります。

`minigo` は `go-scan` の可能性を示す一例です。このような具体的なアプリケーションの視点から `go-scan` の機能セットを継続的に評価し、フィードバックを行っていくことで、`go-scan` はGoエコシステムにおいてさらに価値のあるライブラリへと成長していくことでしょう。
