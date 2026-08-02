# GoAlign — Architecture

CLI that detects Go struct padding waste and optionally rewrites field order into a denser, CPU-friendly layout.

## Overview

`github.com/gopherust-io/goalign` scans Go sources via AST (no `go/packages` by default; opt-in `--packages`), computes current vs suggested layouts per `GOARCH`, and can autofix field order (`fix` / `--diff`). Suitable as a local tool or CI gate (`--fail-on-findings`, SARIF). Part of the gopherust-io developer tooling set alongside **env** and **tel**.

Ecosystem: [gopherust-io](https://github.com/gopherust-io/gopherust-io/blob/main/ARCHITECTURE.md)

## Layer / package overview

```
┌─────────────────────────────────────────────────────────────┐
│  CLI (Cobra)                                                │
│  main.go → cmd.Execute (analyze | fix | …)                  │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  Analyzer                                                   │
│  internal/analyzer — AST walk, ignores, issues              │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  Layout math                                                │
│  internal/layout + alignmath + goarch                       │
│  Compute / Suggest (zero-alloc buffers)                     │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        ▼                                     ▼
┌───────────────────┐               ┌───────────────────┐
│  formatter        │               │  fixer            │
│  text/JSON/table  │               │  rewrite sources  │
└───────────────────┘               └───────────────────┘
```

## Packages

| Path | Responsibility |
|------|----------------|
| `cmd` | Cobra commands: `analyze`, `fix`, `completion`; scan helpers + config |
| `cmd/goalign-analyzer` | vet-compatible `go/analysis` multichecker |
| `analysis` | Exported `Analyzer` for plugins / golangci |
| `internal/analyzer` | AST scan, ignores, severity / findings, policies |
| `internal/layout` | `Sizer`, `Compute`, `Suggest` (+ policies, ptrdata, bool-pack) |
| `internal/layout/cacheguard.go` | False-share detection, `ApplyCacheguard` pad insertion |
| `internal/alignmath` | Alignment padding helpers |
| `internal/goarch` | Pointer size / align tables per architecture |
| `internal/fixer` | Source rewrite, `--diff`, bool-pack + Cacheguard pad inserts |
| `internal/formatter` | text / JSON / table / SARIF (+ `CLINE` for false-share) |
| `internal/config` | `.goalign.yml` loader |
| `internal/diff` | Unified diff for dry-run fix |
| `internal/pkgscan` | Opt-in `go/packages` type sizes |
| `internal/bytesconv` | Unsafe string/bytes helpers |
| `schemas/` | JSON schema for analyze output |
| `examples/` | Sample structs for demos |

## Key design rules

- **CLI → analyzer → layout → format/fix:** commands never reimplement sizing; they call `analyzer` and `layout`.
- **Arch-parameterized:** sizing comes from `goarch` / `SizerFor`; default matches the host unless `--arch` is set.
- **Allocation-sensitive math:** `Compute` / `Suggest` reuse buffers; keep hot paths free of per-field heap churn.
- **Ignore directives:** `// goalign:ignore` skips structs the analyzer must not report or rewrite.
- **Suggest policy:** default `atomics` (atomics first + density); `density` / `stable` via `--policy`. Notes include `atomics-first`, `bool-pack`, `ptrdata`, `false-share`.
- **Cacheguard:** detect contended fields sharing a cache line; opt-in `--cacheguard` inserts `_cgpadN` pads (`internal/layout/cacheguard.go`).
- **Config:** `.goalign.yml` supplies defaults; explicit CLI flags win.

## Core APIs / interfaces

```go
// internal/analyzer
func AnalyzeFile(path string) (*Result, error)
func AnalyzeSource(filename string, src []byte) (*Result, error)

// internal/layout
type Sizer interface { /* size/align of types */ }
func (s *Sizer) Compute(fields []Field) Result
func Suggest(info Info) SuggestResult

// internal/fixer
func FixPath(path string, opts …) ([]FileResult, error)

// cmd
func Execute() error
```

## Request / call flow

Example: `goalign analyze ./pkg`

1. Cobra `analyze` resolves paths and `GOARCH` / format flags.
2. `analyzer.AnalyzeFile` parses AST and collects struct types (honoring ignores).
3. For each struct, `layout.Compute` measures padding; `Suggest` proposes a reorder when waste exceeds policy.
4. Cacheguard: emit `false-share` notes when contended fields share a cache line; with `--cacheguard`, `ApplyCacheguard` adds `_cgpadN` fields to Suggested.
5. `formatter.Format` prints text/JSON/table/SARIF (`CLINE` when false-share); with `--fail-on-findings`, non-zero exit if waste ≥ `--min-waste`.
6. `goalign fix` follows the same analysis path, then `fixer` rewrites (reorder and/or Cacheguard pads; `--diff` for preview).

## Bootstrap / lifecycle

- Process entry: `main.go` → `cmd.Execute()`.
- No long-lived daemon; each invocation is a single scan/fix pass.
- Install: `go install github.com/gopherust-io/goalign@latest`.

## Adding a feature

1. Extend `internal/layout` / `alignmath` / `goarch` if the change is about sizes or suggest rules.
2. Extend `internal/analyzer` for new diagnostics, ignore semantics, or AST coverage.
3. Wire flags and UX in `cmd`; keep formatting in `internal/formatter`.
4. If rewriting source, update `internal/fixer` and add fixtures under tests/examples.
5. Document CI flag behavior in the README when exit-code policy changes.

## Related docs

- [README](README.md)
- [Contributing](CONTRIBUTING.md)
