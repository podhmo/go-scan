.PHONY: test clean e2e update-golden

test: clean
	go test -v ./...

# The 'e2e' and 'test' targets are separate because in CI,
# the generation step (e2e) and the testing step (test) are run independently.
# The 'update-golden' target is for local development to update testdata.
update-golden:
	go test -v . -update

clean:
	rm -rf sampledata/generated
	rm -rf generated_test

e2e:
	mkdir -p sampledata/generated
	go run . \
		-pkg github.com/podhmo/go-scan/examples/convert/sampledata/source \
		-output sampledata/generated/generated.go \
		-pkgname generated \
		-output-pkgpath github.com/podhmo/go-scan/examples/convert/sampledata/generated
	go install -v ./sampledata/...
