package analyzer_test

import (
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/analyzer"
	"github.com/gopherust-io/goalign/internal/layout"
)

func TestAnalyzeFalseShareNotes(t *testing.T) {
	src := `package p
import "sync/atomic"

type Hot struct {
	A atomic.Int64
	B atomic.Int64
}
`
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected false-share finding")
	}
	found := false
	for _, n := range res.Issues[0].Notes {
		if strings.HasPrefix(n, "false-share") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notes=%v", res.Issues[0].Notes)
	}
}

func TestAnalyzeCacheguardSuggest(t *testing.T) {
	src := `package p
import "sync/atomic"

type Hot struct {
	A atomic.Int64
	B atomic.Int64
}
`
	res, err := analyzer.AnalyzeSourceWithOptions("x.go", []byte(src), analyzer.Options{
		Sizer:      layout.SizerFor("amd64"),
		Policy:     layout.PolicyAtomics,
		CacheLine:  64,
		Cacheguard: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issue")
	}
	hasPad := false
	for _, f := range res.Issues[0].Suggested {
		if layout.IsCachePadName(f.Name) {
			hasPad = true
		}
	}
	if !hasPad {
		t.Fatalf("expected pads in suggested: %+v", res.Issues[0].Suggested)
	}
	if layout.HasFalseShare(res.Issues[0].Suggested, 64) {
		t.Fatalf("suggested still shares: %+v", res.Issues[0].Suggested)
	}
}

func TestAnalyzeContendDirective(t *testing.T) {
	src := `package p
type S struct {
	A int32 // goalign:contend
	B int32 // goalign:contend
}
`
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected contend annotation finding")
	}
}

func TestAnalyzeMutexCounter(t *testing.T) {
	src := `package p
import "sync"

type S struct {
	mu sync.Mutex
	n  int64
}
`
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected mutex+int64 finding (atomics-first and/or padding)")
	}
}

func TestPlainInt64NoFalseShare(t *testing.T) {
	src := `package p
type S struct {
	A int64
	B int64
}
`
	res, err := analyzer.AnalyzeSource("x.go", []byte(src), layout.SizerFor("amd64"))
	if err != nil {
		t.Fatal(err)
	}
	for _, iss := range res.Issues {
		for _, n := range iss.Notes {
			if strings.HasPrefix(n, "false-share") {
				t.Fatalf("plain int64 pair must not false-share without annotation: %v", iss.Notes)
			}
		}
	}
}

func TestIssueCacheLineSet(t *testing.T) {
	src := `package p
import "sync/atomic"
type Hot struct {
	A atomic.Int64
	B atomic.Int64
}
`
	res, err := analyzer.AnalyzeSourceWithOptions("x.go", []byte(src), analyzer.Options{
		Sizer:     layout.SizerFor("amd64"),
		CacheLine: 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Issues) == 0 {
		t.Fatal("expected issue")
	}
	if res.Issues[0].CacheLine != 128 {
		t.Fatalf("CacheLine=%d want 128", res.Issues[0].CacheLine)
	}
}
