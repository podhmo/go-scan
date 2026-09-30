.PHONY: test clean demo

test:
	go test -v ./...

demo:
	go run ./ -f testdata/Taskfile.go -l
	go run ./ -f testdata/Taskfile.go Default

clean:
	rm -f testdata/app.out
