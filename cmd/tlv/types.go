package main

import (
	"io"

	"github.com/lightningnetwork/lnd/tlv"
)

type TLVBytes []byte

var _ TLVField = (*TLVBytes)(nil)

func (t TLVBytes) Encoder(w io.Writer, val interface{}, buf *[8]byte) error {
	if b, ok := val.(TLVBytes); ok {
		_, err := w.Write(b)
		return err
	}
	return NewTypeForEncodingErr(val, "TLVBytes")
}

func (t TLVBytes) Decoder(r io.Reader, val interface{}, buf *[8]byte, l uint64) error {
	return tlv.DVarBytes(r, val, buf, l)
}

func (t TLVBytes) HasStaticSize() bool {
	return false
}

func (t TLVBytes) Size() tlv.SizeFunc {
	b := []byte(t)
	return tlv.SizeVarBytes(&b)
}

type TLVInt64 uint64

var _ TLVField = (*TLVInt64)(nil)

func (t TLVInt64) Encoder(w io.Writer, val interface{}, buf *[8]byte) error {
	if i, ok := val.(TLVInt64); ok {
		byteOrder.PutUint64(buf[:], uint64(i))
		_, err := w.Write(buf[:])
		return err
	}
	return NewTypeForEncodingErr(val, "TLVInt64")
}

func (t TLVInt64) Decoder(r io.Reader, val interface{}, buf *[8]byte, l uint64) error {
	return tlv.DUint64(r, val, buf, l)
}

func (t TLVInt64) HasStaticSize() bool {
	return true
}

func (t TLVInt64) Size() tlv.SizeFunc {
	return func() uint64 {
		return 8
	}
}
