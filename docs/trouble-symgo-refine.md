# trouble: symgo refine

`examples/find-orphans` を `symgo` で実行すると無限再帰が発生する。

```
$ go run ./examples/find-orphans -format=json | jq .
...
```

logを仕込むと、`evalSelectorExpr` が `e.outer.outer...` のような形で無限に呼ばれていることがわかる。

```go
// symgo/evaluator/evaluator.go

func (e *Evaluator) evalSelectorExpr(ctx context.Context, n *ast.SelectorExpr, env *object.Environment, pkg *scanner.PackageInfo) object.Object {
	e.logger.Debug("evalSelectorExpr", "selector", n.Sel.Name)
...
}
```

```
{"level":"DEBUG","time":"2024-05-18T12:28:44.223+09:00","message":"evalSelectorExpr","selector":"outer"}
{"level":"DEBUG","time":"2024-05-18T12:28:44.223+09:00","message":"evalSelectorExpr","selector":"outer"}
{"level":"DEBUG","time":"2024-05-18T12:28:44.223+09:00","message":"evalSelectorExpr","selector":"outer"}
...
```

これは `minigo/object/object.go` の以下の `Env` 構造体の `Outer` fieldの解決で発生している。

```go
// minigo/object/object.go

// Environment holds the bindings for variables and functions.
type Environment struct {
	store map[string]Object
	outer *Environment
}
```

`e.outer.outer` のような構造を解決しようとすると、`evalSelectorExpr` が再帰的に呼ばれ、無限ループに陥る。
これを防ぐために、`evalSelectorExpr` に深さ制限を設ける必要があった。

---

## 解決策 (Solution)

この問題の根本的な原因は、`evalSelectorExpr` が `*object.Variable` 型の値を扱う際に、その変数が指す先のインスタンスの状態をチェックしていなかったことにある。
特に、変数がポインタを保持している場合（例：`*Environment`）、そのポインタを解決してインスタンスのフィールド値を取得する必要があった。

修正前の `evalSelectorExpr` は、インスタンスの状態（`State` map）を見ずに、型の情報（`TypeInfo`）だけを元にフィールドを解決しようとしていた。
その結果、`e.outer` のようなフィールドアクセスでは、実際のインスタンスが持つ `nil` 値が取得されず、代わりに新しいシンボリックプレースホルダーが返されてしまい、これが無限再帰を引き起こしていた。

修正では、`evalSelectorExpr` の `case *object.Variable:` ブロックを以下のように変更した：

1.  変数が保持している値（`val.Value`）を取得する。
2.  もし値がポインタ（`*object.Pointer`）であれば、一度解決（dereference）して、ポインタが指す先の値を取得する。
3.  その値がインスタンス（`*object.Instance`）であれば、まずインスタンスの `State` マップにセレクタ（フィールド名）が存在するかチェックする。
4.  存在すれば、その値を返す。これにより、`nil` のような実際のフィールド値が正しく返される。
5.  `State` マップに存在しない場合のみ、フォールバックとして型情報に基づいたフィールドやメソッドの検索を続ける。

この修正により、`e.outer` が `nil` の場合に正しく評価が終了し、無限再帰が解消された。
