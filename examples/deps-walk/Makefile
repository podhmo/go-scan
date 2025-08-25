e2e:
	mkdir -p docs
	go run ./ -direction bidi -hops 3 -format dot -short ../../ > docs/goscan-bidi.dot
	go run ./ -direction bidi -hops 3 -format mermaid -short ../../ > docs/goscan-bidi.mermaid.txt
	go run ./ -direction bidi -hops 3 -format json -short ../../ > docs/goscan-bidi.json
.PHONY: e2e