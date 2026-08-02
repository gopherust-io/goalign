// Package main demos Cacheguard (false-share) findings.
//
//	goalign analyze examples/cacheguard.go
//	goalign analyze --cacheguard examples/cacheguard.go   # Suggested with pads
//	goalign fix --cacheguard --diff examples/cacheguard.go
//
// Plain int64 pairs are NOT auto-contended; use // goalign:contend or atomic.*/sync.
package main

import (
	"sync"
	"sync/atomic"
)

// HotAtomics: two atomics on the same 64-byte cache line (0 waste, still a finding).
type HotAtomics struct {
	A atomic.Int64
	B atomic.Int64
}

// MutexState: mutex + counter — atomics-first may reorder; annotate n for Cacheguard.
type MutexState struct {
	mu sync.Mutex
	n  int64 // goalign:contend
}

// AnnotatedInts: plain ints only collide when marked contend.
type AnnotatedInts struct {
	x int32 // goalign:contend
	y int32 // goalign:contend
}
