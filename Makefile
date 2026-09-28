VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/useless-husband/macdust/internal/cli.Version=$(VERSION)

.PHONY: build test vet release clean

build:
	go build -ldflags "$(LDFLAGS)" -o macdust .

test:
	go vet ./...
	go test ./...

# Cross-compile into dist/ (not tracked by git).
release: clean
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/macdust-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/macdust-darwin-amd64 .
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dist/macdust-linux-amd64 .
	cd dist && shasum -a 256 macdust-* > SHA256SUMS

clean:
	rm -rf dist macdust
