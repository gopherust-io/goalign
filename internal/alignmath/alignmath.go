// Package alignmath provides pure struct layout size/alignment arithmetic
// shared by layout.Compute, Suggest, and nested struct sizing.
package alignmath

// AlignPad returns padding bytes needed so offset is aligned to align.
func AlignPad(offset, align int) int {
	if align <= 1 {
		return 0
	}
	return (align - (offset % align)) % align
}

// AddField places one field of the given size/align at the current total,
// returning the new total, wasted (including this field's pad), and field offset.
func AddField(total, wasted, size, align int) (newTotal, newWasted, offset int) {
	pad := AlignPad(total, align)
	wasted += pad
	offset = total + pad
	return offset + size, wasted, offset
}

// Finish applies the gc ABI trailing rules: if the last field is zero-sized
// and the struct already has non-zero size, bump by one byte; then pad to
// maxAlign. fieldCount is the number of fields placed (0 skips the zero-size bump).
func Finish(total, wasted, maxAlign, lastSize, fieldCount int) (newTotal, newWasted, newMaxAlign int) {
	if fieldCount > 0 && lastSize == 0 && total > 0 {
		total++
		wasted++
	}
	trail := AlignPad(total, maxAlign)
	wasted += trail
	total += trail
	if maxAlign < 1 {
		maxAlign = 1
	}
	return total, wasted, maxAlign
}
