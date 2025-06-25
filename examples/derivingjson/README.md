# Deriving JSON for oneOf

`derivingjson` は、`github.com/podhmo/go-scan` ライブラリの機能を利用して、JSON の `oneOf` スキーマに類する構造を持つ Go の struct に対する `UnmarshalJSON` メソッドを自動生成する実験的なツールです。

## 概要

JSON スキーマにおける `oneOf` は、あるフィールドが複数の型を取りうることを表現します。Go でこれを素直に表現しようとすると、インターフェース型と、そのインターフェースを実装する具体的な struct 群、そして型を判別するための識別子フィールド（discriminator）を持つコンテナ struct のような形になることが一般的です。

このツールは、そのようなコンテナ struct に対して、識別子の値に基づいて適切な具象型にアンマーシャルするための `UnmarshalJSON` メソッドを生成することを目的としています。

## 特徴

-   `github.com/podhmo/go-scan` を利用した型情報の解析。
-   特定のコメントアノテーション (`@deriving:unmarshall`) が付与された struct を対象とします。
-   識別子フィールド (例: `Type string `json:"type"``) と `oneOf` 対象のインターフェースフィールドを特定し、適切なアンマーシャリングロジックを生成します。
-   インターフェースを実装する具象型は、ツールが同一パッケージ内から探索します。

## 使用方法 (想定)

1.  `UnmarshalJSON` を生成したいコンテナ struct のコメントに `@deriving:unmarshall` を追加します。
2.  コマンドラインから `derivingjson` を実行し、対象のパッケージパスを指定します。

   ```bash
   go run examples/derivingjson/*.go <対象パッケージのパス>
   # またはビルド後
   # ./derivingjson <対象パッケージのパス>
   ```

   例:
   ```bash
   go run examples/derivingjson/*.go ./examples/derivingjson/testdata/simple
   ```
3.  指定されたパッケージ内に `xxx_deriving.go` のような名前で `UnmarshalJSON` メソッドが実装されたファイルが生成されます。

## 注意事項

このツールは `go-scan` ライブラリの試用とデモンストレーションを兼ねた実験的なものです。
