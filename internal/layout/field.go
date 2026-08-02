package layout

// Field describes one struct field layout slot.
type Field struct {
	Name   string
	Type   string // display only; may be empty for zero-alloc compute path
	Size   int
	Offset int
	Align  int
	Index  int // stable slot index from Compute (for fixer identity)
	Flags  FieldFlags
}

// FieldFlags marks special field categories for suggest rules.
type FieldFlags uint8

const (
	// FlagAtomic marks int64/uint64/atomic.* counter-like fields.
	FlagAtomic FieldFlags = 1 << iota
	// FlagBool marks boolean fields.
	FlagBool
	// FlagPointer marks fields that contain pointers for GC (string, slice, map, …).
	FlagPointer
	// FlagContend marks fields that must not share a cache line with other contended fields.
	FlagContend
	// FlagHot marks fields annotated // goalign:hot.
	FlagHot
	// FlagCold marks fields annotated // goalign:cold.
	FlagCold
	// FlagCachePad marks synthetic Cacheguard padding fields (_cgpadN).
	FlagCachePad
)

// DefaultCacheLine is the default CPU cache line size in bytes.
const DefaultCacheLine = 64

// IsAtomic reports whether the field should sit at the struct head.
func (f Field) IsAtomic() bool { return f.Flags&FlagAtomic != 0 }

// IsBool reports whether the field is a bool.
func (f Field) IsBool() bool { return f.Flags&FlagBool != 0 }

// IsPointer reports whether the field contributes pointer data for GC.
func (f Field) IsPointer() bool { return f.Flags&FlagPointer != 0 }

// IsContend reports whether the field is contended for Cacheguard.
func (f Field) IsContend() bool { return f.Flags&FlagContend != 0 }

// IsHot reports // goalign:hot.
func (f Field) IsHot() bool { return f.Flags&FlagHot != 0 }

// IsCold reports // goalign:cold.
func (f Field) IsCold() bool { return f.Flags&FlagCold != 0 }

// IsCachePad reports a synthetic Cacheguard pad field.
func (f Field) IsCachePad() bool { return f.Flags&FlagCachePad != 0 }

// IsCachePadName reports whether name is a Cacheguard pad identifier.
func IsCachePadName(name string) bool {
	return len(name) >= 6 && name[:6] == "_cgpad"
}
