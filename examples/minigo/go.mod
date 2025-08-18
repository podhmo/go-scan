module github.com/podhmo/go-scan/examples/minigo

go 1.24

//他のexamplesディレクトリを参考にreplaceディレクティブを追加
//ローカルのgo-scanパッケージを参照するようにします。
replace github.com/podhmo/go-scan => ../..

require github.com/podhmo/go-scan v0.0.0-20250801212757-b46a643f644b

require (
	github.com/google/go-cmp v0.7.0 // indirect
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
)
