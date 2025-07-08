# MiniGo 組み込み関数 自動生成の提案

## 概要

MiniGoインタプリタに新しい組み込み関数を追加する作業を効率化し、一貫性を保つために、組み込み関数のコード（`BuiltinFunction` オブジェクトの定義、引数処理、型チェックなど）を自動生成するツールの導入を提案します。

## 背景

現在、新しい組み込み関数（例: `fmt.Sprintf`, `strings.Join`）を追加するには、以下の手作業が必要です。

1.  関数の実処理を行う `evalXxx` 関数を実装する。
2.  `evalXxx` 関数をラップし、`BuiltinFunction` オブジェクトを生成するコードを記述する。
3.  引数の数や型をチェックするボイラープレートコードを記述する。
4.  インタプリタの登録用マップ（例: `GetBuiltinFmtFunctions`）にエントリを追加する。

これらの作業は定型的であり、関数が増えるにつれて煩雑になり、ミスも発生しやすくなります。

## 設計方針

### 1. インプット: アノテーション付きGoソースファイル

Goのソースファイルに特別なコメント形式のアノテーションを記述することで、組み込み関数の仕様を定義します。

*   **アノテーション対象:** 専用のディレクトリ（例: `builtins_src/`）に配置されたGoのソースファイル (`*.go`)。
*   **アノテーション形式:**
    *   `//minigo:builtin name=<minigo_func_name> [target_go_func=<go_func_name> | wrapper_func=<custom_wrapper_name>]`
        *   `name`: MiniGoインタプリタ内での関数名 (例: `"strings.Contains"`, `"fmt.Sprintf"`).
        *   `target_go_func` (オプション): 直接呼び出すGoの標準ライブラリ関数や自作関数 (例: `"strings.Contains"`)。指定した場合、引数・戻り値の型変換は自動生成ロジックが担当。
        *   `wrapper_func` (オプション): 引数処理からGo関数呼び出し、戻り値変換までをカスタム実装したGo関数名 (例: `"main.evalFmtSprintfOriginal"`)。より複雑なロジックを持つ関数向け。`target_go_func` とは排他的。
    *   `//minigo:args [variadic=true] [format_arg_index=<idx>]`
        *   `variadic`: 可変長引数を取る場合に指定。
        *   `format_arg_index`: `fmt.Sprintf` のようにフォーマット文字列を取る場合にその引数インデックスを指定。
    *   `//minigo:arg index=<idx> name=<arg_name> type=<MINIGO_TYPE> [go_type=<GO_TYPE>]`
        *   `index`: 引数のインデックス (0始まり)。
        *   `name`: 引数名（ドキュメントやエラーメッセージ用）。
        *   `type`: MiniGoでの期待される型 (`STRING`, `INTEGER`, `BOOLEAN`, `ARRAY`, `MAP`, `ANY` など)。
        *   `go_type` (オプション): `target_go_func` を呼び出す際のGoの型。省略時は `MINIGO_TYPE` から推測。
    *   `//minigo:return type=<MINIGO_TYPE> [go_type=<GO_TYPE>]`
        *   `type`: MiniGoでの戻り値の型。
        *   `go_type` (オプション): `target_go_func` の戻り値のGoの型。省略時は `MINIGO_TYPE` から推測。

*   **スタブ関数:** アノテーションは、Goの関数宣言の直前に記述します。このGo関数はスタブとして機能し、`target_go_func` が指定されていれば本体は空でよく、型チェックやドキュメント生成のヒントとして利用できます。`wrapper_func` を使う場合も、引数構成の参考にスタブを定義できます。

**例 (`builtins_src/strings_builtins.go`):**
```go
package builtins_src

import "strings" // target_go_func で使うため

//minigo:builtin name="strings.Contains" target_go_func="strings.Contains"
//minigo:arg index=0 name=s type=STRING go_type=string
//minigo:arg index=1 name=substr type=STRING go_type=string
//minigo:return type=BOOLEAN go_type=bool
func containsStub(s string, substr string) bool { return false }

//minigo:builtin name="custom.StringLength" wrapper_func="main.evalCustomStringLength"
//minigo:arg index=0 name=str type=STRING
//minigo:return type=INTEGER
func customStringLengthStub(str string) int { return 0 }
```

### 2. 生成するコード

自動生成ツールは、上記アノテーションをパースし、以下のGoコードを生成します (例: `builtin_generated.go`)。

*   **`BuiltinFunction` オブジェクトの定義:** アノテーションに基づいて `object.BuiltinFunction` のスライスまたはマップを生成。
*   **ラッパー関数:**
    *   `target_go_func` が指定されている場合:
        *   引数の数と型のチェック。
        *   MiniGoの `Object` 型から指定された `go_type` への変換。
        *   `target_go_func` の呼び出し。
        *   結果のGoの型からMiniGoの `Object` 型への変換。
        *   エラーハンドリング。
    *   `wrapper_func` が指定されている場合:
        *   指定された `wrapper_func` を呼び出すだけのシンプルなラッパー。引数の数チェック程度は共通化可能。
*   **登録用ヘルパー関数:** 生成された全ての組み込み関数をまとめて取得できる関数 (例: `GetGeneratedBuiltinFunctions() map[string]*object.BuiltinFunction`)。

### 3. 既存の組み込み関数への適用

*   **`fmt.Sprintf` や `strings.Join` (現在の特殊実装) など:**
    *   これらは `wrapper_func` を使用して、既存の `evalFmtSprintf` や `evalStringsJoin` (必要ならリネームして `main.evalFmtSprintfOriginal` などとする) を指定します。
    *   アノテーション付きのスタブ関数は、`builtins_src/` 以下に配置します。
    ```go
    // builtins_src/fmt_builtins.go
    package builtins_src

    //minigo:builtin name="fmt.Sprintf" wrapper_func="main.evalFmtSprintfOriginal"
    //minigo:args variadic=true format_arg_index=0
    //minigo:return type=STRING
    func SprintfStub(format string, a ...interface{}) string { return "" }
    ```
*   これにより、既存の複雑なロジックを活かしつつ、定義の管理を一元化できます。

## 自動生成ツールのインターフェース

### コマンド名: `minigo-builtin-gen`

### コマンドラインオプション:

*   `minigo-builtin-gen -source <source_dir> -output <output_file>`
    *   `-source <source_dir>`: アノテーションが記述されたGoソースファイル群が含まれるディレクトリ (例: `./builtins_src`)。
    *   `-output <output_file>`: 生成されるGoコードの出力ファイルパス (例: `builtin_generated.go`)。
    *   (オプション) `-package <pkg_name>`: 生成コードのパッケージ名 (デフォルト: `main`)。
    *   (オプション) `-v`: 詳細ログ出力。

### `go:generate` との連携:

インタプリタの主要なGoファイル (例: `main.go`) に以下を記述:
```go
//go:generate minigo-builtin-gen -source ./builtins_src -output builtin_generated.go
package main
```
`go generate ./...` でコード生成を実行できます。

## 利点

*   **開発効率の向上:** 新しい組み込み関数の追加が迅速かつ容易になる。
*   **一貫性の確保:** 引数処理やエラーハンドリングのスタイルが統一される。
*   **バグの低減:** 定型的なコードの記述ミスが減る。
*   **可読性の向上:** 組み込み関数の仕様がアノテーションとして一箇所にまとまるため、見通しが良くなる。
*   **メンテナンス性の向上:** 仕様変更時の影響範囲がアノテーションと生成ツールに限定される。

## 今後の検討事項

*   アノテーションで表現できる型や引数のパターンの拡充 (例: `ANY` 型、特定インターフェースを満たす型)。
*   生成されるエラーメッセージのカスタマイズ性。
*   より高度な型推論 (例: `go_type` の自動判別)。
*   テストコードの自動生成の可能性。

## 実装上の課題と考慮点 (stringsパッケージ生成シミュレーションに基づく)

提案されたアノテーション方式を用いて `strings` パッケージの組み込み関数生成をシミュレートした結果、いくつかの課題が明らかになりました。

### 1. 複雑な引数構造を持つ関数の表現 (例: `strings.Join`)

現在の `strings.Join` は、MiniGoに配列型がないため、「最後の引数がセパレータで、それ以前の引数が結合対象の文字列」という特殊なシグネチャを持っています。これをアノテーションで表現する際に以下の問題があります。

*   **P1: 特殊な引数解釈のアノテーション構文:**
    *   `//minigo:arg index=-1 type=STRING` や `//minigo:arg index_range=0..-2 type=STRING` のようなカスタム構文は、アノテーションパーサーを複雑化させます。
    *   より宣言的な方法、例えば `//minigo:arg_pattern rule=last_is_separator pattern_var=separator_arg` や、引数グループの定義 (`//minigo:arg_group name=elements type=STRING variadic=true up_to=-2`) など、より高度なアノテーション機能の検討が必要です。
    *   あるいは、このような非常に特殊なケースでは、`wrapper_func` を使用することを前提とし、アノテーション側では引数の型と数を大まかに定義するに留める（例: `//minigo:args variadic=true` のみ）という割り切りも考えられます。この場合、詳細な引数チェックは `wrapper_func` 側の責任となります。

*   **P2: スタブ関数のシグネチャと実際のロジックの乖離:**
    *   `wrapper_func` を使用する場合、Goのスタブ関数のシグネチャ (例: `func joinStub(elements ...string) string`) が、MiniGoの実際の引数構造 (例: `obj1, obj2, ..., separatorObj`) と大きく異なる可能性があります。
    *   スタブ関数は主に存在確認や基本的な型ヒントに用いられると割り切れば許容範囲かもしれませんが、アノテーションとスタブ関数だけを見ても実際の挙動が把握しにくいという問題は残ります。ドキュメントの重要性が増します。

### 2. `wrapper_func` の依存関係とスコープ

*   **P3: `wrapper_func` の可視性とパッケージ依存:**
    *   アノテーションで `wrapper_func="main.evalStringsJoinOriginal"` のように、インタプリタのメインパッケージ内の関数を指定する場合、アノテーション処理ツールや生成されるコードがその関数に正しくアクセスできる必要があります。
    *   `builtins_src` ディレクトリが `main` パッケージとは別パッケージとして管理される場合（通常はそうなる）、生成コードが `main` パッケージの関数を呼び出すためには適切なインポートが必要です。生成ツールがこれを自動で解決するか、あるいは `wrapper_func` は特定のインターフェースを満たすように公開された関数であるべき、といった規約が必要になります。
    *   循環参照の問題も考慮に入れる必要があります。

### 3. 単純な関数の場合の利便性 (例: `strings.Contains`, `strings.ToUpper`)

*   `strings.Contains(s string, substr string) bool` や `strings.ToUpper(s string) string` のような、Go標準ライブラリ関数に直接マッピングできるものは、提案された `target_go_func` アノテーションで比較的スムーズに自動生成できると期待されます。
    ```go
    // builtins_src/strings_builtins.go
    package builtins_src
    import "strings"

    //minigo:builtin name="strings.Contains" target_go_func="strings.Contains"
    //minigo:arg index=0 name=s type=STRING go_type=string
    //minigo:arg index=1 name=substr type=STRING go_type=string
    //minigo:return type=BOOLEAN go_type=bool
    func stringsContainsStub(s string, substr string) bool { return false }
    ```
    この場合の主な作業は、MiniGoの型 (`STRING`, `BOOLEAN`) とGoの型 (`string`, `bool`) の間の定型的な変換コードの生成になります。

これらの課題を解決するためには、アノテーションの語彙を増やす、`wrapper_func` の使い方に関する規約を明確にする、生成ツールのロジックを洗練させるなどの対応が考えられます。

この提案により、MiniGoの組み込み関数開発がより堅牢かつ効率的になることを期待します。
```
