module github.com/podhmo/go-scan/examples/minigo

go 1.26.0

//他のexamplesディレクトリを参考にreplaceディレクティブを追加
//ローカルのgo-scanパッケージを参照するようにします。
replace github.com/podhmo/go-scan => ../..

require (
	github.com/google/go-cmp v0.7.0
	github.com/podhmo/go-scan v0.0.3
)

require (
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
)
