package layout_test

import (
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/layout"
)

func TestParsePolicy(t *testing.T) {
	t.Parallel()
	p, err := layout.ParsePolicy("")
	if err != nil || p != layout.PolicyAtomics {
		t.Fatalf("empty: %v %v", p, err)
	}
	if _, err := layout.ParsePolicy("nope"); err == nil {
		t.Fatal("expected error")
	}
	p, err = layout.ParsePolicy("atomics")
	if err != nil || p != layout.PolicyAtomics {
		t.Fatalf("atomics: %v %v", p, err)
	}
}

func TestSuggestPolicies(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "A", Size: 1, Align: 1, Flags: layout.FlagBool},
		{Name: "B", Size: 8, Align: 8, Flags: layout.FlagAtomic},
		{Name: "C", Size: 4, Align: 4},
	}
	atomics := layout.SuggestWithPolicy(nil, fields, 10, layout.PolicyAtomics)
	if atomics.Fields[0].Name != "B" {
		t.Fatalf("atomics want B first: %+v", atomics.Fields)
	}
	density := layout.SuggestWithPolicy(nil, fields, 10, layout.PolicyDensity)
	if density.Fields[0].Name != "B" {
		t.Fatalf("density want B first: %+v", density.Fields)
	}
	// density should not emit atomics-first note
	for _, n := range density.Notes {
		if strings.HasPrefix(n, "atomics-first") {
			t.Fatalf("density should not note atomics-first: %v", density.Notes)
		}
	}
}

func TestPtrdataNote(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "A", Size: 1, Align: 1, Flags: layout.FlagBool},
		{Name: "S", Size: 16, Align: 8, Flags: layout.FlagPointer},
		{Name: "T", Size: 16, Align: 8, Flags: layout.FlagPointer},
	}
	sug := layout.Suggest(nil, fields, 7) // originalWasted > 0 enables ptrdata notes
	found := false
	for _, n := range sug.Notes {
		if strings.HasPrefix(n, "ptrdata") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ptrdata note: %v", sug.Notes)
	}
	if quiet := layout.Suggest(nil, fields, 0); len(quiet.Notes) != 0 {
		// no padding → keep Suggest alloc-free on the hot path
		for _, n := range quiet.Notes {
			if strings.HasPrefix(n, "ptrdata") {
				t.Fatalf("ptrdata should not fire when originalWasted=0: %v", quiet.Notes)
			}
		}
	}
}

func TestBoolPackCandidates(t *testing.T) {
	t.Parallel()
	fields := []layout.Field{
		{Name: "a", Size: 1, Align: 1, Flags: layout.FlagBool},
		{Name: "X", Size: 8, Align: 8},
		{Name: "b", Size: 1, Align: 1, Flags: layout.FlagBool},
		{Name: "c", Size: 1, Align: 1, Flags: layout.FlagBool},
		{Name: "Exported", Size: 1, Align: 1, Flags: layout.FlagBool},
	}
	got := layout.BoolPackCandidates(fields)
	if len(got) != 3 {
		t.Fatalf("got %v want 3 unexported", got)
	}
}
