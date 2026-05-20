.PHONY: build test lint tidy fmt

build:
	go build ./...

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

fmt:
	gofmt -w .
	goimports -w .
