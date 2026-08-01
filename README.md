# GoAlign

A fast CLI for analyzing Go struct alignment and rewriting fields into a
CPU-friendly order (NATS / `govet/fieldalignment` style).

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/gopherust-io/goalign/badge)](https://scorecard.dev/viewer/?uri=github.com/gopherust-io/goalign)

Module: [`github.com/gopherust-io/goalign`](https://github.com/gopherust-io/goalign)

Latest stable release: see [GitHub Releases](https://github.com/gopherust-io/goalign/releases).

Quick links: [Architecture](ARCHITECTURE.md) · [Usage](#usage) · [Known limitations](#known-limitations) · [Contributing](CONTRIBUTING.md)

---

## Features

- **Struct analysis** — detects inter-field and trailing padding waste
- **Autofix** — `goalign fix` rewrites structs to the suggested field order
- **Suggested reorder** — atomics / 64-bit counters first, then density packing
- **NATS-inspired rules** — `atomics-first` and `bool-pack` notes
- **Zero-alloc layout math** — `Compute` / `Suggest` stay allocation-free with a reused buffer
- **Output formats** — text (colorized on TTY), JSON, table
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

# Apply suggested field order (rewrites source; review the diff)
goalign fix -r .
goalign fix --arch=amd64 ./pkg

# Formats
goalign analyze -f text
goalign analyze -f json
goalign analyze -f table
```

CI-friendly gate example:

```bash
goalign analyze -r --fail-on-findings --min-waste=8 ./...
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
example.go
----------
HIGH BadStruct  line 1
  Struct 'BadStruct' has 10 bytes of padding (41% waste); reorder saves 8 bytes
  Current
    NAME  TYPE   SIZE  OFFSET  ALIGN
    A     bool   1     0       1
    B     int64  8     8       8
    C     int32  4     16      4
    D     bool   1     20      1
  Suggested  (saves 8 bytes)
    NAME  TYPE   SIZE  OFFSET  ALIGN
    B     int64  8     0       8
    C     int32  4     8       4
    A     bool   1     12      1
    D     bool   1     13      1

Summary: 1 issues, 10 bytes wasted, 8 bytes savable
```

Apply the rewrite:

```bash
goalign fix example.go
# Fixed 1 structs in 1 files, saved 8 bytes
```

`fix` rewrites source in place. Review the diff before committing. Structs marked
`// goalign:ignore` are left unchanged.

## NATS-inspired rules

Practices taken from nats-server / nats.go hot structs:

1. **Density packing** — larger align/size fields first (same idea as `govet/fieldalignment`)
2. **Atomics first** — `int64` / `uint64` / `atomic.*` counters at the start of the struct
   (64-bit alignment on 32-bit arches; matches nats.go `Conn` / server `client` layout)
3. **Bool pack** — when 3+ bools are scattered among larger fields, notes a flag-word opportunity

## Known limitations

- Uses fast AST heuristics (not full type-checking) by default for speed.
- Imported named types are estimated using pointer-size assumptions.
- Arrays with unresolvable named constants are skipped to avoid false positives.
- Autofix reorders fields and may split multi-name declarations (`A, B int`); it does not rewrite types into flag words.

## Ignoring structs

```go
// goalign:ignore
type LegacyStruct struct {
    A bool
    B int64
}
```

## Performance

Layout math (`Compute`, `Suggest` with a reused `2*n` buffer and no notes) is designed
for zero heap allocations (validated with `testing.AllocsPerRun` / `b.ReportAllocs()`):

```bash
go test ./internal/layout -bench='BenchmarkCompute|BenchmarkSuggest' -benchmem
# BenchmarkCompute-…   0 B/op   0 allocs/op
# BenchmarkSuggest-…   0 B/op   0 allocs/op

make escape   # filtered -gcflags=-m for hot-path packages
```

Reporting paths (`FillTypeNames`, messages, AST parse) allocate by design.

## Building and testing

```bash
make test       # unit tests
make test-race  # race detector
make coverage   # coverage gate (COVERAGE_MIN=70)
make bench      # all internal package benchmarks
make escape     # filtered compiler escape analysis
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

[Contributing](CONTRIBUTING.md)

## License

Apache License 2.0 — see [LICENSE](LICENSE).
