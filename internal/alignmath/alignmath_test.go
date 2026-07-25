package alignmath

import "testing"

func TestAlignPad(t *testing.T) {
	tests := []struct {
		offset, align, want int
	}{
		{0, 1, 0},
		{0, 8, 0},
		{1, 8, 7},
		{4, 8, 4},
		{8, 8, 0},
		{3, 4, 1},
		{5, 0, 0},
		{5, -1, 0},
	}
	for _, tt := range tests {
		if got := AlignPad(tt.offset, tt.align); got != tt.want {
			t.Errorf("AlignPad(%d, %d) = %d, want %d", tt.offset, tt.align, got, tt.want)
		}
	}
}

func TestAddField(t *testing.T) {
	total, wasted, offset := AddField(0, 0, 1, 1)
	if total != 1 || wasted != 0 || offset != 0 {
		t.Fatalf("bool at start: total=%d wasted=%d offset=%d", total, wasted, offset)
	}
	total, wasted, offset = AddField(total, wasted, 8, 8)
	if offset != 8 || wasted != 7 || total != 16 {
		t.Fatalf("int64 after bool: total=%d wasted=%d offset=%d", total, wasted, offset)
	}
}

func TestFinish(t *testing.T) {
	// bool + int64 → total 16, wasted 7 after Finish with maxAlign 8
	total, wasted, maxAlign := Finish(16, 7, 8, 8, 2)
	if total != 16 || wasted != 7 || maxAlign != 8 {
		t.Fatalf("Finish after int64: total=%d wasted=%d maxAlign=%d", total, wasted, maxAlign)
	}

	// trailing zero-sized field bump
	total, wasted, maxAlign = Finish(8, 0, 8, 0, 2)
	if total != 16 || wasted != 8 || maxAlign != 8 {
		t.Fatalf("zero-size trail: total=%d wasted=%d maxAlign=%d", total, wasted, maxAlign)
	}

	// empty / all-zero: no bump when total==0
	total, wasted, maxAlign = Finish(0, 0, 1, 0, 1)
	if total != 0 || wasted != 0 || maxAlign != 1 {
		t.Fatalf("all-zero: total=%d wasted=%d maxAlign=%d", total, wasted, maxAlign)
	}

	// maxAlign < 1 clamped
	total, wasted, maxAlign = Finish(1, 0, 0, 1, 1)
	if maxAlign != 1 {
		t.Fatalf("maxAlign clamp: got %d", maxAlign)
	}
	_ = total
	_ = wasted
}

func BenchmarkAlignPad(b *testing.B) {
	var sink int
	for b.Loop() {
		sink = AlignPad(5, 8)
	}
	_ = sink
}

func BenchmarkFinish(b *testing.B) {
	var t, w, m int
	for b.Loop() {
		t, w, m = Finish(15, 7, 8, 1, 3)
	}
	_, _, _ = t, w, m
}

func BenchmarkAddField(b *testing.B) {
	var total, wasted, offset int
	for b.Loop() {
		total, wasted, offset = AddField(1, 0, 8, 8)
	}
	_, _, _ = total, wasted, offset
}
