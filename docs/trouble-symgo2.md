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

`find-orphans` ツールが自身のコードベースを分析する（メタサーキュラー分析）際、`e2e` テスト (`make -C examples/find-orphans e2e`) を実行すると、以下のエラーログが出力される。

```
level=ERROR msg="undefined method or field: WithReceiver for pointer type INSTANCE"
```

このエラーは、`symgo` が自身の評価器のコード (`accessor.go` など) を分析している際に発生する。具体的には、`interface{}` 型に格納された `*object.Function` を `type-switch` で元の型にキャストしようとした際、キャスト後のオブジェクトが `*object.Function` ではなく、汎用的な `*object.Instance` になってしまい、`WithReceiver` メソッドの呼び出しに失敗している。

### 調査の経緯と課題

1.  **仮説**: `evalTypeSwitchStmt` の `case` 節の評価ロジックが、`*object.Function` のような `symgo` の内部オブジェクトを特別扱いしておらず、常に新しい `*object.Instance` や `*object.SymbolicPlaceholder` を生成してしまうことが原因であると仮定した。
2.  **修正案**: `case` 節の型と `switch` 対象オブジェクトの具象型が一致する場合、新しいオブジェクトを生成する代わりに、元のオブジェクトを `Clone()` して型情報のみを更新するロジックを `evalTypeSwitchStmt` に追加した。
3.  **問題の深化**: 上記の修正を加えても、テストは成功しなかった。デバッグログを追加して調査したところ、`switch` 対象オブジェクト (`originalObj`) の `TypeInfo.Name` が空文字列 `""` になっており、型比較が失敗していることが判明した。
4.  **さらなる調査**: `originalObj` は `var fn MyFunc = func() {}` のように宣言された変数が `interface{}` を返す関数から返されたものであった。
    -   `evalGenDecl` で変数 `fn` が宣言される際に、`*object.Function` オブジェクトに `MyFunc` の `TypeInfo` を設定する修正を追加した。
    -   しかし、デバッグログで確認したところ、`evalGenDecl` の時点では `TypeInfo` が正しく設定されているように見えるにもかかわらず、その後の `evalTypeSwitchStmt` の時点では、**全く同じメモリアドレスのオブジェクトであるにもかかわらず `TypeInfo.Name` が失われている**という不可解な現象が確認された。

### 結論

`evalGenDecl` で設定された `TypeInfo` が、`evalTypeSwitchStmt` に到達するまでの間に、予期せぬ副作用によって失われている可能性が非常に高い。しかし、その原因となる箇所の特定には至らなかった。`*object.Variable` と `*object.Function` の間での値の受け渡しや、`interface{}` 型への代入を処理する `evalAssignStmt` など、複数の箇所が関連している可能性が考えられるが、これ以上の調査は困難と判断した。

この問題の解決には、`symgo` のオブジェクトモデルと状態管理に関する、より深いレベルの理解が必要となる。