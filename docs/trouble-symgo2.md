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
-   **ステータス**: <span style="color:orange; font-weight:bold">部分的修正</span>

### 現象

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際、`e2e` テスト (`make -C examples/find-orphans e2e`) を実行すると、`undefined method or field: WithReceiver for pointer type INSTANCE` というエラーが出力される。

このエラーは、`interface{}` 型に格納された `*object.Function` を `type-switch` で元の型にキャストしようとした際に発生する。

### 調査と部分的な修正

1.  **根本原因の特定**: 問題の根本原因は、`symgo/scanner` パッケージが `type MyFunc func()` のような関数型エイリアスをパースする際に、そのエイリアス名 (`MyFunc`) とパッケージパスを、`TypeInfo` が内包する `FunctionInfo` 構造体に正しく伝播させていなかったことにあると特定した。これにより、`symgo/evaluator` はこの型の `TypeInfo` を解決できずにいた。

2.  **`scanner` の修正**: `scanner/scanner.go` の `fillTypeInfoFromSpec` 関数を修正し、関数型エイリアスの名前とパッケージパスを、内包する `FunctionInfo` に伝播させるようにした。この修正は、専用の単体テスト (`scanner_func_alias_test.go`) によって検証され、`scanner` レイヤーの問題が解決したことが確認された。

3.  **残存する `evaluator` の問題**: しかし、`scanner` の修正後も、`find-orphans` のe2eテストでは依然として同じエラーが発生する。これは、`scanner` が正しい `TypeInfo` を提供するようになったにもかかわらず、`evaluator` がその情報を `type-switch` の評価に至るまでのどこかの段階で失っているか、あるいは正しく利用できていないことを示している。

### 次のステップ

`evaluator` 側のデバッグを継続し、`evalGenDecl` や `evalAssignStmt` などで、`scanner` から渡された `TypeInfo` がどのように処理されているかを追跡する必要がある。問題は `scanner` と `evaluator` の両方にまたがる、より複雑なものであることが判明した。

## `type-switch` で `*object.Function` の型情報が失われる問題

-   **発見日**: 2025-10-13
-   **関連**: `find-orphans`, `symgo`
-   **ステータス**: <span style="color:green; font-weight:bold">解決済み</span> (2025-10-13)

### 現象

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際、`e2e` テスト (`make -C examples/find-orphans e2e`) を実行すると、`undefined method or field: WithReceiver for pointer type INSTANCE` というエラーが出力されていた。

このエラーは、`interface{}` 型に格納された `*object.Function` を `type-switch` で元の型にキャストしようとした際に発生していた。

### 原因

根本原因は、`symgo/scanner` パッケージが `type MyFunc func()` のような関数型エイリアスをパースする際に、そのエイリアス名 (`MyFunc`) とパッケージパスを、`TypeInfo` が内包する `FunctionInfo` 構造体に正しく伝播させていなかったことにある。

これにより、`symgo/evaluator` が `MyFunc` 型の `TypeInfo` を解決しようとしても、`scanner` から得られる情報にエイリアス名が含まれていないため、解決に失敗していた。その結果、`evalGenDecl` で `*object.Function` に正しい `TypeInfo` が設定されず、後の `evalTypeSwitchStmt` での型比較が失敗し、`default` 節にフォールバックして `*object.Instance` が生成され、エラーを引き起こしていた。

### 解決策

`scanner/scanner.go` の `fillTypeInfoFromSpec` 関数を修正した。`*ast.FuncType` を処理するケースで、親の `TypeInfo` が持つエイリアス名とパッケージパスを、新しく生成される `FunctionInfo` に明示的にコピーするようにした。

この修正により、`scanner` は関数型エイリアスの情報を完全に保持するようになり、`evaluator` はそれを正しく解決できるようになった。結果として、`evaluator` 側の修正は不要となり、`scanner` の修正のみで `find-orphans` のe2eテストが成功することが確認された。