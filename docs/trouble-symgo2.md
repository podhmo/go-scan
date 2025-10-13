# `symgo` のトラブルシューティング (2)

## `panic(nil)` がクラッシュを引き起こす問題

-   **発見日**: 2024-07-25
-   **関連**: `goinspect`, `find-orphans`
-   **ステータス**: <span style="color:green; font-weight:bold">解決済み</span> (2024-07-25)

### 現象

`symgo` が `panic(nil)` を含むコードを分析すると、`evaluator.Eval` 内で Go のランタイムパニック（nil ポインタ逆参照）が発生し、分析全体がクラッシュする。

### 原因

`evalPanicStmt` は `panic` の引数を評価するが、その引数が `nil` の場合、`e.Eval` は `object.NIL` を返す。しかし、`evalPanicStmt` の後続のコードは、この返り値が常に有効なオブジェクト（`Inspect()` メソッドを持つ）であることを期待していたため、`object.NIL` の `Inspect()` を呼び出そうとしてランタイムパニックが発生していた。

```go
// symgo/evaluator/evaluator.go: evalPanicStmt
func (e *Evaluator) evalPanicStmt(ctx context.Context, n *ast.PanicStmt, env *object.Environment, pkg *scan.PackageInfo) object.Object {
	// ...
	val := e.Eval(ctx, n.X, env, pkg)
	if isError(val) {
		return val
	}
	// ここで `val` が object.NIL になりうる
	e.logc(ctx, slog.LevelDebug, "panic statement evaluated", "value", val.Inspect()) // val.Inspect() でパニック
	return &object.PanicError{Value: val}
}
```

### 解決策

`evalPanicStmt` 内で、`e.Eval` から返された値が `object.NIL` であるかどうかをチェックする処理を追加した。`object.NIL` の場合は、`PanicError` の `Value` フィールドにそのまま `object.NIL` を設定する。これにより、`Inspect()` の呼び出しが回避され、クラッシュしなくなった。

## `find-orphans` が自己分析時にクラッシュする問題

-   **発見日**: 2024-07-26
-   **関連**: `find-orphans`, `symgo`
-   **ステータス**: <span style="color:green; font-weight:bold">解決済み</span> (2025-10-13)

### 現象

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際、またはそれと同様の状況で、関数型エイリアスに定義されたメソッドがポインタ経由で呼び出されると、エバリュエーターが `undefined method or field` エラーを発生させていた。

### 原因

根本原因は、関数リテラルが、名前付きの関数型（例: `type MyFunc func()`）を返り値とする関数から返される際に、その名前付きの型の情報 (`TypeInfo`) が、結果の `*object.Function` に関連付けられていなかったことにある。

```go
func getFn() MyFunc { // MyFunc は type MyFunc func()
    return func() {} // この時点で返される *object.Function は MyFunc の TypeInfo を持たない
}

func main() {
    fn := getFn() // fn の *object.Variable には MyFunc の型情報があるが...
    fn_ptr := &fn
    fn_ptr.WithReceiver() // ここで fn_ptr の先の Function の TypeInfo が見つからずエラー
}
```

このため、後続の `evalSelectorExpr` がメソッドを解決しようとしても、レシーバの `TypeInfo` が見つからず、失敗していた。

### 解決策

`evalReturnStmt` を修正し、値が返される際に、その値が `*object.Function` であり、かつ呼び出し元の関数のシグネチャが名前付きの関数型を返すよう定義されている場合、その名前付きの型の `TypeInfo` を `*object.Function` に設定するようにした。

この修正により、`*object.Function` は自身の型情報を正しく保持するようになり、後続のメソッド呼び出しが正しく解決されるようになった。この修正で、`find-orphans` の自己分析時に発生していた `INSTANCE` エラーも解消されたことから、より複雑な状況で発生していたエラーも、この `TypeInfo` の欠落が根本原因であったことが確認された。

## `type-switch` で `*object.Function` の型情報が失われる問題

-   **発見日**: 2025-10-13
-   **関連**: `find-orphans`, `symgo`
-   **ステータス**: <span style="color:red; font-weight:bold">未解決</span>

### 現象

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際、`e2e` テスト (`make -C examples/find-orphans e2e`) を実行すると、`undefined method or field: WithReceiver for pointer type INSTANCE` というエラーが出力される。

このエラーは、`interface{}` 型に格納された `*object.Function` を `type-switch` で元の型にキャストしようとした際に発生する。

### 調査の経緯と結論

当初は `symgo/evaluator` の `evalTypeSwitchStmt` や `evalGenDecl` に問題があると仮説を立て、修正を試みたが解決しなかった。

より詳細な調査の結果、問題の根本原因は `symgo/scanner` パッケージにあると特定された。

1.  **`scanner` の問題の特定**: `type MyFunc func()` のような関数型エイリアスをスキャンする単体テストを作成したところ、`scanner` が生成した `TypeInfo` の `Func` フィールド (`*scanner.FunctionInfo`) に、エイリアス名 (`MyFunc`) とパッケージパスが設定されていないことが判明した。これにより、後続の `symgo` の評価器がこの型を正しく解決できず、`nil` を返していた。これが、`evalGenDecl` で `TypeInfo` が設定されない直接の原因であった。
2.  **`scanner` の修正**: `scanner/scanner.go` の `fillTypeInfoFromSpec` 関数を修正し、関数型エイリアスの名前とパッケージパスを、内包する `FunctionInfo` に伝播させるようにした。これにより、`scanner` の単体テストは成功した。
3.  **問題の残留**: しかし、`scanner` の修正だけでは、`find-orphans` のe2eテストは依然として失敗した。これは、`scanner` の修正によって `evaluator` が `TypeInfo` を解決できるようになったものの、その後の `evaluator` 内での `TypeInfo` のハンドリング（特に `evalGenDecl` と `evalTypeSwitchStmt`）にも別の問題が存在することを示唆している。

`scanner` と `evaluator` の両方にまたがる複雑な問題であり、単純な修正では解決できないことが確認された。この問題の完全な解決には、両レイヤー間での型情報の受け渡しに関する、より広範な見直しが必要である。