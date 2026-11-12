.PHONY: build test lint

build:
	go build -ldflags "-X main.version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" -o papercuts ./cmd/papercuts

test:
	go test ./...

lint:
	go vet ./...
