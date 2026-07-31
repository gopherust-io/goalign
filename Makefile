export GOWORK := off

PKGS := $(shell go list ./... | grep -vE '/examples')
COVER_PKGS := $(shell go list ./internal/...)
FUZZ_PKGS := ./internal/layout
FUZZ_TESTS := FuzzComputeSource FuzzTypeInfo
NPROCS := $(shell getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)
GO_TEST_FLAGS := -count=1 -parallel=$(NPROCS) -timeout=60s
COVERAGE_MIN ?= 70

.PHONY: help build test test-race coverage coverage-html bench fuzz escape ci vet fmt fmt-check lint lint-fix govulncheck \
	install clean run-examples run-source run-verbose run-json run-table build-all

GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@v1.6.0

help:
	@echo "Targets:"
	@echo "  build           Build bin/goalign"
	@echo "  test            Run unit tests (max parallel)"
	@echo "  test-race       Run tests with -race"
	@echo "  coverage        Cover ./internal/... and enforce COVERAGE_MIN ($(COVERAGE_MIN)%)"
	@echo "  coverage-html   Open HTML coverage report"
	@echo "  bench           Run critical benchmarks"
	@echo "  escape          Compiler escape analysis for hot-path packages"
	@echo "  fuzz            Fuzz smoke (15s per target)"
	@echo "  ci              fmt-check + test + test-race + vet + lint"
	@echo "  fmt             gofmt -w all Go files"
	@echo "  fmt-check       Fail if any file needs gofmt"
	@echo "  lint            govulncheck + golangci-lint (includes fieldalignment)"
	@echo "  lint-fix        golangci-lint --fix"
	@echo "  govulncheck     Scan dependencies for known vulnerabilities"
	@echo "  vet             go vet on all packages"
	@echo "  run-examples    Analyze examples/"
	@echo "  run-source      Analyze source (exclude examples/)"

build:
	go build -o bin/goalign .

install:
	go install .

test:
	go test $(GO_TEST_FLAGS) $(PKGS)

test-race:
	go test -race $(GO_TEST_FLAGS) $(PKGS)

coverage:
	go test $(GO_TEST_FLAGS) $(COVER_PKGS) -coverprofile=coverage.out
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/,"",$$NF); print $$NF}'); \
	echo "Total coverage: $${total}% (minimum $(COVERAGE_MIN)%)"; \
	awk -v t="$${total}" -v m="$(COVERAGE_MIN)" 'BEGIN { if (t+0 < m+0) { print "coverage below threshold"; exit 1 } }'
	go tool cover -func=coverage.out

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

bench:
	go test -bench='.' -benchmem -run '^$$' ./internal/layout/ ./internal/analyzer/ ./internal/alignmath/ ./internal/fixer/ ./internal/formatter/ ./internal/bytesconv/

# Filtered compiler escape analysis for layout hot path.
# Ideal: no lines for Compute/Suggest when callers reuse dst with enough capacity.
# Cold-path grows (appendField, Suggest make) and reporting still show up — expected.
escape:
	@echo "== alignmath + layout (heap escapes) =="
	@go build -gcflags='-m=2' ./internal/alignmath/ ./internal/layout/ 2>&1 | \
		grep -E 'escapes to heap|moved to heap' | \
		grep -vE 'typeString|FillTypeNames|CollectLocals|ruleNotes' || true
	@echo "== done (Compute/Suggest reused-buffer path should stay quiet) =="

fuzz:
	@set -e; for fuzz in $(FUZZ_TESTS); do \
		go test -fuzz=$$fuzz -fuzztime=15s $(FUZZ_PKGS) -run '^$$'; \
	done

ci: fmt-check test test-race vet lint

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

lint: govulncheck
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run ./... --fix

govulncheck:
	$(GOVULNCHECK) ./...

clean:
	rm -rf bin/ coverage.out coverage.html

run-examples: build
	./bin/goalign analyze -r examples/

run-source: build
	./bin/goalign analyze -r . -e examples/,bin/

run-verbose: build
	./bin/goalign analyze -v -r .

run-json: build
	./bin/goalign analyze -f json -r examples/

run-table: build
	./bin/goalign analyze -f table -r examples/

build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/goalign-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/goalign-darwin-amd64 .
	GOOS=windows GOARCH=amd64 go build -o bin/goalign-windows-amd64.exe .
