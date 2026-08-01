# GoAlign — Architecture

CLI that detects Go struct padding waste and optionally rewrites field order into a denser, CPU-friendly layout.

## Overview

`github.com/gopherust-io/goalign` scans Go sources via AST (no `go/packages` by default), computes current vs suggested layouts per `GOARCH`, and can autofix field order. Suitable as a local tool or CI gate (`--fail-on-findings`). Part of the gopherust-io developer tooling set alongside **env**, **tel**, and **nats**.

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
| `cmd` | Cobra commands: `analyze`, `fix`, `completion`, `grpc-decode`; scan helpers |
| `internal/analyzer` | AST scan, `// goalign:ignore`, severity / findings |
| `internal/layout` | `Sizer`, `Compute`, `Suggest`, field sizing |
| `internal/alignmath` | Alignment padding helpers |
| `internal/goarch` | Pointer size / align tables per architecture |
| `internal/fixer` | Source rewrite to suggested field order |
| `internal/formatter` | text / JSON / table output |
| `internal/bytesconv` | Unsafe string/bytes helpers |
| `examples/` | Sample structs for demos |

## Key design rules

- **CLI → analyzer → layout → format/fix:** commands never reimplement sizing; they call `analyzer` and `layout`.
- **Arch-parameterized:** sizing comes from `goarch` / `SizerFor`; default matches the host unless `--arch` is set.
- **Allocation-sensitive math:** `Compute` / `Suggest` reuse buffers; keep hot paths free of per-field heap churn.
- **Ignore directives:** `// goalign:ignore` skips structs the analyzer must not report or rewrite.
- **Suggest policy:** atomics / wide counters first where applicable, then density packing (NATS-inspired notes such as `atomics-first` / `bool-pack`).

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
4. `formatter.Format` prints text/JSON/table; with `--fail-on-findings`, non-zero exit if waste ≥ `--min-waste`.
5. `goalign fix` follows the same analysis path, then `fixer.FixPath` rewrites source field order.

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
