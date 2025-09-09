package lnwire

import (
	"bytes"
	"io"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/tlv"
)

type OutPoint wire.OutPoint

func (o *OutPoint) Record() tlv.Record {
	return tlv.MakeStaticRecord(0, o, 36, outpointEncoder, outpointDecoder)
}

func outpointEncoder(w io.Writer, val interface{}, _ *[8]byte) error {
	if v, ok := val.(*OutPoint); ok {
		buf := bytes.NewBuffer(nil)
		err := WriteOutPoint(buf, wire.OutPoint(*v))
		if err != nil {
			return err
		}
		_, err = w.Write(buf.Bytes())

		return err
	}

	return tlv.NewTypeForEncodingErr(val, "OutPoint")
}

func outpointDecoder(r io.Reader, val interface{}, _ *[8]byte, l uint64) error {
	if v, ok := val.(*OutPoint); ok {
		var o wire.OutPoint
		if err := ReadElement(r, &o); err != nil {
			return err
		}

		*v = OutPoint(o)

		return nil
	}

	return tlv.NewTypeForDecodingErr(val, "OutPoint", l, 36)
}
