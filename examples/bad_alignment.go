package main

import "fmt"

type BadStruct struct {
	A bool  // 1 byte
	B int64 // 8 bytes (7 bytes padding before)
	C int32 // 4 bytes
	D bool  // 1 byte (3 bytes padding before)
}

type GoodStruct struct {
	B int64 // 8 bytes
	C int32 // 4 bytes
	A bool  // 1 byte
	D bool  // 1 byte
}

// goalign:ignore
type IgnoredStruct struct {
	A bool
	B int64
	C int32
}

func main() {
	fmt.Println("Struct alignment examples")
}
