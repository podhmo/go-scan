.PHONY: test clean e2e

test: clean
	go test -v ./...

clean:
	rm -rf sampledata/generated

e2e:
	mkdir -p sampledata/generated
	go run . \
		-pkg github.com/podhmo/go-scan/examples/convert/sampledata/source \
		-output sampledata/generated/generated.go \
		-pkgname generated \
		-output-pkgpath github.com/podhmo/go-scan/examples/convert/sampledata/generated
	go build -v ./sampledata/...
