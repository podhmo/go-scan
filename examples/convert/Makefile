.PHONY: test clean e2e update-golden

test: e2e
	go test -v ./... ./generated_test

update-golden: e2e
	go test -v . -update

clean:
	rm -rf sampledata/generated

e2e:
	mkdir -p sampledata/generated
	go run . \
		-pkg github.com/podhmo/go-scan/examples/convert/sampledata/source \
		-output sampledata/generated/generated.go \
		-pkgname generated \
		-output-pkgpath github.com/podhmo/go-scan/examples/convert/sampledata/generated
	go install -v ./sampledata/...
