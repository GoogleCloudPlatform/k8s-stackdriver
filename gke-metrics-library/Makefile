GOOS ?= linux

.PHONY: test

tidy:
	go mod tidy

build:
	$(ENVVAR) GOOS=$(GOOS) go build ./...

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

coverage:
	rm -f coverage.out
	go test -coverprofile=coverage.out ./...