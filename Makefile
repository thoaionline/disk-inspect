.PHONY: build test check

build:
	go build -o disk-inspect .

test:
	go test -race ./...

check:
	go vet ./...
	go test -race ./...
