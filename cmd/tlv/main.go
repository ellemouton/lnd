package main

import (
	"bytes"
	"fmt"
)

// MessageEx1 has some compulsory fields and some optional ones.
type MessageEx1 struct {
	Item1 TLVInt64  `tlv:"1"`
	Item2 TLVBytes  `tlv:"2"`
	Item3 *TLVBytes `tlv:"3"`
}

// MessageEx2 has only optional fields.
type MessageEx2 struct {
	Item1 *TLVInt64 `tlv:"1"`
	Item2 *TLVBytes `tlv:"2"`
}

func main() {
	var (
		b1 = TLVBytes{1, 3, 4}
		b2 = TLVBytes{3, 2, 1}
		i1 = TLVInt64(15)
	)

	m1 := MessageEx1{
		Item1: TLVInt64(10),
		Item2: b1,
		Item3: &b2,
	}
	printEncoding("m1", m1)

	m2 := MessageEx1{
		Item1: TLVInt64(10),
		Item2: b1,
	}
	printEncoding("m2", m2)

	m3 := MessageEx1{
		Item1: TLVInt64(10),
		Item2: b1,
		Item3: &b2,
	}
	printEncoding("m3", m3)

	m4 := MessageEx2{
		Item1: &i1,
		Item2: &b2,
	}
	printEncoding("m4", m4)

	m5 := MessageEx2{}
	printEncoding("m5", m5)
}

func printEncoding(label string, v interface{}) {
	stream, err := Encode(v)
	if err != nil {
		panic(err)
	}

	buf := bytes.NewBuffer(nil)
	err = stream.Encode(buf)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %x\n", label, buf.Bytes())
}
