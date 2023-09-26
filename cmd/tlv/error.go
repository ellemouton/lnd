package main

import "fmt"

// ErrTypeForEncoding signals that an incorrect type was passed to an Encoder.
type ErrTypeForEncoding struct {
	val     interface{}
	expType string
}

// NewTypeForEncodingErr creates a new ErrTypeForEncoding given the incorrect
// val and the expected type.
func NewTypeForEncodingErr(val interface{}, expType string) ErrTypeForEncoding {
	return ErrTypeForEncoding{
		val:     val,
		expType: expType,
	}
}

// Error returns a human-readable description of the type mismatch.
func (e ErrTypeForEncoding) Error() string {
	return fmt.Sprintf("ErrTypeForEncoding want (type: *%s), "+
		"got (type: %T)", e.expType, e.val)
}

// ErrTypeForDecoding signals that an incorrect type was passed to a Decoder or
// that the expected length of the encoding is different from that required by
// the expected type.
type ErrTypeForDecoding struct {
	val       interface{}
	expType   string
	valLength uint64
	expLength uint64
}

// NewTypeForDecodingErr creates a new ErrTypeForDecoding given the incorrect
// val and expected type, or the mismatch in their expected lengths.
func NewTypeForDecodingErr(val interface{}, expType string,
	valLength, expLength uint64) ErrTypeForDecoding {

	return ErrTypeForDecoding{
		val:       val,
		expType:   expType,
		valLength: valLength,
		expLength: expLength,
	}
}

// Error returns a human-readable description of the type mismatch.
func (e ErrTypeForDecoding) Error() string {
	return fmt.Sprintf("ErrTypeForDecoding want (type: *%s, length: %v), "+
		"got (type: %T, length: %v)", e.expType, e.expLength, e.val,
		e.valLength)
}
