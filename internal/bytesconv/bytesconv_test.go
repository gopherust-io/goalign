package bytesconv_test

import (
	"testing"

	"github.com/gopherust-io/goalign/internal/bytesconv"
)

func TestIsEmpty(t *testing.T) {
	t.Parallel()
	if !bytesconv.IsEmpty("") {
		t.Fatal("empty")
	}
	if bytesconv.IsEmpty("x") {
		t.Fatal("non-empty")
	}
}

func TestStringBytesRoundTrip(t *testing.T) {
	t.Parallel()
	if bytesconv.StringToBytes("") != nil {
		t.Fatal("empty string -> nil slice")
	}
	s := "hello"
	b := bytesconv.StringToBytes(s)
	if string(b) != s {
		t.Fatalf("got %q", b)
	}
	if bytesconv.BytesToString(nil) != "" {
		t.Fatal("nil bytes")
	}
	if bytesconv.BytesToString(b) != s {
		t.Fatal("round trip")
	}
}

func BenchmarkStringToBytes(b *testing.B) {
	s := "struct alignment padding waste"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bytesconv.StringToBytes(s)
	}
}

func BenchmarkBytesToString(b *testing.B) {
	raw := []byte("struct alignment padding waste")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = bytesconv.BytesToString(raw)
	}
}
