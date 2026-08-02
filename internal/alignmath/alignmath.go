// Package alignmath provides pure struct layout size/alignment arithmetic
// shared by layout.Compute, Suggest, and nested struct sizing.
package alignmath

import "math"

// AlignPad returns padding bytes needed so offset is aligned to align.
func AlignPad(offset, align int) int {
	if align <= 1 {
		return 0
	}
	return (align - (offset % align)) % align
}

// AddField places one field of the given size/align at the current total,
// returning the new total, wasted (including this field's pad), and field offset.
// ok is false when the placement would overflow int.
func AddField(total, wasted, size, align int) (newTotal, newWasted, offset int, ok bool) {
	if size < 0 || total < 0 || wasted < 0 {
		return 0, 0, 0, false
	}
	pad := AlignPad(total, align)
	if total > math.MaxInt-pad {
		return 0, 0, 0, false
	}
	offset = total + pad
	if offset > math.MaxInt-size {
		return 0, 0, 0, false
	}
	if wasted > math.MaxInt-pad {
		return 0, 0, 0, false
	}
	return offset + size, wasted + pad, offset, true
}

// Finish applies the gc ABI trailing rules: if the last field is zero-sized
// and the struct already has non-zero size, bump by one byte; then pad to
// maxAlign. fieldCount is the number of fields placed (0 skips the zero-size bump).
// ok is false when trailing math would overflow int.
func Finish(total, wasted, maxAlign, lastSize, fieldCount int) (newTotal, newWasted, newMaxAlign int, ok bool) {
	if total < 0 || wasted < 0 {
		return 0, 0, 1, false
	}
	if fieldCount > 0 && lastSize == 0 && total > 0 {
		if total == math.MaxInt || wasted == math.MaxInt {
			return 0, 0, 1, false
		}
		total++
		wasted++
	}
	trail := AlignPad(total, maxAlign)
	if total > math.MaxInt-trail || wasted > math.MaxInt-trail {
		return 0, 0, 1, false
	}
	wasted += trail
	total += trail
	if maxAlign < 1 {
		maxAlign = 1
	}
	return total, wasted, maxAlign, true
}
