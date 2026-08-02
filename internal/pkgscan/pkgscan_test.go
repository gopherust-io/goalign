package pkgscan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopherust-io/goalign/internal/pkgscan"
)

func TestLoadTypeSizesLocal(t *testing.T) {
	dir := t.TempDir()
	mod := `module example.com/p

go 1.22
`
	src := `package p

type Wide struct {
	A int64
	B bool
}
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	sizes, err := pkgscan.LoadTypeSizes([]string{"."}, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	info, ok := sizes["Wide"]
	if !ok || info.Size != 16 {
		t.Fatalf("Wide=%v ok=%v sizes=%v", info, ok, sizes)
	}
}

func TestLoadTypeSizesImports(t *testing.T) {
	dir := t.TempDir()
	mod := `module example.com/p

go 1.22
`
	src := `package p

import "io"

type S struct {
	R io.Reader
}
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	sizes, err := pkgscan.LoadTypeSizes([]string{"."}, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	info, ok := sizes["io.Reader"]
	if !ok {
		keys := make([]string, 0, 20)
		for k := range sizes {
			keys = append(keys, k)
			if len(keys) >= 20 {
				break
			}
		}
		t.Fatalf("io.Reader missing: sample keys=%v", keys)
	}
	// io.Reader is an interface → 16 bytes on amd64
	if info.Size != 16 {
		t.Fatalf("io.Reader size=%d want 16 (%v)", info.Size, info)
	}
}
