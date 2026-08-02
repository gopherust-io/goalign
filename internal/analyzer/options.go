package analyzer

import "github.com/gopherust-io/goalign/internal/layout"

// Options configures analysis behavior.
type Options struct {
	Sizer  layout.Sizer
	Policy layout.Policy
	// TypeSizes maps qualified or local type names to resolved sizes
	// (populated by --packages mode). Nil means AST heuristics only.
	TypeSizes map[string]layout.Info
	// CacheLine is the CPU cache line size for Cacheguard (default 64).
	CacheLine int
	// Cacheguard, when true, rewrites Suggested to insert cache-line pads.
	Cacheguard bool
}

// DefaultOptions returns Options with host sizer and atomics-first policy.
func DefaultOptions() Options {
	return Options{
		Sizer:     layout.DefaultSizer(),
		Policy:    layout.PolicyAtomics,
		CacheLine: layout.DefaultCacheLine,
	}
}
