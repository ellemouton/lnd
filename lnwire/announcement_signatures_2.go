package lnwire

import (
	"bytes"
	"io"

	"github.com/lightningnetwork/lnd/tlv"
)

// AnnounceSignatures2 is a direct message between two endpoints of a
// channel and serves as an opt-in mechanism to allow the announcement of
// a taproot channel to the rest of the network. It contains the necessary
// signatures by the sender to construct the channel_announcement_2 message.
type AnnounceSignatures2 struct {
	// ChannelID is the unique description of the funding transaction.
	// Channel id is better for users and debugging and short channel id is
	// used for quick test on existence of the particular utxo inside the
	// blockchain, because it contains information about block.
	ChannelID tlv.RecordT[tlv.TlvType0, ChannelID]

	// ShortChannelID is the unique description of the funding transaction.
	// It is constructed with the most significant 3 bytes as the block
	// height, the next 3 bytes indicating the transaction index within the
	// block, and the least significant two bytes indicating the output
	// index which pays to the channel.
	ShortChannelID tlv.RecordT[tlv.TlvType2, ShortChannelID]

	// PartialSignature is the combination of the partial Schnorr signature
	// created for the node's bitcoin key with the partial signature created
	// for the node's node ID key.
	PartialSignature tlv.RecordT[tlv.TlvType4, PartialSig]

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData ExtraOpaqueData
}

func NewAnnSigs2(chanID ChannelID, scid ShortChannelID,
	partialSig PartialSig) *AnnounceSignatures2 {

	return &AnnounceSignatures2{
		ChannelID: tlv.NewRecordT[tlv.TlvType0, ChannelID](chanID),
		ShortChannelID: tlv.NewRecordT[tlv.TlvType2, ShortChannelID](
			scid,
		),
		PartialSignature: tlv.NewRecordT[tlv.TlvType4, PartialSig](
			partialSig,
		),
		ExtraOpaqueData: make([]byte, 0),
	}
}

// A compile time check to ensure AnnounceSignatures2 implements the
// lnwire.Message interface.
var _ Message = (*AnnounceSignatures2)(nil)

// Decode deserializes a serialized AnnounceSignatures2 stored in the passed
// io.Reader observing the specified protocol version.
//
// This is part of the lnwire.Message interface.
func (a *AnnounceSignatures2) Decode(r io.Reader, _ uint32) error {
	// First extract into extra opaque data.
	var tlvRecords ExtraOpaqueData
	if err := ReadElements(r, &tlvRecords); err != nil {
		return err
	}

	_, err := tlvRecords.ExtractRecords(
		&a.ChannelID, &a.ShortChannelID, &a.PartialSignature,
	)

	if len(tlvRecords) != 0 {
		a.ExtraOpaqueData = tlvRecords
	}

	return err
}

// Encode serializes the target AnnounceSignatures2 into the passed io.Writer
// observing the protocol version specified.
//
// This is part of the lnwire.Message interface.
func (a *AnnounceSignatures2) Encode(w *bytes.Buffer, _ uint32) error {
	err := EncodeMessageExtraData(
		&a.ExtraOpaqueData, &a.ChannelID, &a.ShortChannelID,
		&a.PartialSignature,
	)
	if err != nil {
		return err
	}

	return WriteBytes(w, a.ExtraOpaqueData)
}

// MsgType returns the integer uniquely identifying this message type on the
// wire.
//
// This is part of the lnwire.Message interface.
func (a *AnnounceSignatures2) MsgType() MessageType {
	return MsgAnnounceSignatures2
}

// SCID returns the ShortChannelID of the channel.
//
// NOTE: this is part of the AnnounceSignatures interface.
func (a *AnnounceSignatures2) SCID() ShortChannelID {
	return a.ShortChannelID.Val
}

// ChanID returns the ChannelID identifying the channel.
//
// NOTE: this is part of the AnnounceSignatures interface.
func (a *AnnounceSignatures2) ChanID() ChannelID {
	return a.ChannelID.Val
}
