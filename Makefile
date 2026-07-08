BINARY := skillrunner
PKG := ./cmd/skillrunner

.PHONY: build all test clean install

# Build for the current platform into bin/skillrunner
build:
	go build -o bin/$(BINARY) $(PKG)

# Cross-compile for macOS (arm64/amd64), Linux, and Windows
all:
	GOOS=darwin  GOARCH=arm64 go build -o bin/$(BINARY)-darwin-arm64 $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o bin/$(BINARY)-darwin-amd64 $(PKG)
	GOOS=linux   GOARCH=amd64 go build -o bin/$(BINARY)-linux-amd64 $(PKG)
	GOOS=windows GOARCH=amd64 go build -o bin/$(BINARY).exe $(PKG)

test:
	go vet ./...
	go test ./...

# Copy the current-platform binary into $GOBIN or /usr/local/bin
install: build
	go install $(PKG)

clean:
	rm -rf bin
