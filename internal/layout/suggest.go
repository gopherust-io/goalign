package layout

import "sort"

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

	atomicCount := 0
	boolCount := 0
	for _, f := range fields {
		if f.IsAtomic() {
			atomicCount++
		}
		if f.IsBool() {
			boolCount++
		}
	}

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
		pad := alignPad(total, f.Align)
		wasted += pad
		f.Offset = total + pad
		out[i] = f
		total = f.Offset + f.Size
		lastSize = f.Size
	}
	if n > 0 && lastSize == 0 && total > 0 {
		total++
		wasted++
	}
	trail := alignPad(total, maxAlign)
	wasted += trail
	total += trail

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

func ruleNotes(original []Field, atomicCount, boolCount, wasted int) []string {
	var notes []string

	if atomicCount > 0 && !atomicsAreLeading(original) {
		notes = append(notes, "atomics-first: place int64/uint64/atomic.* counters at the start of the struct (NATS/nats.go convention)")
	}

	if boolCount >= 3 && wasted > 0 && hasScatteredBools(original) {
		notes = append(notes, "bool-pack: 3+ bools with intervening padding — consider a flag word or bitfield")
	}

	return notes
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
	atomicCount := 0
	boolCount := 0
	for _, f := range fields {
		if f.IsAtomic() {
			atomicCount++
		}
		if f.IsBool() {
			boolCount++
		}
	}
	if atomicCount > 0 && !atomicsAreLeading(fields) {
		return true
	}
	if boolCount >= 3 && hasScatteredBools(fields) {
		return true
	}
	return false
}
