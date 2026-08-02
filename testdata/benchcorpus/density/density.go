package density

// Density corpus: std types only. Fair size-savings cases for goalign vs
// betteralign / fieldalignment (no imported named types).

type D01 struct {
	A bool
	B int64
	C int32
	D bool
}

type D02 struct {
	A byte
	B int64
	C int16
	D int32
	E bool
}

type D03 struct {
	Flag  bool
	Count int64
	ID    int32
	Ok    bool
}

type D04 struct {
	X bool
	Y float64
	Z int32
	W bool
}

type D05 struct {
	A int8
	B int64
	C int8
	D int64
}

type D06 struct {
	P bool
	Q complex128
	R bool
}

type D07 struct {
	A uint8
	B uint64
	C uint16
	D uint32
}

type D08 struct {
	Name   string
	ID     int32
	Active bool
	Score  float64
}

type D09 struct {
	A bool
	B string
	C bool
	D int64
}

type D10 struct {
	A int16
	B int64
	C int16
	D int32
}

type D11 struct {
	A bool
	B [3]byte
	C int64
}

type D12 struct {
	A byte
	B [7]byte
	C int64
}

type D13 struct {
	A bool
	B []byte
	C bool
}

type D14 struct {
	A int32
	B string
	C bool
	D []string
	E uint16
}

type D15 struct {
	A bool
	B map[string]int
	C int64
}

type D16 struct {
	A uintptr
	B bool
	C int32
}

type D17 struct {
	A bool
	B *int64
	C bool
}

type D18 struct {
	A int8
	B int16
	C int32
	D int64
	E bool
}

type D19 struct {
	A bool
	B int8
	C int16
	D int32
	E int64
}

type D20 struct {
	A float32
	B bool
	C float64
	D bool
}

type D21 struct {
	A bool
	B chan int
	C bool
}

type D22 struct {
	A func()
	B bool
	C int64
}

type D23 struct {
	A bool
	B interface{}
	C int32
}

type D24 struct {
	A int64
	B bool
	C int32
	D bool
	E int16
	F bool
}

type D25 struct {
	A byte
	B byte
	C byte
	D int64
}

type D26 struct {
	A int32
	B bool
	C int32
	D bool
}

type D27 struct {
	A uint32
	B bool
	C uint64
}

type D28 struct {
	A bool
	B complex64
	C bool
}

type D29 struct {
	A int16
	B bool
	C int64
	D bool
}

type D30 struct {
	A bool
	B int32
	C bool
	D int64
	E bool
	F int16
}

type D31 struct {
	A [1]int64
	B bool
	C int32
}

type D32 struct {
	A bool
	B [2]int32
	C bool
}

type D33 struct {
	A byte
	B int32
	C byte
	D int64
}

type D34 struct {
	A bool
	B uint
	C bool
}

type D35 struct {
	A int8
	B string
	C int8
	D float64
}

type NestedInner struct {
	A bool
	B int64
}

type D36 struct {
	Outer bool
	Inner NestedInner
	Tail  bool
}

type D37 struct {
	A bool
	B struct {
		X int64
		Y bool
	}
	C int32
}

type D38 struct {
	A int64
	B bool
	C bool
	D bool
	E int32
}

type D39 struct {
	A bool
	B bool
	C bool
	D int64
}

type D40 struct {
	A int32
	B int64
	C bool
	D float64
	E uint16
	F bool
}

type E01 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E02 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E03 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E04 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E05 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E06 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E07 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E08 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E09 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E10 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E11 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E12 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E13 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E14 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E15 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E16 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E17 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E18 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E19 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E20 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E21 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E22 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E23 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E24 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E25 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E26 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E27 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E28 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E29 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E30 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E31 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E32 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E33 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E34 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E35 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E36 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E37 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E38 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E39 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E40 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E41 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E42 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E43 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E44 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E45 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E46 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E47 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E48 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E49 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E50 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E51 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E52 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E53 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E54 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E55 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E56 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E57 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E58 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E59 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E60 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E61 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E62 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E63 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E64 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E65 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E66 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E67 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E68 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E69 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E70 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E71 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E72 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E73 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E74 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E75 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E76 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E77 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E78 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E79 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}

type E80 struct {
	A bool
	B int64
	C int32
	D bool
	E byte
	F float64
	G int16
	H bool
}
