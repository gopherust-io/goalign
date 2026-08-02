# Competitive analyze (wall-clock)

- Corpus: `testdata/benchcorpus/density`
- Repeats: 5 (mean / median ms)
- go version go1.26.5 darwin/arm64
- Darwin/arm64 — Apple M4 Pro

| Tool | mean ms | median ms |
|------|--------:|----------:|
| goalign analyze -r -f json | 16.4 | 16.3 |
| betteralign | 55.7 | 53.4 |
| fieldalignment | 62.4 | 61.5 |

Preface: goalign is AST-only (no typechecker). betteralign/fieldalignment load packages via `go/packages` and optimize for true type sizes + GC ptrdata. Numbers are directional.
