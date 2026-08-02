package layout

import (
	"fmt"

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

// Suggest reorders fields with the default atomics-first policy.
func Suggest(dst []Field, fields []Field, originalWasted int) SuggestResult {
	return SuggestWithPolicy(dst, fields, originalWasted, PolicyAtomics)
}

// SuggestWithPolicy reorders fields according to policy.
//
// Scratch: when dst has capacity >= 2*n, the second half is used as sort scratch
// so Suggest avoids heap allocations for the partition/sort buffers.
func SuggestWithPolicy(dst []Field, fields []Field, originalWasted int, policy Policy) SuggestResult {
	n := len(fields)
	if n == 0 {
		return SuggestResult{Fields: dst[:0]}
	}
	if policy == "" {
		policy = PolicyAtomics
	}

	atomicCount, boolCount := countFlags(fields)

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

	switch policy {
	case PolicyDensity, PolicyStable:
		copy(scratch, fields)
		if policy == PolicyStable {
			densitySortStable(scratch)
		} else {
			densitySort(scratch)
		}
	default: // PolicyAtomics
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
	}

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
		var ok bool
		total, wasted, offset, ok = alignmath.AddField(total, wasted, f.Size, f.Align)
		if !ok {
			return SuggestResult{Fields: dst[:0]}
		}
		f.Offset = offset
		out[i] = f
		lastSize = f.Size
	}
	var ok bool
	total, wasted, _, ok = alignmath.Finish(total, wasted, maxAlign, lastSize, n)
	if !ok {
		return SuggestResult{Fields: dst[:0]}
	}

	saved := originalWasted - wasted
	if saved < 0 {
		saved = 0
	}

	notes := ruleNotes(fields, atomicCount, boolCount, originalWasted, policy)
	return SuggestResult{
		Fields: out,
		Total:  total,
		Wasted: wasted,
		Saved:  saved,
		Notes:  notes,
	}
}

// densitySort is a stable insertion sort by align desc, then size desc.
// Hand-rolled to keep the slice off the heap (sort.SliceStable / slices.Sort*
// force the backing array to escape).
func densitySort(fields []Field) {
	for i := 1; i < len(fields); i++ {
		key := fields[i]
		j := i - 1
		for j >= 0 && densityLess(key, fields[j]) {
			fields[j+1] = fields[j]
			j--
		}
		fields[j+1] = key
	}
}

// densitySortStable sorts by align/size desc, preserving original relative
// order for equal keys (insertion sort is already stable).
func densitySortStable(fields []Field) {
	densitySort(fields)
}

func densityLess(a, b Field) bool {
	if a.Align != b.Align {
		return a.Align > b.Align
	}
	return a.Size > b.Size
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

func ruleNotes(original []Field, atomicCount, boolCount, wasted int, policy Policy) []string {
	var notes []string

	if policy == PolicyAtomics && needsAtomicsFirstNote(original, atomicCount) {
		notes = append(notes, "atomics-first: place int64/uint64/atomic.* counters at the start of the struct")
	}

	if wasted > 0 && needsBoolPack(original, boolCount) {
		notes = append(notes, "bool-pack: 3+ bools with intervening padding — consider a flag word or bitfield")
	}

	if wasted > 0 {
		if note := ptrdataNote(original); note != "" {
			notes = append(notes, note)
		}
	}

	return notes
}

func ptrdataNote(fields []Field) string {
	ptrBytes := 0
	total := 0
	ptrFields := 0
	for _, f := range fields {
		total += f.Size
		if f.IsPointer() {
			ptrFields++
			ptrBytes += f.Size
		}
	}
	if ptrFields == 0 || total == 0 {
		return ""
	}
	// Only advise when pointers are a meaningful fraction of the object.
	if ptrBytes*4 < total && ptrFields < 2 {
		return ""
	}
	return fmt.Sprintf("ptrdata: %d/%d field bytes are pointer-bearing (%d fields) — denser packing can improve GC scan efficiency", ptrBytes, total, ptrFields)
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
	return NeedsReportCacheLine(wasted, fields, DefaultCacheLine)
}

// NeedsReportCacheLine is NeedsReport with an explicit cache line size.
func NeedsReportCacheLine(wasted int, fields []Field, cacheLine int) bool {
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
	if HasFalseShare(fields, cacheLine) {
		return true
	}
	return false
}

// BoolPackCandidates returns unexported bool field names suitable for flag-word rewrite.
func BoolPackCandidates(fields []Field) []string {
	if !needsBoolPack(fields, countBools(fields)) {
		return nil
	}
	var names []string
	for _, f := range fields {
		if !f.IsBool() {
			continue
		}
		if f.Name == "" || f.Name == "_" {
			continue
		}
		if f.Name[0] < 'a' || f.Name[0] > 'z' {
			continue // exported — skip for safety
		}
		names = append(names, f.Name)
	}
	if len(names) < 3 {
		return nil
	}
	return names
}

func countBools(fields []Field) int {
	_, b := countFlags(fields)
	return b
}
