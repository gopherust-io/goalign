package cmd

import (
	"testing"
	"unicode/utf8"

	"github.com/gopherust-io/goalign/internal/bytesconv"
)

func TestDecodeVarintTagField16(t *testing.T) {
	// Field 16, wire type 0 (varint), value 7 → tag = (16<<3)|0 = 0x80,0x01 then value 0x07
	data := []byte{0x80, 0x01, 0x07}
	fields := parseProtobufFields(data, 0)
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d: %+v", len(fields), fields)
	}
	if fields[0]["field_number"] != 16 {
		t.Fatalf("field_number=%v want 16", fields[0]["field_number"])
	}
	if fields[0]["value"] != uint64(7) {
		t.Fatalf("value=%v want 7", fields[0]["value"])
	}
}

func TestIsValidUTF8(t *testing.T) {
	if !utf8.Valid(bytesconv.StringToBytes("hi")) {
		t.Fatal("ascii should be valid utf8")
	}
	if utf8.Valid([]byte{0xff, 0xfe, 0xfd}) {
		t.Fatal("invalid bytes should not be valid utf8")
	}
}

func TestMessageBytesFromFrameRejectsCompressed(t *testing.T) {
	bytes := []byte{0x01, 0x00, 0x00, 0x00, 0x01, 0x00}
	_, err := messageBytesFromFrame(bytes)
	if err == nil {
		t.Fatal("expected error for compressed frame")
	}
}

func TestMessageBytesFromFrameRawProtobufNotCompressed(t *testing.T) {
	// Typical protobuf field 1 varint — first byte is 0x08, not a gRPC compression flag.
	bytes := []byte{0x08, 0x01}
	got, err := messageBytesFromFrame(bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(bytes) {
		t.Fatalf("got len=%d want raw len=%d", len(got), len(bytes))
	}
}

func TestMessageBytesFromFrameInvalidLengthKeepsRaw(t *testing.T) {
	// Length claims more bytes than available; must not slice from byte 5.
	bytes := []byte{0x00, 0x00, 0x00, 0x00, 0xff, 0x08, 0x01}
	got, err := messageBytesFromFrame(bytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(bytes) {
		t.Fatalf("got len=%d want raw payload len=%d", len(got), len(bytes))
	}
}

func TestParseProtobufFieldsHugeLengthNoPanic(t *testing.T) {
	// Length-delimited tag (field 1) + oversized length varint must not panic.
	data := []byte{0x0a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}
	fields := parseProtobufFields(data, 0)
	_ = fields // may be empty or partial; must not panic
}
