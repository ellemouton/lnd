package lnwire

import (
	"io"

	"github.com/lightningnetwork/lnd/tlv"
)

const (
	QueryOptionsRecordType tlv.Type = 1

	QueryOptionWithTimeStamp = 0
)

type QueryOptions RawFeatureVector

func (c QueryOptions) featureBitLen() uint64 {
	fv := RawFeatureVector(c)
	return uint64(fv.SerializeSize())
}

func (c *QueryOptions) Record() tlv.Record {
	return tlv.MakeDynamicRecord(
		QueryOptionsRecordType, c, c.featureBitLen, queryOptionsEncoder,
		queryOptionsDecoder,
	)
}

func queryOptionsEncoder(w io.Writer, val interface{}, _ *[8]byte) error {
	if v, ok := val.(*QueryOptions); ok {
		// Encode the feature bits as a byte slice without its length
		// prepended, as that's already taken care of by the TLV record.
		fv := RawFeatureVector(*v)
		return fv.encode(w, fv.SerializeSize(), 8)
	}

	return tlv.NewTypeForEncodingErr(val, "lnwire.QueryOptions")
}

func queryOptionsDecoder(r io.Reader, val interface{}, _ *[8]byte,
	l uint64) error {

	if v, ok := val.(*QueryOptions); ok {
		fv := NewRawFeatureVector()
		if err := fv.decode(r, int(l), 8); err != nil {
			return err
		}
		*v = QueryOptions(*fv)
		return nil
	}

	return tlv.NewTypeForEncodingErr(val, "lnwire.QueryOptions")
}
