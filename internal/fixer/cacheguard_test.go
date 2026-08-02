package fixer_test

import (
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/fixer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestFixCacheguardDiff(t *testing.T) {
	src := `package p

import "sync/atomic"

type Hot struct {
	A atomic.Int64
	B atomic.Int64
}
`
	opts := analyzer.Options{
		Sizer:      layout.SizerFor("amd64"),
		Cacheguard: true,
		CacheLine:  64,
	}
	res, err := analyzer.AnalyzeSourceWithOptions("x.go", []byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	fr, err := fixer.FixContentWithOptions("x.go", []byte(src), res.Issues, fixer.Options{
		DiffOnly:   true,
		Cacheguard: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fr.Changed || fr.Diff == "" {
		t.Fatalf("expected diff: %+v", fr)
	}
	if !strings.Contains(fr.Diff, "_cgpad") {
		t.Fatalf("diff missing pad:\n%s", fr.Diff)
	}
}

func TestFixCacheguardIdempotent(t *testing.T) {
	src := `package p

import "sync/atomic"

type Hot struct {
	A atomic.Int64
	B atomic.Int64
}
`
	opts := analyzer.Options{Sizer: layout.SizerFor("amd64"), Cacheguard: true, CacheLine: 64}
	res, err := analyzer.AnalyzeSourceWithOptions("x.go", []byte(src), opts)
	if err != nil {
		t.Fatal(err)
	}
	out, n, _, err := fixer.FixFileWithOptions("x.go", []byte(src), res.Issues, fixer.Options{Cacheguard: true})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	res2, err := analyzer.AnalyzeSourceWithOptions("x.go", out, opts)
	if err != nil {
		t.Fatal(err)
	}
	out2, n2, _, err := fixer.FixFileWithOptions("x.go", out, res2.Issues, fixer.Options{Cacheguard: true})
	if err != nil {
		t.Fatal(err)
	}
	// Second apply should be no-op or not stack pads
	padCount := strings.Count(string(out), "_cgpad")
	padCount2 := strings.Count(string(out2), "_cgpad")
	if padCount2 > padCount {
		t.Fatalf("pads stacked: %d -> %d\n%s", padCount, padCount2, out2)
	}
	if n2 > 0 && string(out) != string(out2) {
		// allow rewrite if identical pad layout reformatted
		if padCount2 != padCount {
			t.Fatalf("unexpected change n2=%d", n2)
		}
	}
}
