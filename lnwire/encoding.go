package lnwire

// Encoding is an enum-like type that represents exactly how a set
// of short channel ID's is encoded on the wire. The set of encodings allows us
// to take advantage of the structure of a list of short channel ID's to
// achieving a high degree of compression.
type Encoding uint8

const (
	// EncodingSortedPlain signals that the set of short channel ID's is
	// encoded using the regular encoding, in a sorted order.
	EncodingSortedPlain Encoding = 0

	// EncodingSortedZlib signals that the set of short channel ID's is
	// encoded by first sorting the set of channel ID's, as then
	// compressing them using zlib.
	EncodingSortedZlib Encoding = 1
)
