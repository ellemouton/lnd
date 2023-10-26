package lnwire

import (
	"bytes"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/fn"
	"github.com/lightningnetwork/lnd/tlv"
)

const (
	// TimeStampRangeFirstBlockType is the TLV type number of the
	// GossipTimestampRange FirstTimestamp field.
	TimeStampRangeFirstBlockType tlv.Type = 2

	// TimeStampRangeBlockRangeType is the TLV type number of the
	// GossipTimestampRange BlockRange field.
	TimeStampRangeBlockRangeType tlv.Type = 4
)

// GossipTimestampRange is a message that allows the sender to restrict the set
// of future gossip announcements sent by the receiver. Nodes should send this
// if they have the gossip-queries feature bit active. Nodes are able to send
// new GossipTimestampRange messages to replace the prior window.
type GossipTimestampRange struct {
	// ChainHash denotes the chain that the sender wishes to restrict the
	// set of received announcements of.
	ChainHash chainhash.Hash

	// FirstTimestamp is the timestamp of the earliest announcement message
	// that should be sent by the receiver. This is only to be used for
	// querying message of gossip 1.0 which are timestamped using Unix
	// timestamps. FirstBlockHeight and BlockRange should be used to
	// query for announcement messages timestamped using block heights.
	FirstTimestamp uint32

	// TimestampRange is the horizon beyond the FirstTimestamp that any
	// announcement messages should be sent for. The receiving node MUST
	// NOT send any announcements that have a timestamp greater than
	// FirstTimestamp + TimestampRange. This is used together with
	// FirstTimestamp to query for gossip 1.0 messages timestamped with
	// Unix timestamps.
	TimestampRange uint32

	// FirstBlockHeight is the height of earliest announcement message that
	// should be sent by the receiver. This is used only for querying
	// announcement messages that use block heights as a timestamp.
	FirstBlockHeight fn.Option[uint32]

	// BlockRange is the horizon beyond FirstBlockHeight that any
	// announcement messages should be sent for. The receiving node MUST NOT
	// send any announcements that have a timestamp greater than
	// FirstBlockHeight + BlockRange.
	BlockRange fn.Option[uint32]

	// ExtraData is the set of data that was appended to this message to
	// fill out the full maximum transport message size. These fields can
	// be used to specify optional data such as custom TLV fields.
	ExtraData ExtraOpaqueData
}

// NewGossipTimestampRange creates a new empty GossipTimestampRange message.
func NewGossipTimestampRange() *GossipTimestampRange {
	return &GossipTimestampRange{}
}

// A compile time check to ensure GossipTimestampRange implements the
// lnwire.Message interface.
var _ Message = (*GossipTimestampRange)(nil)

// Decode deserializes a serialized GossipTimestampRange message stored in the
// passed io.Reader observing the specified protocol version.
//
// This is part of the lnwire.Message interface.
func (g *GossipTimestampRange) Decode(r io.Reader, _ uint32) error {
	err := ReadElements(r,
		g.ChainHash[:],
		&g.FirstTimestamp,
		&g.TimestampRange,
	)
	if err != nil {
		return err
	}

	var (
		firstBlock uint32
		blockRange uint32
	)
	records := []tlv.Record{
		tlv.MakePrimitiveRecord(
			TimeStampRangeFirstBlockType, &firstBlock,
		),
		tlv.MakePrimitiveRecord(
			TimeStampRangeBlockRangeType, &blockRange,
		),
	}

	var tlvRecords ExtraOpaqueData
	if err := ReadElements(r, &tlvRecords); err != nil {
		return err
	}

	typeMap, err := tlvRecords.ExtractRecords(records...)
	if err != nil {
		return err
	}

	if _, ok := typeMap[TimeStampRangeFirstBlockType]; ok {
		g.FirstBlockHeight = fn.Some(firstBlock)
	}

	if _, ok := typeMap[TimeStampRangeBlockRangeType]; ok {
		g.BlockRange = fn.Some(blockRange)
	}

	if len(tlvRecords) != 0 {
		g.ExtraData = tlvRecords
	}

	return nil
}

// Encode serializes the target GossipTimestampRange into the passed io.Writer
// observing the protocol version specified.
//
// This is part of the lnwire.Message interface.
func (g *GossipTimestampRange) Encode(w *bytes.Buffer, pver uint32) error {
	if err := WriteBytes(w, g.ChainHash[:]); err != nil {
		return err
	}

	if err := WriteUint32(w, g.FirstTimestamp); err != nil {
		return err
	}

	if err := WriteUint32(w, g.TimestampRange); err != nil {
		return err
	}

	var records []tlv.Record

	g.FirstBlockHeight.WhenSome(func(u uint32) {
		records = append(records, tlv.MakePrimitiveRecord(
			TimeStampRangeFirstBlockType, &u,
		))
	})

	g.BlockRange.WhenSome(func(u uint32) {
		records = append(records, tlv.MakePrimitiveRecord(
			TimeStampRangeBlockRangeType, &u,
		))
	})

	err := EncodeMessageExtraDataFromRecords(&g.ExtraData, records...)
	if err != nil {
		return err
	}

	return WriteBytes(w, g.ExtraData)
}

// MsgType returns the integer uniquely identifying this message type on the
// wire.
//
// This is part of the lnwire.Message interface.
func (g *GossipTimestampRange) MsgType() MessageType {
	return MsgGossipTimestampRange
}
