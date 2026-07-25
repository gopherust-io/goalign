// Package goarch maps GOARCH names to pointer size and validates known arches.
package goarch

// PtrSize returns the pointer size in bytes for goarch (4 or 8).
func PtrSize(goarch string) int {
	switch goarch {
	case "386", "arm", "mips", "mipsle", "ppc", "riscv", "wasm":
		return 4
	default:
		return 8
	}
}

// Valid reports whether goarch is a known GOARCH name.
func Valid(goarch string) bool {
	switch goarch {
	case "386", "amd64", "arm", "arm64", "mips", "mipsle", "mips64", "mips64le",
		"ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390x", "wasm":
		return true
	default:
		return false
	}
}
