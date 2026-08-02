package layout_test

import (
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/layout"
)

func TestFalseShareTwoAtomics(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "A", Size: 8, Align: 8, Offset: 0, Flags: layout.FlagAtomic | layout.FlagContend},
		{Name: "B", Size: 8, Align: 8, Offset: 8, Flags: layout.FlagAtomic | layout.FlagContend},
	}
	if !layout.HasFalseShare(fields, 64) {
		t.Fatal("expected collision")
	}
	notes := layout.FalseShareNotes(fields, 64)
	if len(notes) == 0 || !strings.Contains(notes[0], "false-share") {
		t.Fatalf("notes=%v", notes)
	}
	guarded, n := layout.ApplyCacheguard(fields, 64)
	if n < 1 {
		t.Fatalf("expected pads, n=%d fields=%+v", n, guarded)
	}
	padSize := 0
	for _, f := range guarded {
		if f.IsCachePad() {
			padSize = f.Size
		}
	}
	if padSize != 56 {
		t.Fatalf("pad size=%d want 56 (8 → next 64-byte line)", padSize)
	}
	relayout, _, _, ok := layout.Relayout(guarded)
	if !ok {
		t.Fatal("relayout failed")
	}
	if layout.HasFalseShare(relayout, 64) {
		t.Fatalf("still colliding: %+v", relayout)
	}
}

func TestCacheguardIdempotent(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "A", Size: 8, Align: 8, Flags: layout.FlagContend},
		{Name: "B", Size: 8, Align: 8, Flags: layout.FlagContend},
	}
	once, n1 := layout.ApplyCacheguard(fields, 64)
	twice, n2 := layout.ApplyCacheguard(once, 64)
	if n1 < 1 {
		t.Fatal("first pass")
	}
	if n2 != n1 {
		t.Fatalf("re-apply after strip should insert same pad count; n1=%d n2=%d", n1, n2)
	}
	padOnce, padTwice := 0, 0
	for _, f := range once {
		if f.IsCachePad() {
			padOnce++
		}
	}
	for _, f := range twice {
		if f.IsCachePad() {
			padTwice++
		}
	}
	if padTwice != padOnce {
		t.Fatalf("pads stacked: %d -> %d", padOnce, padTwice)
	}
	relayout, _, _, ok := layout.Relayout(twice)
	if !ok || layout.HasFalseShare(relayout, 64) {
		t.Fatalf("not isolated: %+v", relayout)
	}
}

func TestNeedsReportFalseShare(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "A", Size: 8, Align: 8, Offset: 0, Flags: layout.FlagContend},
		{Name: "B", Size: 8, Align: 8, Offset: 8, Flags: layout.FlagContend},
	}
	if !layout.NeedsReportCacheLine(0, fields, 64) {
		t.Fatal("0-waste false-share should report")
	}
}

func TestContendAnnotationOnly(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "x", Size: 4, Align: 4, Offset: 0, Flags: layout.FlagContend},
		{Name: "y", Size: 4, Align: 4, Offset: 4, Flags: layout.FlagContend},
	}
	if !layout.HasFalseShare(fields, 64) {
		t.Fatal("expected")
	}
}

func TestCollisionKeyOrderIndependent(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "B", Size: 8, Align: 8, Offset: 0, Flags: layout.FlagContend},
		{Name: "A", Size: 8, Align: 8, Offset: 8, Flags: layout.FlagContend},
	}
	cols := layout.FalseShareCollisions(fields, 64)
	if len(cols) != 1 {
		t.Fatalf("cols=%+v", cols)
	}
	if cols[0].A != "A" || cols[0].B != "B" {
		t.Fatalf("want sorted A,B got %+v", cols[0])
	}
}
