# Contributing

## Prerequisites

- Go version from [`go.mod`](go.mod)
- `make` helpers (optional)

## Development

```bash
make test          # unit tests
make test-race     # race detector
make coverage      # coverage + COVERAGE_MIN gate
make bench         # critical benchmarks
make fuzz          # fuzz smoke (15s)
make lint          # govulncheck + golangci-lint
make ci            # fmt-check + test + race + vet + lint
make run-examples  # build and analyze examples/
```

## Pull requests

1. Keep changes focused; prefer table-driven tests with `t.Parallel` where safe.
2. Run `make ci` before opening a PR.
3. Hot-path layout math must stay zero-alloc (`TestComputeNoAlloc` / `BenchmarkCompute`).
4. Update `README.md` when changing CLI flags or analysis rules.
5. Do not commit secrets.

Useful CLI flags for CI: `--fail-on-findings`, `--min-waste=N`, `--arch=amd64`.

## CI

On PRs and `main`/`master`, GitHub Actions runs:

| Job | Checks |
|-----|--------|
| Test and lint | tidy, gofmt, vet, coverage on `./internal/...` (≥70%), build, golangci-lint, self-check analyze |
| Race | `go test -race` |
| Critical benchmarks | `BenchmarkCompute`, `BenchmarkAnalyzeFile` |
| Fuzz smoke | `FuzzComputeSource`, `FuzzTypeInfo` (2s each) |
| Vulnerability scan | `govulncheck` |
