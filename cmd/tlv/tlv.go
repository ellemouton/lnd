package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"strconv"

	"github.com/lightningnetwork/lnd/tlv"
)

var byteOrder = binary.BigEndian

const tlvTagName = "tlv"

type TLVField interface {
	Encoder(w io.Writer, val interface{}, buf *[8]byte) error
	Decoder(r io.Reader, val interface{}, buf *[8]byte, l uint64) error
	HasStaticSize() bool
	Size() tlv.SizeFunc
}

func CreateRecord(v TLVField, num tlv.Type) (tlv.Record, error) {
	if v.HasStaticSize() {
		return tlv.MakeStaticRecord(
			num, v, v.Size()(), v.Encoder, v.Decoder,
		), nil
	}

	return tlv.MakeDynamicRecord(num, v, v.Size(), v.Encoder, v.Decoder),
		nil
}

func Decode(r io.Reader, msg interface{}) {

}

func Encode(v interface{}) (*tlv.Stream, error) {
	// Iterate over each item and collect the TLVs.
	rv := reflect.ValueOf(v)
	t := rv.Type()

	tlvTagNumbers := make(map[tlv.Type]bool)
	var records []tlv.Record
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := rv.Field(i)

		// Get the tlv tag. We require that a tlv tag be set. It must
		// also not clash with any previous tlv tag.
		tagValue, ok := field.Tag.Lookup(tlvTagName)
		if !ok {
			return nil, fmt.Errorf("no tlv tag found for field %s",
				field.Name)
		}

		// Parse the tag value. We expect it to be a single,
		// non-negative integer.
		tlvNum, err := strconv.Atoi(tagValue)
		if err != nil {
			return nil, err
		}

		if tlvNum < 0 {
			return nil, fmt.Errorf("tlv number cannot be negative")
		}

		if _, ok := tlvTagNumbers[tlv.Type(tlvNum)]; ok {
			return nil, fmt.Errorf("cannot use the same tlv "+
				"type (%d) for multiple members", tlvNum)
		}

		// If the field is a pointer and it is nil, then we skip it.
		if field.Type.Kind() == reflect.Pointer && rv.Field(i).IsNil() {
			continue
		}

		var fieldValDeref reflect.Value
		if value.Kind() == reflect.Ptr {
			fieldValDeref = value.Elem()
		} else {
			fieldValDeref = value
		}

		tlvValue, ok := fieldValDeref.Interface().(TLVField)
		if !ok {
			return nil, fmt.Errorf("field does not implement " +
				"TLVField")
		}

		record, err := CreateRecord(tlvValue, tlv.Type(tlvNum))
		if err != nil {
			return nil, err
		}

		tlvTagNumbers[tlv.Type(tlvNum)] = true
		records = append(records, record)
	}

	return tlv.NewStream(records...)
}
