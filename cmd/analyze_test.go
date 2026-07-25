package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopherust-io/goalign/internal/layout"
)

func TestFindGoFilesWalkRootVendor(t *testing.T) {
	dir := t.TempDir()
	vendor := filepath.Join(dir, "vendor")
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(vendor, "x.go")
	if err := os.WriteFile(src, []byte("package v\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := findGoFiles(vendor, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%v want 1 (walk root vendor must not SkipDir)", files)
	}
}

func TestFindGoFilesSkipsChildVendor(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vendor := filepath.Join(dir, "vendor")
	_ = os.MkdirAll(vendor, 0o755)
	_ = os.WriteFile(filepath.Join(vendor, "x.go"), []byte("package v\n"), 0o644)

	files, err := findGoFiles(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files=%v want only main.go", files)
	}
}

func TestValidArchHelper(t *testing.T) {
	if layout.ValidArch("nope") {
		t.Fatal("expected invalid")
	}
}
