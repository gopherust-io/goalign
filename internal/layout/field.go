package layout

// Field describes one struct field layout slot.
type Field struct {
	Name   string
	Type   string // display only; may be empty for zero-alloc compute path
	Size   int
	Offset int
	Align  int
	Flags  FieldFlags
}

// FieldFlags marks special field categories for NATS-style rules.
type FieldFlags uint8

const (
	// FlagAtomic marks int64/uint64/atomic.* counter-like fields.
	FlagAtomic FieldFlags = 1 << iota
	// FlagBool marks boolean fields.
	FlagBool
)

// IsAtomic reports whether the field should sit at the struct head.
func (f Field) IsAtomic() bool { return f.Flags&FlagAtomic != 0 }

// IsBool reports whether the field is a bool.
func (f Field) IsBool() bool { return f.Flags&FlagBool != 0 }
