package goarch

import "testing"

func TestPtrSize(t *testing.T) {
	four := []string{"386", "arm", "mips", "mipsle", "ppc", "riscv", "wasm"}
	for _, arch := range four {
		if got := PtrSize(arch); got != 4 {
			t.Errorf("PtrSize(%q) = %d, want 4", arch, got)
		}
	}
	eight := []string{"amd64", "arm64", "mips64", "ppc64", "riscv64", "s390x", "unknown"}
	for _, arch := range eight {
		if got := PtrSize(arch); got != 8 {
			t.Errorf("PtrSize(%q) = %d, want 8", arch, got)
		}
	}
}

func TestValid(t *testing.T) {
	known := []string{
		"386", "amd64", "arm", "arm64", "mips", "mipsle", "mips64", "mips64le",
		"ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390x", "wasm",
	}
	for _, arch := range known {
		if !Valid(arch) {
			t.Errorf("Valid(%q) = false, want true", arch)
		}
	}
	if Valid("sparc") {
		t.Error("Valid(sparc) = true, want false")
	}
	if Valid("") {
		t.Error("Valid(\"\") = true, want false")
	}
}
