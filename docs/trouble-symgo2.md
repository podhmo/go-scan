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

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際に、`symgo` エバリュエーターがクラッシュする。`TODO.md` によると、エラーは `undefined method or field: WithReceiver for pointer type INSTANCE` とされているが、最小再現テストケースでは `undefined method or field: WithReceiver for pointer type FUNCTION` というエラーが発生する。

この問題は、関数型エイリアスに定義されたメソッドが、その型の変数へのポインタ経由で呼び出された場合に発生する。

### 調査ログ (2025-10-13)

1.  **テストケース**: `symgo/evaluator/call_test.go` に、問題を再現するためのテスト `TestEval_MethodCallOnFuncPointer` を追加済み。

    ```go
    // main.go の抜粋
    type MyFunc func()
    func (f *MyFunc) WithReceiver() {}

    func main() {
        fn := getFn() // MyFunc 型の関数リテラルを返す
        fn_ptr := &fn
        fn_ptr.WithReceiver() // ここでエラーが発生する
    }
    ```

2.  **詳細デバッグの実施**: `newError` を一時的に変更し、エラー発生時の解析コールスタックとGoランタイムスタックをファイルに出力するようにした。

3.  **ログ分析**:
    -   **Goランタイムスタック**: `applyFunctionImpl` -> `evalCallExpr` -> `evalSelectorExpr` の順で呼び出され、`evalSelectorExpr` 内で `newError` が呼ばれていることを確認。これは想定通りの動作。
    -   **symgo解析スタック**: エラー発生時、解析対象のコードは `main` 関数内にいることを確認。
    -   **結論**: これまでの分析通り、`fn_ptr.WithReceiver()` のレシーバ (`fn_ptr`) を評価した結果である `*object.Pointer` が指す `*object.Function` に `TypeInfo` が設定されていないことが根本原因であると再確認した。

4.  **根本原因の深掘り**:
    -   `TypeInfo` が失われるのは、`evalSelectorExpr` よりも前の段階である。
    -   `fn := getFn()` という代入文が評価される際、`getFn()` が返す関数リテラル (`func() {}`) の評価結果である `*object.Function` は、それが `MyFunc` 型であるという文脈を持っていない。
    -   `evalReturnStmt` を確認したが、ここでも型情報を付与する処理は行われていない。
    -   `assignIdentifier` での代入時に `TypeInfo` を伝播させる修正を試みたが、`:=` の場合、左辺の変数の型情報がまだ確定していないため、右辺の `*object.Function` に正しい `TypeInfo` を設定できなかった。

### 現状の結論

問題の根本原因は、関数リテラルが評価され、名前付きの関数型（例: `MyFunc`）の変数に代入されるまでの一連の評価フローのどこかで、その「名前付きの型」の情報が `*object.Function` オブジェクトに伝播・関連付けられていないことにある。

この問題を解決するには、`evalReturnStmt` や `assignIdentifier` を含む、より広範な評価フローの見直しが必要となる可能性が高い。`evalSelectorExpr` のみの修正では解決は困難。