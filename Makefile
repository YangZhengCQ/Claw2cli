BINARY := c2c
MODULE := github.com/user/claw2cli
GOFLAGS := -trimpath
COVERAGE_OUT := coverage.out

.PHONY: build test test-verbose coverage coverage-html coverage-func lint clean install

build:
	go build $(GOFLAGS) -o $(BINARY) .

test:
	go test ./...

test-verbose:
	go test -v ./...

coverage:
	go test -coverprofile=$(COVERAGE_OUT) -covermode=atomic ./...

coverage-html: coverage
	go tool cover -html=$(COVERAGE_OUT) -o coverage.html
	@echo "Open coverage.html in a browser to view the report."

coverage-func: coverage
	go tool cover -func=$(COVERAGE_OUT)

lint:
	go vet ./...

clean:
	rm -f $(BINARY) $(COVERAGE_OUT) coverage.html

install:
	go install $(GOFLAGS) .
