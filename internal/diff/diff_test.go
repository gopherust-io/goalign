package diff_test

import (
	"strings"
	"testing"

	"github.com/gopherust-io/goalign/internal/diff"
)

func TestUnifiedEmpty(t *testing.T) {
	t.Parallel()
	if got := diff.Unified("a.go", []byte("x"), []byte("x")); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestUnifiedChange(t *testing.T) {
	t.Parallel()
	old := "package p\n\ntype S struct {\n\tA bool\n\tB int64\n}\n"
	new := "package p\n\ntype S struct {\n\tB int64\n\tA bool\n}\n"
	got := diff.Unified("x.go", []byte(old), []byte(new))
	if !strings.Contains(got, "--- a/x.go") || !strings.Contains(got, "+++ b/x.go") {
		t.Fatalf("headers:\n%s", got)
	}
	if !strings.Contains(got, "-	A bool") || !strings.Contains(got, "+	A bool") {
		t.Fatalf("body:\n%s", got)
	}
}
