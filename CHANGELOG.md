# Changelog

## [1.4.0] - 2026-08-02

### Added
- Cacheguard (`--cacheguard` / `--cache-line`) for false-share detection and opt-in pad insertion
- Project config (`.goalign.yml` / `goalign.yml`) with example file
- `--packages` for imported type sizes via go/packages
- Suggest policies: `--policy=atomics|density|stable`
- SARIF output for GitHub Code Scanning; JSON schema under `schemas/`
- `goalign fix --diff` / `--dry-run`; opt-in `--rewrite-bools`
- Multi-arch summary (`--arches`); skip-generated / ignore globs
- go/analysis vet plugin (`goalign-analyzer`)
- Per-field `// goalign:ignore`; field Index-based fixer mapping

### Fixed
- Correct `atomic.Value` / `Uintptr` / `Pointer` / `unsafe.Pointer` sizing
- Full-struct metrics with field-ignore; clear BoolPack; refuse rewrite-bools on ignore
- Atomic file writes; overflow-safe layout math; bounded `-j` and grpc-decode input
- `--packages` walks imports and remaps import aliases
- Density reorder preferred over bool-pack when both apply
- Vet pass honors EOL ignore and same-file locals
- `fix` rejects config/CLI `--arches`; machine formats under `--arches`; min-waste keeps false-share/cacheguard
- Cacheguard/CacheLine wired for multi-arch analyze

### Changed
- Default suggest policy name is `atomics` (NATS-style atomics-first)

## [1.3.0] - prior

See GitHub Releases for earlier notes.
