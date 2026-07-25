package layout

import (
	"sort"

	"github.com/gopherust-io/goalign/internal/alignmath"
)

// SuggestResult holds a density-optimized field order and metrics.
type SuggestResult struct {
	Fields []Field
	Notes  []string
	Total  int
	Wasted int
	Saved  int // originalWasted - Wasted (clamped >= 0)
}

// Suggest reorders fields: atomic/64-bit counters first (NATS), then density
// sort (align desc, size desc). Writes into dst when capacity allows.
// originalWasted is used to compute Saved.
//
// Scratch: when dst has capacity >= 2*n, the second half is used as sort scratch
// so Suggest avoids heap allocations for the partition/sort buffers.
func Suggest(dst []Field, fields []Field, originalWasted int) SuggestResult {
	n := len(fields)
	if n == 0 {
		return SuggestResult{Fields: dst[:0]}
	}

	atomicCount, boolCount := countFlags(fields)

	// Prefer in-place partition into dst[0:n], using dst[n:2n] as scratch when available.
	var out []Field
	var scratch []Field
	if cap(dst) >= 2*n {
		out = dst[:n]
		scratch = dst[n : 2*n]
	} else if cap(dst) >= n {
		out = dst[:n]
		scratch = make([]Field, n)
	} else {
		out = make([]Field, n)
		scratch = make([]Field, n)
	}

	// Stable partition into scratch: atomics first, then rest (preserve relative order).
	ai, ri := 0, atomicCount
	for _, f := range fields {
		if f.IsAtomic() {
			scratch[ai] = f
			ai++
		} else {
			scratch[ri] = f
			ri++
		}
	}
	densitySort(scratch[:atomicCount])
	densitySort(scratch[atomicCount:n])

	// Relayout into out (also apply zero-size trailing rule).
	total := 0
	wasted := 0
	maxAlign := 1
	lastSize := 0
	for i := 0; i < n; i++ {
		f := scratch[i]
		if f.Align > maxAlign {
			maxAlign = f.Align
		}
		var offset int
		total, wasted, offset = alignmath.AddField(total, wasted, f.Size, f.Align)
		f.Offset = offset
		out[i] = f
		lastSize = f.Size
	}
	total, wasted, _ = alignmath.Finish(total, wasted, maxAlign, lastSize, n)

	saved := originalWasted - wasted
	if saved < 0 {
		saved = 0
	}

	notes := ruleNotes(fields, atomicCount, boolCount, originalWasted)
	return SuggestResult{
		Fields: out,
		Total:  total,
		Wasted: wasted,
		Saved:  saved,
		Notes:  notes,
	}
}

func densitySort(fields []Field) {
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Align != fields[j].Align {
			return fields[i].Align > fields[j].Align
		}
		return fields[i].Size > fields[j].Size
	})
}

func countFlags(fields []Field) (atomicCount, boolCount int) {
	for _, f := range fields {
		if f.IsAtomic() {
			atomicCount++
		}
		if f.IsBool() {
			boolCount++
		}
	}
	return atomicCount, boolCount
}

func ruleNotes(original []Field, atomicCount, boolCount, wasted int) []string {
	var notes []string

	if needsAtomicsFirstNote(original, atomicCount) {
		notes = append(notes, "atomics-first: place int64/uint64/atomic.* counters at the start of the struct (NATS/nats.go convention)")
	}

	if wasted > 0 && needsBoolPack(original, boolCount) {
		notes = append(notes, "bool-pack: 3+ bools with intervening padding — consider a flag word or bitfield")
	}

	return notes
}

func needsAtomicsFirstNote(fields []Field, atomicCount int) bool {
	return atomicCount > 0 && !atomicsAreLeading(fields)
}

func needsBoolPack(fields []Field, boolCount int) bool {
	return boolCount >= 3 && hasScatteredBools(fields)
}

func atomicsAreLeading(fields []Field) bool {
	seenNonAtomic := false
	for _, f := range fields {
		if f.IsAtomic() {
			if seenNonAtomic {
				return false
			}
		} else {
			seenNonAtomic = true
		}
	}
	return true
}

func hasScatteredBools(fields []Field) bool {
	boolSeen := false
	nonBoolAfterBool := false
	for _, f := range fields {
		if f.IsBool() {
			if nonBoolAfterBool {
				return true
			}
			boolSeen = true
		} else if boolSeen {
			nonBoolAfterBool = true
		}
	}
	return false
}

// NeedsReport returns true when the struct should be reported as an issue.
func NeedsReport(wasted int, fields []Field) bool {
	if wasted > 0 {
		return true
	}
	atomicCount, boolCount := countFlags(fields)
	if needsAtomicsFirstNote(fields, atomicCount) {
		return true
	}
	if needsBoolPack(fields, boolCount) {
		return true
	}
	return false
}
