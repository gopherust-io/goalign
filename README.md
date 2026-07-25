# GoAlign

A fast CLI for analyzing Go struct alignment and suggesting CPU-friendly field order
(NATS / `govet/fieldalignment` style).

## Features

- **Struct analysis** — detects inter-field and trailing padding waste
- **Suggested reorder** — atomics / 64-bit counters first, then density packing
- **NATS-inspired rules** — `atomics-first` and `bool-pack` notes
- **Zero-alloc layout math** — `layout.Compute` is allocation-free with a reused buffer
- **Output formats** — text, JSON, table (streamed to stdout)
- **Ignore comments** — `// goalign:ignore`
- **Recursive scan** — parallel file analysis

## Installation

```bash
go install github.com/gopherust-io/goalign@latest
```

## Usage

```bash
# Analyze current directory
goalign analyze

# Analyze a file
goalign analyze main.go

# Recursive with excludes (vendor/.git/node_modules/bin are skipped by default)
goalign analyze -r -e testdata/ ./src

# Target a specific GOARCH
goalign analyze --arch=386 ./pkg

# CI gate: fail if any issue wastes >= 8 bytes
goalign analyze -r --fail-on-findings --min-waste=8 .

# Formats
goalign analyze -f text
goalign analyze -f json
goalign analyze -f table
```

## Example

Given:

```go
type BadStruct struct {
    A bool    // 1 byte
    B int64   // 8 bytes (7 bytes padding before)
    C int32   // 4 bytes
    D bool    // 1 byte (+ trailing pad)
}
```

GoAlign reports waste and a suggested order:

```
📁 example.go
================

🟡 BadStruct (line 1)
   Struct 'BadStruct' has 10 bytes of padding (41% waste); reorder saves 8 bytes
   Fields:
     A bool (size: 1, offset: 0, align: 1)
     B int64 (size: 8, offset: 8, align: 8)
     C int32 (size: 4, offset: 16, align: 4)
     D bool (size: 1, offset: 20, align: 1)
   Suggested order (saves 8 bytes):
     B int64 (size: 8, offset: 0, align: 8)
     C int32 (size: 4, offset: 8, align: 4)
     A bool (size: 1, offset: 12, align: 1)
     D bool (size: 1, offset: 13, align: 1)

📊 Summary: 1 issues found, 10 bytes wasted, 8 bytes savable by reorder
```

## NATS-inspired rules

Practices taken from nats-server / nats.go hot structs:

1. **Density packing** — larger align/size fields first (same idea as `govet/fieldalignment`)
2. **Atomics first** — `int64` / `uint64` / `atomic.*` counters at the start of the struct
   (64-bit alignment on 32-bit arches; matches nats.go `Conn` / server `client` layout)
3. **Bool pack** — when 3+ bools are scattered among larger fields, notes a flag-word opportunity

## Ignoring structs

```go
// goalign:ignore
type LegacyStruct struct {
    A bool
    B int64
}
```

## Performance

Layout computation is designed for zero heap allocations when given a reused field buffer
(validated with `testing.AllocsPerRun` / `b.ReportAllocs()`, same style as the nats wrapper):

```bash
go test ./internal/layout -bench=BenchmarkCompute -benchmem
# BenchmarkCompute-…   0 B/op   0 allocs/op
```

## Development

```bash
make test       # unit tests
make test-race  # race detector
make coverage   # coverage gate (COVERAGE_MIN=70)
make bench      # critical benchmarks
make fuzz       # fuzz smoke
make ci         # fmt-check + test + race + vet + lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full PR checklist and CI job inventory.

## Architecture notes

- Fast **AST heuristics** (no `go/packages` by default) for scan speed
- `--arch` selects GOARCH size tables (`amd64`/`arm64`/`386`/`arm`)
- Handles fixed arrays (including `1<<n` lengths), anonymous nested structs, embeds, trailing padding, and trailing zero-sized fields (gc ABI)
- Unresolvable array lengths (named consts) are skipped to avoid false positives
- Named imported types still use pointer-sized heuristics (no type checker)

## License

This project is licensed under the MIT License — see [LICENSE](LICENSE) for details.
