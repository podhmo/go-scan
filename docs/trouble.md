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

*   **`main.go` の `-pkg` オプションで複数パッケージを扱えるようにする:**
    *   現状、`-pkg` オプションは単一のパッケージしか受け付けません。`go-scan` が複数のパッケージをスキャンできるように、`-pkg` オプションをカンマ区切りなどで複数指定できるように修正する必要があります。

*   **`parser.Parse` で、`@derivingconvert` に指定された外部パッケージを自動的にスキャンする:**
    *   `main.go` で明示的に `destination` パッケージをスキャンするのではなく、`parser.Parse` が `@derivingconvert` アノテーションを解析し、必要に応じて `goscan.Scanner.ScanPackageByImport` を呼び出して外部パッケージをスキャンするように修正するのが、よりクリーンな解決策です。

*   **`generator.go` の型名解決の改善:**
    *   `getTypeName` 関数が、異なるパッケージの型名を正しく解決できていません。`ImportManager` の使い方を見直すか、あるいは `ImportManager` に頼らない方法で型名を解決する必要があります。

*   **`generator.go` の変換ロジックの改善:**
    *   `generateConversion` 関数が、ポインタ、スライス、マップなどの複雑な型を正しく変換できていません。特に、`time.Time` から `string` への変換など、型が異なる場合の変換ロジックを拡充する必要があります。

*   **`main_test.go` のインテグレーションテストの修正:**
    *   インテグレーションテストが、一時的なディレクトリで実行されるために `go.mod` の情報を正しく読み込めていません。テスト実行時に `go.mod` の情報を正しく渡すか、あるいはテストの前提条件を見直す必要があります。

## 当初の見通しとの差異と実装の不備

当初は、`go-scan` が `@derivingconvert` アノテーションに書かれたインポートパスを自動的に解決し、必要なパッケージをスキャンしてくれるものと想定していました。しかし、実際には `-pkg` オプションで指定されたパッケージしかスキャンせず、これが問題の根本原因でした。

`main.go` で複数のパッケージをスキャンしてマージするというアプローチは、この問題を回避するための場当たり的な対応であり、`go-scan` の設計思想とは異なっていた可能性があります。その結果、`parser` や `generator` で、マージされた `PackageInfo` を正しく扱いきれず、多数のコンパイルエラーを引き起こしました。

`go-scan` のコアな機能を修正するのではなく、`go-scan` の使い方を工夫するか、あるいは `go-scan` の機能拡張として、複数のパッケージをスキャンする仕組みを正式に導入する、といったアプローチの方が、よりクリーンな解決に繋がるかもしれません。
