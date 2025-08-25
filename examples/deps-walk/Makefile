e2e:
	mkdir -p docs
	go run ./ -direction bidi -format dot -short ../../ > docs/goscan-bidi.dot
	go run ./ -direction bidi -format mermaid -short ../../ > docs/goscan-bidi.mermaid.txt
	go run ./ -direction bidi -format json -short ../../ > docs/goscan-bidi.json
.PHONY: e2e