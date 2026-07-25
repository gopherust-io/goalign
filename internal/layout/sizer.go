package layout

import (
	"runtime"

	"github.com/gopherust-io/goalign/internal/goarch"
)

// Sizer provides architecture-aware sizes and alignments for Go types.
type Sizer struct {
	PtrSize int // size of pointers, int, uint, uintptr
}

// DefaultSizer returns a Sizer for the current GOARCH (amd64/arm64 → 8, else 4).
func DefaultSizer() Sizer {
	return SizerFor(runtime.GOARCH)
}

// SizerFor returns a Sizer for the given GOARCH string.
func SizerFor(name string) Sizer {
	return Sizer{PtrSize: goarch.PtrSize(name)}
}

// Info holds size and alignment of a type in bytes.
type Info struct {
	Size  int
	Align int
}

func (s Sizer) ptr() Info {
	return Info{Size: s.PtrSize, Align: s.PtrSize}
}
