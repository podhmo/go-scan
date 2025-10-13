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
-   **ステータス**: <span style="color:red; font-weight:bold">未解決</span>

### 現象

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際に、`symgo` エバリュエーターがエラーを発生させる。ツール自体はクラッシュしないが、ログにエラーが出力される。

-   **`TODO.md`での記述**: `undefined method or field: WithReceiver for pointer type INSTANCE`
-   **最小再現テストでのエラー**: `undefined method or field: WithReceiver for pointer type FUNCTION`

この問題は、関数型エイリアスに定義されたメソッドが、その型の変数へのポインタ経由で呼び出された場合に発生する。

### 調査ログ (2025-10-13)

1.  **最小再現テストの作成**: `symgo/evaluator/call_test.go` に、`... for pointer type FUNCTION` エラーを再現するテスト `TestEval_MethodCallOnFuncPointer` を追加した。
2.  **原因分析**: 詳細なデバッグにより、このエラーの根本原因は、関数リテラルが名前付きの関数型（例: `MyFunc`）の変数に代入される際に、その「名前付きの型」の情報 (`TypeInfo`) が結果の `*object.Function` に関連付けられていないことにあると特定した。
3.  **修正の試みと失敗**: `evalReturnStmt` を修正し、関数の返り値に `TypeInfo` を伝播させるアプローチを試みた。これにより最小再現テスト (`TestEval_MethodCallOnFuncPointer`) は成功した。
4.  **`find-orphans`での再検証**: しかし、この修正を適用した状態で `make -C examples/find-orphans` を実行したところ、ログには依然として `... for pointer type INSTANCE` エラーが出力された。
5.  **結論**: 私の修正は、より基本的な `FUNCTION` のケースは解決したが、`find-orphans` の自己分析時に発生する、より複雑な `INSTANCE` のケースは解決できなかった。根本原因は同じ（`TypeInfo`の欠落）だが、`*object.Function` が `*object.Instance` として誤認される、あるいは別のオブジェクトとして扱われる、より複雑な状態遷移が存在することを示唆している。

### 現状

問題は未解決。`evalReturnStmt` の修正は不完全であったため元に戻した。ただし、基本的な `FUNCTION` のケースを再現するテストケース (`TestEval_MethodCallOnFuncPointer`) は、将来のデバッグのために `t.Skip()` 付きで残しておく価値がある。