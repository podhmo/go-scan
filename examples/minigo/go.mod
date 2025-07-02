module github.com/podhmo/go-scan/examples/minigo

go 1.24

//他のexamplesディレクトリを参考にreplaceディレクティブを追加
//ローカルのgo-scanパッケージを参照するようにします。
replace github.com/go-scan/go-scan => ../..

require github.com/go-scan/go-scan v0.0.0-00010101000000-000000000000
