BINARY := c2c
MODULE := github.com/YangZhengCQ/Claw2cli
GOFLAGS := -trimpath
COVERAGE_OUT := coverage.out

.PHONY: build test test-verbose coverage coverage-html coverage-func lint clean install shim-test

build:
	go build $(GOFLAGS) -o $(BINARY) .

test:
	go test -race ./...

test-verbose:
	go test -race -v ./...

coverage:
	go test -coverprofile=$(COVERAGE_OUT) -covermode=atomic ./...

coverage-html: coverage
	go tool cover -html=$(COVERAGE_OUT) -o coverage.html
	@echo "Open coverage.html in a browser to view the report."

coverage-func: coverage
	go tool cover -func=$(COVERAGE_OUT)

shim-test:
	cd shim && node --test test/*.test.js

lint:
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || go vet ./...

clean:
	rm -f $(BINARY) $(COVERAGE_OUT) coverage.html

install:
	go install $(GOFLAGS) .
	@echo "Note: You may need to copy the shim/ directory to your install location."
	@echo "  For Homebrew installs, GoReleaser handles this automatically."
	@echo "  For manual installs: cp -r shim/ $$(go env GOPATH)/libexec/shim/"
