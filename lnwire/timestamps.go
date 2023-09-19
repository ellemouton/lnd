package lnwire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// TimestampsRecordType is the TLV number of the timestamps TLV record
	// in the reply_channel_range message.
	TimestampsRecordType tlv.Type = 1

	// timestampPairSize is the number of bytes required to encode two
	// timestamps. Each timestamp is four bytes.
	timestampPairSize = 8
)

// Timestamps is a type representing the timestamps TLV field used in the
// reply_channel_range message to communicate the timestamps info of the updates
// of the SCID list being communicated.
type Timestamps []ChanUpdateTimestamps

// ChanUpdateTimestamps holds the timestamp info of the latest known channel
// updates corresponding to the two sides of a channel.
type ChanUpdateTimestamps struct {
	Timestamp1 uint32
	Timestamp2 uint32
}

// Record constructs the tlv.Record from the Timestamps.
func (t *Timestamps) Record() tlv.Record {
	return tlv.MakeDynamicRecord(
		TimestampsRecordType, t, t.encodedLen, timeStampsEncoder,
		timeStampsDecoder,
	)
}

// encodedLen calculates the length of the encoded Timestamps.
func (t *Timestamps) encodedLen() uint64 {
	return uint64(1 + 8*(len(*t)))
}

// timeStampsEncoder encodes the Timestamps and writes the encoded bytes to the
// given writer.
func timeStampsEncoder(w io.Writer, val interface{}, _ *[8]byte) error {
	if v, ok := val.(*Timestamps); ok {
		buf := bytes.NewBuffer(nil)

		// Add the encoding byte.
		err := WriteEncoding(buf, EncodingSortedPlain)
		if err != nil {
			return err
		}

		// For each timestamp, write 4 byte timestamp of node 1 and the
		// 4 byte timestamp of node 2.
		for _, timestamps := range *v {
			err = WriteUint32(buf, timestamps.Timestamp1)
			if err != nil {
				return err
			}

			err = WriteUint32(buf, timestamps.Timestamp2)
			if err != nil {
				return err
			}
		}

		_, err = w.Write(buf.Bytes())

		return err
	}

	return tlv.NewTypeForEncodingErr(val, "lnwire.Timestamps")
}

// timeStampsDecoder attempts to read and reconstruct a Timestamps object from
// the given reader.
func timeStampsDecoder(r io.Reader, val interface{}, _ *[8]byte,
	l uint64) error {

	if v, ok := val.(*Timestamps); ok {
		var encodingByte [1]byte
		if _, err := r.Read(encodingByte[:]); err != nil {
			return err
		}

		encoding := Encoding(encodingByte[0])
		if encoding != EncodingSortedPlain {
			return fmt.Errorf("unsupported encoding: %x", encoding)
		}

		// The number of timestamps bytes is equal to the passed length
		// minus one since the first byte is used for the encoding type.
		numTimestampBytes := l - 1

		if numTimestampBytes%timestampPairSize != 0 {
			return fmt.Errorf("whole number of timestamps not " +
				"encoded")
		}

		numTimestamps := int(numTimestampBytes) / timestampPairSize

		var (
			timestamps      = make(Timestamps, numTimestamps)
			timestampBytes1 [4]byte
			timestampBytes2 [4]byte
		)
		for i := 0; i < numTimestamps; i++ {
			if _, err := r.Read(timestampBytes1[:]); err != nil {
				return err
			}

			if _, err := r.Read(timestampBytes2[:]); err != nil {
				return err
			}

			t1 := binary.BigEndian.Uint32(timestampBytes1[:])
			t2 := binary.BigEndian.Uint32(timestampBytes2[:])

			timestamps[i] = ChanUpdateTimestamps{
				Timestamp1: t1,
				Timestamp2: t2,
			}
		}

		*v = timestamps

		return nil
	}

	return tlv.NewTypeForEncodingErr(val, "lnwire.Timestamps")
}
