package analyzer

import "github.com/gopherust-io/goalign/internal/layout"

// Options configures analysis behavior.
type Options struct {
	TypeSizes  map[string]layout.Info // --packages resolved sizes (nil = AST only)
	Policy     layout.Policy
	Sizer      layout.Sizer
	CacheLine  int  // CPU cache line size for Cacheguard (default 64)
	Cacheguard bool // rewrite Suggested to insert cache-line pads
}

// DefaultOptions returns Options with host sizer and atomics-first policy.
func DefaultOptions() Options {
	return Options{
		Sizer:     layout.DefaultSizer(),
		Policy:    layout.PolicyAtomics,
		CacheLine: layout.DefaultCacheLine,
	}
}
