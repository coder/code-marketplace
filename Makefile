lint: lint/go
.PHONY: lint

lint/go:
	golangci-lint run
.PHONY: lint/go

test-clean:
	go clean -testcache
.PHONY: test-clean

test: test-clean
	gotestsum -- -v -short -coverprofile coverage ./...
.PHONY: test

coverage:
	go tool cover -func=coverage
.PHONY: coverage

gen:
	bash ./fixtures/generate.bash
.PHONY: gen

upload:
	bash ./fixtures/upload.bash
.PHONY: gen

TAG=$(shell git describe --always)
GO_SRC=$(shell find . -name '*.go' -type f)
LDFLAGS=-ldflags "-X github.com/coder/code-marketplace/buildinfo.tag=$(TAG)"
$(shell mkdir -p bin)

# Individual build targets for each OS/arch combination
bin/code-marketplace-mac-amd64: $(GO_SRC) go.mod go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/marketplace/main.go

bin/code-marketplace-mac-arm64: $(GO_SRC) go.mod go.sum
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/marketplace/main.go

bin/code-marketplace-linux-amd64: $(GO_SRC) go.mod go.sum
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/marketplace/main.go

bin/code-marketplace-linux-arm64: $(GO_SRC) go.mod go.sum
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/marketplace/main.go

bin/code-marketplace-windows-amd64: $(GO_SRC) go.mod go.sum
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $@ ./cmd/marketplace/main.go

bin/code-marketplace-windows-arm64: $(GO_SRC) go.mod go.sum
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $@ ./cmd/marketplace/main.go

# Main build target - builds all platforms
build: bin/code-marketplace-mac-amd64 \
       bin/code-marketplace-mac-arm64 \
       bin/code-marketplace-linux-amd64 \
       bin/code-marketplace-linux-arm64 \
       bin/code-marketplace-windows-amd64 \
       bin/code-marketplace-windows-arm64
.PHONY: build
