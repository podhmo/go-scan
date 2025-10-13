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

### 調査ログ

1.  **テストケースの作成**: `symgo/evaluator/call_test.go` に、問題を再現するためのテスト `TestEval_MethodCallOnFuncPointer` を追加した。このテストは、`type MyFunc func()` という型を定義し、そのポインタレシーバ `(f *MyFunc) WithReceiver()` を持つメソッドを定義する。`main` 関数内で、この型の変数をポインタ経由で呼び出す。

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

2.  **エラーの確認**: テストを実行し、期待通り以下のエラーで失敗することを確認した。
    `symgo runtime error: undefined method or field: WithReceiver for pointer type FUNCTION`

3.  **原因分析 (1) - `evalSelectorExpr`**: `TODO.md` の指示に基づき、`symgo/evaluator/evaluator_eval_selector_expr.go` の `case *object.Pointer:` ブロックを修正しようと試みた。ポインティが `*object.Function` である場合の処理を追加したが、状況は改善しなかった。

4.  **原因分析 (2) - `TypeInfo` の欠落**: デバッグログを追加したところ、エラー発生時点でセレクタの左辺（`fn_ptr`）が指す `*object.Function` に `TypeInfo` が設定されていないことが判明した。`TypeInfo` がなければ、`accessor.findMethodOnType` はメソッドを見つけることができない。

5.  **原因分析 (3) - `assignIdentifier`**: `TypeInfo` が設定されるべき場所は、値が変数に代入される時点であると推測。`symgo/evaluator/evaluator_eval_assign_stmt.go` の `assignIdentifier` 関数を修正し、`:=` での代入時に `MyFunc` 型の `TypeInfo` が `*object.Function` に伝播するように試みた。しかし、リファクタリングの過程でビルドエラーを繰り返し、最終的に修正を適用してもテストは成功しなかった。

    -   **課題**: `fn := getFn()` のような代入（`token.DEFINE`）において、左辺の変数 `fn` の型（`MyFunc`）が `scanner` によって正しく解決されているにもかかわらず、その情報が `*object.Variable` の `FieldType` に設定されていない、あるいは設定されていても、右辺の `*object.Function` オブジェクトに伝播していない。この型情報の伝播が `symgo` の評価フローのどこかで行われるべきだが、その正確な場所の特定には至っていない。

### 現状の結論

問題の根本原因は、関数リテラルが名前付きの関数型（例: `MyFunc`）の変数に代入される際に、その名前付きの型の情報（`TypeInfo`）が結果の `*object.Function` に関連付けられていないことにある。このため、後続のメソッド呼び出しの解決時に、レシーバの型情報が欠落し、メソッドが見つからずにエラーとなる。

`assignIdentifier` での修正が最も確実なアプローチと思われるが、現在の実装は複雑であり、正しい修正箇所の特定にはさらなる詳細なデバッグと `symgo` の評価フロー全体の深い理解が必要である。これ以上の試行は非効率と判断し、一旦調査を中断する。