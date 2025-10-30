package main

type ComplexStruct struct {
	ID       int32                  // 4 bytes
	Name     string                 // 16 bytes (8 bytes padding before)
	Active   bool                   // 1 byte
	Score    float64                // 8 bytes (7 bytes padding before)
	Tags     []string               // 24 bytes
	Metadata map[string]interface{} // 8 bytes
	Count    uint16                 // 2 bytes
	Enabled  bool                   // 1 byte (5 bytes padding before)
}

type AnotherBadStruct struct {
	A byte  // 1 byte
	B int64 // 8 bytes (7 bytes padding)
	C int16 // 2 bytes
	D int32 // 4 bytes (2 bytes padding)
	E bool  // 1 byte (3 bytes padding)
}
