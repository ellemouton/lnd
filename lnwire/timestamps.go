package lnwire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/lightningnetwork/lnd/tlv"
)

const TimestampsRecordType tlv.Type = 1

type Timestamps []ChanUpdateTimestamps

type ChanUpdateTimestamps struct {
	Timestamp1 uint32
	Timestamp2 uint32
}

func (t *Timestamps) Record() tlv.Record {
	return tlv.MakeDynamicRecord(
		TimestampsRecordType, t, t.encodedLen, timeStampsEncoder,
		timeStampsDecoder,
	)
}

func (t *Timestamps) encodedLen() uint64 {
	return uint64(1 + 8*(len(*t)))
}

func timeStampsEncoder(w io.Writer, val interface{}, buf *[8]byte) error {
	if v, ok := val.(*Timestamps); ok {
		buf := bytes.NewBuffer(nil)

		// Add the encoding byte.
		err := WriteEncoding(buf, EncodingSortedPlain)
		if err != nil {
			return err
		}

		// For each timestamp, write 4 byte node 1 & 4 byte node 2
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

func timeStampsDecoder(r io.Reader, val interface{}, buf *[8]byte,
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

		if (l-1)%8 != 0 {
			return fmt.Errorf("whole number of timestamps not " +
				"encoded")
		}

		numTimestamps := (int(l) - 1) / 8

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
