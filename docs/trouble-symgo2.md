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

根本原因は、`type MyFunc func()` のような関数型エイリアスが `scanner` によって正しくパースされておらず、その結果 `symgo` の評価器が `MyFunc` の `TypeInfo` を解決できずに `nil` を返してしまうことにある。

1.  **問題の再現**: `type MyFunc func()` を定義し、その型の変数を `interface{}` に代入した後、`type-switch` で `MyFunc` にキャストバックする単体テストを作成した。このテストは、`case MyFunc:` にマッチせず、`default:` にフォールバックすることで失敗を再現した。
2.  **原因の追跡**: 詳細なデバッグログにより、`evalGenDecl` 内で `MyFunc` 型を `resolver.ResolveType` で解決しようとした際に `nil` が返されていることを特定した。
3.  **根本原因の特定**: `resolver.ResolveType` は内部で `scanner` の `fieldType.Resolve()` を呼び出している。`scanner` の `fillTypeInfoFromSpec` 関数が `type MyFunc func()` のような宣言を処理する際、生成される `TypeInfo` には `Kind = FuncKind` が設定されるが、`TypeInfo` の中の `Func` フィールド (`*FunctionInfo`) に、エイリアス名(`MyFunc`) やパッケージパスといった重要な情報が伝播されていなかった。これにより、後続の `Lookup` が失敗していた。
4.  **修正の試み**: `scanner/scanner.go` の `fillTypeInfoFromSpec` を修正し、`typeInfo` から `funcInfo` へ名前とパッケージパスを伝播させた。しかし、この修正だけではテストは成功せず、`evaluator` 側にも変更が必要であることが示唆された。
5.  **evaluator側の修正**: `evalGenDecl` と `evalTypeSwitchStmt` にも修正を加えたが、問題は解決しなかった。複数のレイヤーにまたがる複雑な状態管理の問題であり、単純な修正では対応できないことが判明した。

### 結論

この問題は、`scanner` が関数型エイリアスの情報を `FunctionInfo` に完全に伝播させていないことに起因するが、それを修正しても `evaluator` 側での `TypeInfo` のハンドリングにさらなる問題があり、解決には至っていない。`scanner` と `evaluator` の間の型情報の受け渡しに関する、より広範な見直しが必要である。