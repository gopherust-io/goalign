package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopherust-io/goalign/internal/config"
)

func TestLoadMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg, path, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" || cfg.Arch != "" {
		t.Fatalf("path=%q cfg=%+v", path, cfg)
	}
}

func TestLoadYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := `
arch: amd64
min-waste: 8
exclude:
  - testdata/
  - vendor/
fail-on-findings: true
policy: density
skip-generated: true
generated:
  - "*.pb.go"
ignore:
  - "**/generated/**"
`
	if err := os.WriteFile(filepath.Join(dir, ".goalign.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "pkg", "x")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, path, err := config.Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != ".goalign.yml" {
		t.Fatalf("path=%q", path)
	}
	if cfg.Arch != "amd64" {
		t.Fatalf("arch=%q", cfg.Arch)
	}
	if cfg.MinWaste == nil || *cfg.MinWaste != 8 {
		t.Fatalf("min-waste=%v", cfg.MinWaste)
	}
	if len(cfg.Exclude) != 2 || cfg.Exclude[0] != "testdata/" {
		t.Fatalf("exclude=%v", cfg.Exclude)
	}
	if cfg.FailOnFindings == nil || !*cfg.FailOnFindings {
		t.Fatal("fail-on-findings")
	}
	if cfg.Policy != "density" {
		t.Fatalf("policy=%q", cfg.Policy)
	}
	if cfg.SkipGenerated == nil || !*cfg.SkipGenerated {
		t.Fatal("skip-generated")
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "goalign.yml"), []byte("min-waste: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := config.Load(dir); err == nil {
		t.Fatal("expected parse error")
	}
}
