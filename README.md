# GoAlign

A fast CLI for analyzing Go struct alignment and rewriting fields into a
CPU-friendly order (`govet/fieldalignment` style, with atomics-first packing).

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/gopherust-io/goalign/badge)](https://scorecard.dev/viewer/?uri=github.com/gopherust-io/goalign)

Module: [`github.com/gopherust-io/goalign`](https://github.com/gopherust-io/goalign)

Latest stable release: see [GitHub Releases](https://github.com/gopherust-io/goalign/releases).

Quick links: [Architecture](ARCHITECTURE.md) · [Usage](#usage) · [Cacheguard](#cacheguard) · [Known limitations](#known-limitations) · [Contributing](CONTRIBUTING.md)

---

## Features

- **Struct analysis** — detects inter-field and trailing padding waste
- **Autofix** — `goalign fix` rewrites structs to the suggested field order
- **Diff / dry-run** — `goalign fix --diff` prints a unified diff without writing
- **Suggested reorder** — atomics / 64-bit counters first, then density packing
- **Suggest policies** — `--policy=atomics|density|stable`
- **Cacheguard** — detect false sharing; opt-in `--cacheguard` pads isolate contended fields
- **Layout notes** — `atomics-first`, `bool-pack`, `ptrdata`, `false-share`
- **Opt-in bool rewrite** — `--rewrite-bools` packs unexported scattered bools into a flags word
- **Opt-in type accuracy** — `--packages` resolves imported sizes via `go/packages`
- **Config file** — `.goalign.yml` / `goalign.yml` project defaults
- **Output formats** — text, JSON, table, SARIF (GitHub Code Scanning)
- **go/analysis plugin** — `goalign-analyzer` vet tool
- **Ignore** — struct/field `// goalign:ignore`, path globs, generated-file skip
- **Multi-arch** — `--arches=amd64,arm64,386` summary matrix
- **CI gate** — `--fail-on-findings` / `--min-waste`
- **Zero-alloc layout math** — `Compute` / `Suggest` stay allocation-free with a reused buffer
- **Recursive scan** — parallel file analysis

## Installation

```bash
go install github.com/gopherust-io/goalign@latest

# Optional vet-compatible analyzer
go install github.com/gopherust-io/goalign/cmd/goalign-analyzer@latest
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

# Multi-arch summary
goalign analyze --arches=amd64,arm64,386 -r .

# CI gate: fail if any issue wastes >= 8 bytes
goalign analyze -r --fail-on-findings --min-waste=8 .

# Accurate imported-type sizes (slower)
goalign analyze --packages -r .

# Apply suggested field order (rewrites source; review the diff)
goalign fix -r .
goalign fix --diff -r .          # unified diff only
goalign fix --dry-run --arch=amd64 ./pkg

# Opt-in bool → flags word (unexported bools only; breaking — review carefully)
goalign fix --rewrite-bools --diff ./pkg

# Cacheguard: isolate contended fields across cache lines (may grow the struct)
goalign analyze ./pkg                    # reports false-share notes
goalign fix --cacheguard --diff ./pkg    # preview _cgpadN inserts

# Formats
goalign analyze -f text
goalign analyze -f json
goalign analyze -f table
goalign analyze -f sarif > findings.sarif

# Suggest policy
goalign analyze --policy=density .
```

CI-friendly gate example:

```bash
goalign analyze -r --fail-on-findings --min-waste=8 .
```

Exit codes: `0` success, `1` analysis errors or findings with `--fail-on-findings`, `2` usage errors.

### Config file

Place `.goalign.yml` (or `goalign.yml`) in the project root. Flags override config.

```yaml
# see goalign.example.yml
arch: amd64
min-waste: 8
fail-on-findings: true
policy: atomics
skip-generated: true
exclude:
  - testdata/
ignore:
  - "**/*.pb.go"
```

### go/analysis / vet

```bash
go vet -vettool=$(which goalign-analyzer) ./...
```

Or import `github.com/gopherust-io/goalign/analysis` into a multichecker / golangci-lint plugin.

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

goalign fix --diff example.go   # preview without writing
```

`fix` rewrites source in place unless `--diff` / `--dry-run` is set. Review the
diff before committing. Structs marked `// goalign:ignore` are left unchanged.

## Suggest rules

Default policy (`--policy=atomics`):

1. **Atomics first** — `int64` / `uint64` / `atomic.*` counters at the start of the struct
   (keeps 64-bit alignment on 32-bit arches)
2. **Density packing** — larger align/size fields next (same idea as `govet/fieldalignment`)
3. **Bool pack** — when 3+ bools are scattered among larger fields, notes a flag-word opportunity
   (`--rewrite-bools` can apply an opt-in flags-word rewrite for unexported bools)
4. **ptrdata** — notes when a meaningful fraction of the struct is pointer-bearing (GC scan)
5. **Cacheguard / false-share** — see [Cacheguard](#cacheguard) below.

Use `--policy=density` for pure density sort, or `--policy=stable` to reduce churn
(no atomics-first partition).

## Cacheguard

Size-only aligners pack fields tightly. Under concurrency that can put two hot
counters or a `sync.Mutex` next to an atomic on the **same 64-byte cache line**,
causing false sharing and hurting throughput. Cacheguard detects that failure
mode even when padding waste is already **0**.

**Other aligners shrink; Cacheguard protects** (and may grow `sizeof` on purpose).

### Auto-detect vs annotations

| Contended automatically | Needs annotation |
|-------------------------|------------------|
| `atomic.*` | plain `int32` / `int64` / … → `// goalign:contend` |
| `sync.Mutex`, `sync.RWMutex` | optional `// goalign:hot` / `// goalign:cold` hints |

Plain `int64`/`uint64` still get **atomics-first** packing; they are **not**
auto-contended (avoids false-share spam on non-concurrent structs).

### Commands

```bash
# Notes only (safe default)
goalign analyze examples/cacheguard.go

# Preview Suggested layout with pads (does not write)
goalign analyze --cacheguard examples/cacheguard.go

# Apply pads (review with --diff first)
goalign fix --cacheguard --diff examples/cacheguard.go
goalign fix --cacheguard examples/cacheguard.go
```

Config (see [`goalign.example.yml`](goalign.example.yml)): `cacheguard`, `cache-line` (default 64).

### Before / after

```go
// Before — 0 padding waste, both atomics on cache line 0
type Hot struct {
    A atomic.Int64
    B atomic.Int64
}

// After goalign fix --cacheguard
type Hot struct {
    A       atomic.Int64
    _cgpad0 [56]byte // goalign:cacheguard — separate contended fields
    B       atomic.Int64
}
```

Text output includes a `CLINE` column when false-share notes are present.

### Caveats

- Pads grow the struct; always review `--diff` before committing.
- Cacheguard is not a substitute for API/design choices (prefer uncontended ownership when you can).
- Pad fields are named `_cgpadN` and marked `// goalign:cacheguard` (re-runs are idempotent).
- `sync.Mutex` / `RWMutex` sizes are AST heuristics unless `--packages` is set.

## Known limitations

- Default path uses fast AST heuristics (not full type-checking) for speed.
- Imported named types are **skipped** (unknown) unless `--packages` is set — GoAlign does not guess pointer-sized placeholders for unresolved types.
- Arrays with unresolvable named constants are skipped to avoid false positives.
- Autofix reorders fields and may split multi-name declarations (`A, B int`).
- `--rewrite-bools` changes field types (breaking); callers must be updated manually.
- Per-field `// goalign:ignore` reports waste for remaining fields but skips autofix.
- Cacheguard auto-contend is limited to `atomic.*` and `sync.Mutex`/`RWMutex`; plain integers need `// goalign:contend`.
- `// goalign:hot` / `cold` are soft ordering hints, not a full profile-guided layout.

## Ignoring structs and fields

```go
// goalign:ignore
type LegacyStruct struct {
    A bool
    B int64
}

type Partial struct {
    A bool // goalign:ignore
    B int64
    C bool
}
```

Generated files (`Code generated … DO NOT EDIT`, `*.pb.go`, `*.gen.go`) are skipped by default (`--skip-generated=false` to include).

## Performance

### Hot-path layout math

`Compute` / `Suggest` (reused `2*n` buffer, no notes) are designed for zero heap allocations:

```bash
go test ./internal/layout -bench='BenchmarkCompute|BenchmarkSuggest' -benchmem
# BenchmarkCompute-…   0 B/op   0 allocs/op
# BenchmarkSuggest-…   0 B/op   0 allocs/op

make escape   # filtered -gcflags=-m for hot-path packages
```

Reporting paths (`FillTypeNames`, messages, AST parse) allocate by design.

### Competitive scan

GoAlign optimizes for **AST scan speed** (no typechecker by default). betteralign / fieldalignment use `go/packages` and optimize for true type sizes + GC ptrdata. Use `--packages` when you need import accuracy. Do not treat these as identical work.

| Capability | goalign | betteralign | fieldalignment |
|------------|---------|-------------|----------------|
| Autofix | yes (`fix` / `--diff`) | yes (`-apply`) | yes (`-fix`; often drops comments) |
| Formats | text / json / table / sarif | analysis json / diff | json |
| Arch select | `--arch` / `--arches` | host / typesizes | host / typesizes |
| Atomics-first | yes (`--policy=atomics`) | no | no |
| bool-pack notes / rewrite | notes + opt-in `--rewrite-bools` | no | no |
| Cacheguard / false-share | notes + opt-in `--cacheguard` pads | no | no |
| ptrdata / GC notes | advisory notes | yes | yes |
| Type accuracy (imports) | heuristic; `--packages` for full | full | full |
| Ignore directive | `goalign:ignore` (+ field / globs) | `betteralign:ignore` | — |
| CI gate | `--fail-on-findings` / `--min-waste` | exit on diagnostics | exit on diagnostics |

Library path over the shared density+atomics corpus (`AnalyzeSource`, in-memory):

```bash
make bench-corpus
# ~105 µs/op on Apple M4 Pro (directional)
```

CLI wall-clock on the density tree (warm module cache, Apple M4 Pro sample):

| Tool | mean ms | median ms |
|------|--------:|----------:|
| **goalign** `analyze -r -f json` | **16** | **16** |
| betteralign | 56 | 53 |
| fieldalignment | 62 | 62 |

```bash
make bench-compare   # writes artifacts/benchcmp.md
```

Corpus: [`testdata/benchcorpus/`](testdata/benchcorpus/). Methodology: [`scripts/bench-compare.sh`](scripts/bench-compare.sh).

## Building and testing

```bash
make test       # unit tests
make test-race  # race detector
make coverage   # coverage gate (COVERAGE_MIN=70)
make bench          # all internal package benchmarks
make bench-corpus   # AnalyzeSource over competitive corpus
make bench-compare  # CLI wall-clock vs betteralign/fieldalignment
make escape         # filtered compiler escape analysis
make fuzz           # fuzz smoke
make ci             # fmt-check + test + race + vet + lint
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full PR checklist and CI job inventory.

## Architecture notes

- Fast **AST heuristics** (no `go/packages` by default) for scan speed; `--packages` opt-in
- `--arch` selects GOARCH size tables (amd64, arm64, 386, arm, mips, riscv64, wasm, …)
- Handles fixed arrays (including `1<<n` lengths), anonymous nested structs, embeds, trailing padding, and trailing zero-sized fields (gc ABI)
- Unresolvable array lengths (named consts) and unresolved imported types are skipped to avoid false positives

[Contributing](CONTRIBUTING.md)

## License

Apache License 2.0 — see [LICENSE](LICENSE).
