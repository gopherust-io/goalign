package layout

import (
	"fmt"
	"strconv"

	"github.com/gopherust-io/goalign/internal/alignmath"
)

// NormalizeCacheLine returns a positive cache line size (default 64).
func NormalizeCacheLine(n int) int {
	if n <= 0 {
		return DefaultCacheLine
	}
	return n
}

// StripCachePads removes synthetic Cacheguard pads from a field list.
func StripCachePads(fields []Field) []Field {
	out := fields[:0:0]
	for _, f := range fields {
		if f.IsCachePad() || IsCachePadName(f.Name) {
			continue
		}
		out = append(out, f)
	}
	if len(out) == 0 && len(fields) > 0 {
		// preserve non-pad-only edge case
		return nil
	}
	return out
}

// HasFalseShare reports whether any two contended fields share a cache line.
func HasFalseShare(fields []Field, cacheLine int) bool {
	return len(FalseShareCollisions(fields, cacheLine)) > 0
}

// FalseShareCollision is a pair of contended fields on the same cache line.
type FalseShareCollision struct {
	A, B string
	Line int
}

// FalseShareCollisions returns contended field pairs that share a cache line.
func FalseShareCollisions(fields []Field, cacheLine int) []FalseShareCollision {
	cacheLine = NormalizeCacheLine(cacheLine)
	type occ struct {
		name string
		lo   int
		hi   int // inclusive line
	}
	var contended []occ
	for _, f := range fields {
		if f.IsCachePad() || !f.IsContend() || f.Size <= 0 {
			continue
		}
		lo := f.Offset / cacheLine
		hi := (f.Offset + f.Size - 1) / cacheLine
		contended = append(contended, occ{name: f.Name, lo: lo, hi: hi})
	}
	var out []FalseShareCollision
	for i := 0; i < len(contended); i++ {
		for j := i + 1; j < len(contended); j++ {
			a, b := contended[i], contended[j]
			if a.hi < b.lo || b.hi < a.lo {
				continue
			}
			line := a.lo
			if b.lo > line {
				line = b.lo
			}
			// Canonical order so A|B and B|A dedupe to one note.
			nameA, nameB := a.name, b.name
			if nameB < nameA {
				nameA, nameB = nameB, nameA
			}
			out = append(out, FalseShareCollision{A: nameA, B: nameB, Line: line})
		}
	}
	return out
}

// FalseShareNotes builds advisory notes for false-sharing collisions.
func FalseShareNotes(fields []Field, cacheLine int) []string {
	cols := FalseShareCollisions(fields, cacheLine)
	if len(cols) == 0 {
		return nil
	}
	cacheLine = NormalizeCacheLine(cacheLine)
	notes := make([]string, 0, len(cols))
	seen := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		key := c.A + "\x00" + c.B + "\x00" + strconv.Itoa(c.Line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		notes = append(notes, fmt.Sprintf(
			"false-share: %s and %s share cache line %d (%d-byte lines) — isolate with goalign fix --cacheguard",
			c.A, c.B, c.Line, cacheLine,
		))
	}
	return notes
}

// ApplyCacheguard inserts _cgpadN fields so each contended field occupies
// exclusive cache lines. Existing pads are stripped first (idempotent).
// Non-contended fields keep relative order; cold fields tend toward the end.
func ApplyCacheguard(fields []Field, cacheLine int) ([]Field, int) {
	cacheLine = NormalizeCacheLine(cacheLine)
	fields = StripCachePads(fields)
	if len(fields) == 0 {
		return nil, 0
	}
	fields = cacheguardOrder(fields)

	out := make([]Field, 0, len(fields)+4)
	total := 0
	lastContendEnd := -1
	padIdx := 0
	padsInserted := 0

	for _, f := range fields {
		if f.IsContend() {
			minStart := 0
			if lastContendEnd >= 0 {
				minStart = ((lastContendEnd + cacheLine - 1) / cacheLine) * cacheLine
			}
			padSize := padBytesToMinStart(total, f.Align, minStart)
			if padSize > 0 {
				out = append(out, makeCachePad(padIdx, padSize))
				padIdx++
				padsInserted++
				total += padSize
			}
		}

		var offset int
		var ok bool
		total, _, offset, ok = alignmath.AddField(total, 0, f.Size, f.Align)
		if !ok {
			return fields, 0
		}
		f.Offset = offset
		out = append(out, f)
		if f.IsContend() {
			lastContendEnd = offset + f.Size
		}
	}
	return out, padsInserted
}

// padBytesToMinStart returns how many align-1 pad bytes to insert at total so
// that Align(total+pad, fieldAlign) >= minStart.
func padBytesToMinStart(total, fieldAlign, minStart int) int {
	if fieldAlign < 1 {
		fieldAlign = 1
	}
	if minStart < 0 {
		minStart = 0
	}
	// Natural start without pad.
	natural := total
	if rem := natural % fieldAlign; rem != 0 {
		natural += fieldAlign - rem
	}
	if natural >= minStart {
		return 0
	}
	// Choose the smallest field-aligned start >= minStart, then pad up to it.
	want := minStart
	if rem := want % fieldAlign; rem != 0 {
		want += fieldAlign - rem
	}
	if want < total {
		return 0
	}
	return want - total
}

func makeCachePad(idx, size int) Field {
	return Field{
		Name:   fmt.Sprintf("_cgpad%d", idx),
		Type:   fmt.Sprintf("[%d]byte", size),
		Size:   size,
		Align:  1,
		Offset: 0,
		Index:  -1 - idx, // synthetic; negative index
		Flags:  FlagCachePad,
	}
}

// cacheguardOrder keeps relative order but moves cold fields after non-cold,
// and hot non-contended slightly earlier among non-contended peers.
func cacheguardOrder(fields []Field) []Field {
	if len(fields) < 2 {
		return fields
	}
	hot := make([]Field, 0, len(fields))
	mid := make([]Field, 0, len(fields))
	cold := make([]Field, 0, len(fields))
	for _, f := range fields {
		switch {
		case f.IsCold() && !f.IsContend():
			cold = append(cold, f)
		case f.IsHot() && !f.IsContend():
			hot = append(hot, f)
		default:
			mid = append(mid, f)
		}
	}
	out := make([]Field, 0, len(fields))
	out = append(out, mid...)
	out = append(out, hot...)
	out = append(out, cold...)
	return out
}

// Relayout recomputes offsets for an ordered field list.
func Relayout(fields []Field) ([]Field, int, int, bool) {
	total, wasted := 0, 0
	maxAlign := 1
	lastSize := 0
	out := make([]Field, len(fields))
	for i, f := range fields {
		if f.Align > maxAlign {
			maxAlign = f.Align
		}
		var offset int
		var ok bool
		total, wasted, offset, ok = alignmath.AddField(total, wasted, f.Size, f.Align)
		if !ok {
			return nil, 0, 0, false
		}
		f.Offset = offset
		out[i] = f
		lastSize = f.Size
	}
	var ok bool
	total, wasted, _, ok = alignmath.Finish(total, wasted, maxAlign, lastSize, len(fields))
	return out, total, wasted, ok
}
