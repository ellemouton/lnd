package lnwire

import (
	"bytes"
	"io"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

func TestPureTLVMessage(t *testing.T) {
	t.Parallel()

	var (
		_, pkA   = btcec.PrivKeyFromBytes([]byte{1})
		_, pkB   = btcec.PrivKeyFromBytes([]byte{2})
		capacity = MilliSatoshi(100)
	)

	t.Run("Encode and Decode of GossipMsg1", func(t *testing.T) {
		t.Parallel()

		// Test Encode and Decode of GossipMsg1.
		msgOld := newMsgV1(pkA, &capacity)

		buf := bytes.NewBuffer(nil)
		require.NoError(t, msgOld.Encode(buf, 0))

		var msgOld2 MsgV1
		require.NoError(t, msgOld2.Decode(buf, 0))

		require.Equal(t, msgOld, &msgOld2)
	})

	t.Run("Encode and Decode of GossipMsg2", func(t *testing.T) {
		t.Parallel()

		// Test Encode and Decode of GossipMsg2.
		msgNew := newMsgV2(
			pkA, &capacity, pkB, []byte{1, 2, 3, 4}, 90, true,
		)

		buf := bytes.NewBuffer(nil)
		require.NoError(t, msgNew.Encode(buf, 0))

		var msgNew2 MsgV2
		require.NoError(t, msgNew2.Decode(buf, 0))

		require.Equal(t, msgNew, &msgNew2)
	})

	t.Run("Encode Msg2 and decode via Msg1", func(t *testing.T) {
		t.Parallel()

		// Ok cool, now try see if we can parse the new message with the
		// old decoder.
		var (
			buf    = bytes.NewBuffer(nil)
			msgNew = newMsgV2(
				pkA, &capacity, pkB, []byte{1, 2, 3, 4}, 90,
				true,
			)
		)
		require.NoError(t, msgNew.Encode(buf, 0))

		// Get the serialised bytes that would be signed for msgNew.
		signData1, err := SerialiseFieldsToSign(msgNew)
		require.NoError(t, err)

		// Decoding via the old message should store some of the extra
		// fields.
		var msgOld MsgV1
		require.NoError(t, msgOld.Decode(buf, 0))
		require.NotEmpty(t, msgOld.ExtraFieldsInSignedRange)

		// The serialised bytes to verify the signature against should
		// be the same though.
		signData2, err := SerialiseFieldsToSign(&msgOld)
		require.NoError(t, err)

		require.Equal(t, signData1, signData2)

		// Re-encoding via the old message should keep the extra fields.
		buf = bytes.NewBuffer(nil)
		require.NoError(t, msgOld.Encode(buf, 0))

		var msgOld2 MsgV1
		require.NoError(t, msgOld2.Decode(buf, 0))

		require.Equal(t, &msgOld, &msgOld2)
	})
}

// MsgV1 represents a more minimal, first version of a Lightning Network
// message.
type MsgV1 struct {
	// Two known fields in the signed range.
	NodeKey  tlv.RecordT[tlv.TlvType0, *btcec.PublicKey]
	Capacity tlv.OptionalRecordT[tlv.TlvType1, MilliSatoshi]

	// Signature in the unsigned range.
	Signature tlv.RecordT[tlv.TlvType160, Sig]

	// Any extra fields in the signed range that we do not yet know about,
	// but we need to keep them for signature validation and to produce a
	// valid message.
	ExtraFieldsInSignedRange map[uint64][]byte
}

var _ Message = (*MsgV1)(nil)
var _ PureTLVMessage = (*MsgV1)(nil)

// newMsgV1 is a constructor for MsgV1.
func newMsgV1(nodeKey *btcec.PublicKey, cap *MilliSatoshi) *MsgV1 {
	newMsg := &MsgV1{
		NodeKey: tlv.NewPrimitiveRecord[tlv.TlvType0](
			nodeKey,
		),
		Signature: tlv.NewRecordT[tlv.TlvType160](
			testSchnorrSig,
		),
		ExtraFieldsInSignedRange: make(map[uint64][]byte),
	}

	if cap != nil {
		newMsg.Capacity = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType1](*cap),
		)
	}

	return newMsg
}

// Decode deserializes a serialized MsgV1 in the passed io.Reader.
//
// This is part of the lnwire.Message interface.
func (g *MsgV1) Decode(r io.Reader, _ uint32) error {
	var capacity = tlv.ZeroRecordT[tlv.TlvType1, MilliSatoshi]()
	stream, err := tlv.NewStream(
		ProduceRecordsSorted(
			&g.NodeKey,
			&capacity,
			&g.Signature,
		)...,
	)
	if err != nil {
		return err
	}
	g.Signature.Val.ForceSchnorr()

	typeMap, err := stream.DecodeWithParsedTypesP2P(r)
	if err != nil {
		return err
	}

	if _, ok := typeMap[g.Capacity.TlvType()]; ok {
		g.Capacity = tlv.SomeRecordT(capacity)
	}

	g.ExtraFieldsInSignedRange = ExtraSignedFieldsFromTypeMap(typeMap)

	return nil
}

// Encode serializes the target MsgV1 into the passed buffer.
//
// This is part of the lnwire.Message interface.
func (g *MsgV1) Encode(buf *bytes.Buffer, _ uint32) error {
	return EncodePureTLVMessage(g, buf)
}

// MsgType returns the integer uniquely identifying this message type on the
// wire.
//
// This is part of the lnwire.Message interface.
func (g *MsgV1) MsgType() MessageType {
	return 7777
}

// AllRecords returns all the TLV records for the message. This will
// include all the records we know about along with any that we don't
// know about but that fall in the signed TLV range.
//
// This is part of the PureTLVMessage interface.
func (g *MsgV1) AllRecords() []tlv.Record {
	recordProducers := []tlv.RecordProducer{
		&g.NodeKey,
		&g.Signature,
	}
	recordProducers = append(
		recordProducers,
		RecordsAsProducers(
			tlv.MapToRecords(g.ExtraFieldsInSignedRange),
		)...,
	)

	g.Capacity.WhenSome(func(cap tlv.RecordT[tlv.TlvType1, MilliSatoshi]) {
		recordProducers = append(recordProducers, &cap)
	})

	return ProduceRecordsSorted(recordProducers...)
}

// MsgV2 represents a newer version of MsgV1 which contains more fields both in
// the unsigned and signed TLV ranges.
type MsgV2 struct {
	NodeKey  tlv.RecordT[tlv.TlvType0, *btcec.PublicKey]
	Capacity tlv.OptionalRecordT[tlv.TlvType1, MilliSatoshi]

	// An additional fields (optional) in the signed range.
	BitcoinKey tlv.OptionalRecordT[tlv.TlvType3, *btcec.PublicKey]

	// A zero length TLV in the signed range.
	SecondPeer tlv.OptionalRecordT[tlv.TlvType5, TrueBoolean]

	// Signature in the unsigned range.
	Signature tlv.RecordT[tlv.TlvType160, Sig]

	// Another field in the unsigned range. An older node can throw this
	// away.
	SPVProof tlv.RecordT[tlv.TlvType161, []byte]

	// A new field in the second signed range. An older node should keep
	// this since it is part of the serialised message that is signed.
	Num tlv.RecordT[tlv.TlvType1000000000, uint8]

	// Any extra fields in the signed range that we do not yet know about,
	// but we need to keep them for signature validation and to produce a
	// valid message.
	ExtraFieldsInSignedRange map[uint64][]byte
}

// newMsgV2 is a constructor for MsgV2.
func newMsgV2(nodeKey *btcec.PublicKey, cap *MilliSatoshi,
	btcKey *btcec.PublicKey, spvProof []byte, num uint8,
	secondPeer bool) *MsgV2 {

	newMsg := &MsgV2{
		NodeKey:  tlv.NewPrimitiveRecord[tlv.TlvType0](nodeKey),
		SPVProof: tlv.NewPrimitiveRecord[tlv.TlvType161](spvProof),
		Num:      tlv.NewPrimitiveRecord[tlv.TlvType1000000000](num),
		Signature: tlv.NewRecordT[tlv.TlvType160](
			testSchnorrSig,
		),
		ExtraFieldsInSignedRange: make(map[uint64][]byte),
	}

	if secondPeer {
		newMsg.SecondPeer = tlv.SomeRecordT(
			tlv.NewRecordT[tlv.TlvType5](TrueBoolean{}),
		)
	}

	if cap != nil {
		newMsg.Capacity = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType1](*cap),
		)
	}

	if btcKey != nil {
		newMsg.BitcoinKey = tlv.SomeRecordT(
			tlv.NewPrimitiveRecord[tlv.TlvType3](btcKey),
		)
	}

	return newMsg
}

// Decode deserializes a serialized MsgV2 in the passed io.Reader.
//
// This is part of the lnwire.Message interface.
func (g *MsgV2) Decode(r io.Reader, _ uint32) error {
	var (
		capacity   = tlv.ZeroRecordT[tlv.TlvType1, MilliSatoshi]()
		btcKey     = tlv.ZeroRecordT[tlv.TlvType3, *btcec.PublicKey]()
		secondPeer = tlv.ZeroRecordT[tlv.TlvType5, TrueBoolean]()
	)

	stream, err := tlv.NewStream(
		ProduceRecordsSorted(
			&g.NodeKey,
			&capacity,
			&btcKey,
			&secondPeer,
			&g.Signature,
			&g.SPVProof,
			&g.Num,
		)...,
	)
	if err != nil {
		return err
	}
	g.Signature.Val.ForceSchnorr()

	typeMap, err := stream.DecodeWithParsedTypesP2P(r)
	if err != nil {
		return err
	}

	if _, ok := typeMap[g.Capacity.TlvType()]; ok {
		g.Capacity = tlv.SomeRecordT(capacity)
	}

	if _, ok := typeMap[g.SecondPeer.TlvType()]; ok {
		g.SecondPeer = tlv.SomeRecordT(secondPeer)
	}

	if _, ok := typeMap[g.BitcoinKey.TlvType()]; ok {
		g.BitcoinKey = tlv.SomeRecordT(btcKey)
	}

	g.ExtraFieldsInSignedRange = ExtraSignedFieldsFromTypeMap(typeMap)

	return nil
}

// Encode serializes the target MsgV2 into the passed buffer.
//
// This is part of the lnwire.Message interface.
func (g *MsgV2) Encode(buf *bytes.Buffer, _ uint32) error {
	return EncodePureTLVMessage(g, buf)
}

// MsgType returns the integer uniquely identifying this message type on the
// wire.
//
// This is part of the lnwire.Message interface.
func (g *MsgV2) MsgType() MessageType {
	return 7779
}

// AllRecords returns all the TLV records for the message. This will
// include all the records we know about along with any that we don't
// know about but that fall in the signed TLV range.
//
// This is part of the PureTLVMessage interface.
func (g *MsgV2) AllRecords() []tlv.Record {
	recordProducers := []tlv.RecordProducer{
		&g.NodeKey,
		&g.Signature,
		&g.SPVProof,
		&g.Num,
	}
	recordProducers = append(recordProducers, RecordsAsProducers(
		tlv.MapToRecords(g.ExtraFieldsInSignedRange),
	)...)

	g.Capacity.WhenSome(func(cap tlv.RecordT[tlv.TlvType1, MilliSatoshi]) {
		recordProducers = append(recordProducers, &cap)
	})
	g.BitcoinKey.WhenSome(func(key tlv.RecordT[tlv.TlvType3, *btcec.PublicKey]) {
		recordProducers = append(recordProducers, &key)
	})
	g.SecondPeer.WhenSome(func(second tlv.RecordT[tlv.TlvType5, TrueBoolean]) {
		recordProducers = append(recordProducers, &second)
	})

	return ProduceRecordsSorted(recordProducers...)
}
