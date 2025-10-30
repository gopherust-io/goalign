.PHONY: build test install clean run-examples lint

# Build the application
build:
	go build -o bin/goalign .

# Install the application
install:
	go install .

# Run tests
test:
	go test -v ./...

# Run linting
lint:
	golangci-lint run

# Clean build artifacts
clean:
	rm -rf bin/

# Run examples
run-examples:
	goalign analyze -r examples/

# Run on source code
run-source:
	goalign analyze -r . -e examples/

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/goalign-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/goalign-darwin-amd64 .
	GOOS=windows GOARCH=amd64 go build -o bin/goalign-windows-amd64.exe .

# Run with verbose output
run-verbose:
	goalign analyze -v -r .

# Run with JSON output
run-json:
	goalign analyze -f json -r examples/

# Run with table output
run-table:
	goalign analyze -f table -r examples/
