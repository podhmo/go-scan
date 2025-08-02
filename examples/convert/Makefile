.PHONY: e2e
e2e:
	@mkdir -p sampledata/generated
	go run . \
		-pkg github.com/podhmo/go-scan/examples/convert/sampledata/source \
		-output sampledata/generated/generated.go \
		-pkgname generated
