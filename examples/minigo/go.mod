module github.com/podhmo/go-scan/examples/minigo

go 1.24.2

toolchain go1.24.3

//他のexamplesディレクトリを参考にreplaceディレクティブを追加
//ローカルのgo-scanパッケージを参照するようにします。
replace github.com/podhmo/go-scan => ../..

require (
	github.com/google/go-cmp v0.7.0
	github.com/podhmo/go-scan v0.0.0-20250801212757-b46a643f644b
)

require (
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
)
