package atomics

import "sync/atomic"

// Atomics-first / bool-pack fixtures for competitive corpus.
// Order may differ from betteralign/fieldalignment; size savings still apply.

type HotCounters struct {
	Ready bool
	Hits  atomic.Int64
	Miss  atomic.Uint64
	Name  string
	Ok    bool
}

type ConnLike struct {
	Closed bool
	Ops    uint64
	Errs   int64
	State  int32
	Pad    bool
}

type ClientHot struct {
	ID      uint64
	Pending int32
	Active  bool
	Subs    int64
	Drain   bool
}

type BoolScatter struct {
	A bool
	X int64
	B bool
	Y int32
	C bool
	Z int16
	D bool
}

type AtomicThenDense struct {
	Flag  bool
	Count atomic.Uint64
	Small int8
	Big   int64
	Tiny  bool
}

type MixedAtomics struct {
	A   bool
	Seq atomic.Uint64
	B   bool
	Gen atomic.Int64
	C   int32
}
