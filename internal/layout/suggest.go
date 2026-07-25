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
func Suggest(dst []Field, fields []Field, originalWasted int) SuggestResult {
	n := len(fields)
	if n == 0 {
		return SuggestResult{Fields: dst[:0]}
	}

	// Work on a copy of indices to avoid mutating caller's Fields.
	type item struct {
		f      Field
		atomic bool
	}
	items := make([]item, n)
	atomicCount := 0
	boolCount := 0
	for i, f := range fields {
		items[i] = item{f: f, atomic: f.IsAtomic()}
		if f.IsAtomic() {
			atomicCount++
		}
		if f.IsBool() {
			boolCount++
		}
	}

	// Stable partition: atomics first, preserving relative order within groups,
	// then density-sort non-atomics; atomics themselves density-sorted too.
	atomics := make([]Field, 0, atomicCount)
	rest := make([]Field, 0, n-atomicCount)
	for _, it := range items {
		if it.atomic {
			atomics = append(atomics, it.f)
		} else {
			rest = append(rest, it.f)
		}
	}
	densitySort(atomics)
	densitySort(rest)

	ordered := make([]Field, 0, n)
	ordered = append(ordered, atomics...)
	ordered = append(ordered, rest...)

	// Relayout
	total := 0
	wasted := 0
	maxAlign := 1
	out := dst
	if cap(out) < n {
		out = make([]Field, n)
	} else {
		out = out[:n]
	}
	for i, f := range ordered {
		if f.Align > maxAlign {
			maxAlign = f.Align
		}
		pad := alignPad(total, f.Align)
		wasted += pad
		f.Offset = total + pad
		out[i] = f
		total = f.Offset + f.Size
	}
	trail := alignPad(total, maxAlign)
	wasted += trail
	total += trail

	saved := originalWasted - wasted
	if saved < 0 {
		saved = 0
	}

	notes := ruleNotes(fields, ordered, atomicCount, boolCount, originalWasted)
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

func ruleNotes(original, suggested []Field, atomicCount, boolCount, wasted int) []string {
	var notes []string

	if atomicCount > 0 && !atomicsAreLeading(original) {
		notes = append(notes, "atomics-first: place int64/uint64/atomic.* counters at the start of the struct (NATS/nats.go convention)")
	}

	if boolCount >= 3 && wasted > 0 && hasScatteredBools(original) {
		notes = append(notes, "bool-pack: 3+ bools with intervening padding — consider a flag word or bitfield")
	}

	_ = suggested
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
