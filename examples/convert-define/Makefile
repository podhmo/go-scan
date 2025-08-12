.PHONY: test clean e2e update-golden

test:
	go test -v ./...

# The 'e2e' and 'test' targets are separate because in CI,
# the generation step (e2e) and the testing step (test) are run independently.
# The 'update-golden' target is for local development to update testdata.
update-golden:
	go test -v . -update

clean:
	rm -rf generated_test

e2e:
	go run . -file ./generated_test/mappings.go -output ./generated_test/generated.go
	go test -v -tags=e2e ./generated_test/...
