# `generated` パッケージの変換関数生成に関する問題

## 概要

`examples/convert/sampledata/generated` パッケージを作成し、`source` パッケージから `destination` パッケージへの変換関数を自動生成しようとしましたが、生成されたコードにコンパイルエラーが残り、テストをパスさせることができませんでした。

## 目標

`examples/convert/main.go` を実行し、`examples/convert/sampledata/source` に定義された型から `examples/convert/sampledata/destination` に定義された型への変換関数を、`examples/convert/sampledata/generated` パッケージに `generated.go` として出力する。

## 実行したこと

1.  **`generated` パッケージの作成:**
    *   `examples/convert/sampledata/generated` ディレクトリを作成しました。

2.  **`source.go` へのアノテーション追加:**
    *   `examples/convert/sampledata/source/source.go` の `SrcInternalDetail` と `SubSource` に `@derivingconvert` アノテーションを追加し、これらの型に対応する変換関数が生成されるようにしました。

3.  **`main.go` の修正:**
    *   当初、`go-scan` が `source` パッケージしかスキャンしないため、`destination` パッケージの型情報が見つけられないという問題がありました。
    *   これを解決するため、`main.go` の `run` 関数を修正し、`source` と `destination` の両方のパッケージを `goscan.Scanner.ScanPackage` でスキャンし、その結果（`scanner.PackageInfo`）をマージしてから `parser.Parse` に渡すように試みました。

4.  **`parser/parser.go` の修正:**
    *   `main.go` の修正に伴い、`parser.Parse` がマージされた `PackageInfo` を正しく解釈できるように、`parser.Parse` とその内部で呼び出される関数を修正しました。
    *   具体的には、`dstTypeName` からパッケージパスと型名を分割し、マージされた `PackageInfo.Types` の中から目的の型を探すように変更しました。

5.  **`generator/generator.go` の修正:**
    *   生成されるコードの型名が、パッケージプレフィックスなしで出力される問題があったため、`getTypeName` 関数を修正し、`ImportManager` を使って正しくパッケージ名を修飾するようにしました。
    *   `time.Time` から `string` への変換が定義されていなかったため、`generateConversion` 関数に変換ロジックを追加しました。
    *   ポインタ、スライス、マップの変換ロジックを修正し、より堅牢にしました。

## 残っている課題 (TODO)

*   **生成されたコードのコンパイルエラー:**
    *   `go run` で `generated.go` を生成すると、型の不一致や未定義の関数呼び出しなどのコンパイルエラーが多数発生します。
    *   これは、`generator.go` が、複数のパッケージにまたがる型変換を依然として正しく扱えていないことが原因です。

*   **`main_test.go` のインテグレーションテストの失敗:**
    *   `go test` を実行すると、`main_test.go` のインテグレーションテストがすべて失敗します。
    *   エラーメッセージは `could not find destination package dir for ...` となっており、テスト実行時に `destination` パッケージの場所を解決できていないことを示しています。これは、テストが一時的なディレクトリで実行され、`go.mod` のコンテキストが失われるためだと考えられます。

## 今後のデバッグ方針

*   **`generator.go` の `getTypeName` と `generateConversion` の再調査:**
    *   これらの関数が、`source` と `destination` の両方のパッケージの型を、パッケージプレフィックス付きで正しく扱えるように、さらにデバッグが必要です。
    *   特に、`generateConversion` の中で、異なるパッケージの型同士を代入する際のロジックに問題がある可能性が高いです。

*   **`ImportManager` の役割の再確認:**
    *   `generator.go` で `ImportManager` を使って型名を修飾していますが、その使い方、あるいは `ImportManager` の機能自体が、今回のユースケースに合っていない可能性があります。`ImportManager` がどのようにインポートパスを管理し、エイリアスを生成しているのかを再確認する必要があります。

*   **`main_test.go` の修正:**
    *   インテグレーションテストが通るように、テストコードを修正する必要があります。テスト実行時に `go.mod` の情報を正しく渡す方法を調査するか、テストの前提条件を見直す必要があります。

## 当初の見通しとの差異と実装の不備

当初は、`go-scan` が `@derivingconvert` アノテーションに書かれたインポートパスを自動的に解決し、必要なパッケージをスキャンしてくれるものと想定していました。しかし、実際には `-pkg` オプションで指定されたパッケージしかスキャンせず、これが問題の根本原因でした。

`main.go` で複数のパッケージをスキャンしてマージするというアプローチは、この問題を回避するための場当たり的な対応であり、`go-scan` の設計思想とは異なっていた可能性があります。その結果、`parser` や `generator` で、マージされた `PackageInfo` を正しく扱いきれず、多数のコンパイルエラーを引き起こしました。

`go-scan` のコアな機能を修正するのではなく、`go-scan` の使い方を工夫するか、あるいは `go-scan` の機能拡張として、複数のパッケージをスキャンする仕組みを正式に導入する、といったアプローチの方が、よりクリーンな解決に繋がるかもしれません。
