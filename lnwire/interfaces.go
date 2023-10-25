package lnwire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/input"
)

// ChannelAnnouncement is an interface that must be satisfied by any message
// used to announce and prove the existence of a channel.
type ChannelAnnouncement interface {
	// SCID returns the short channel ID of the channel.
	SCID() ShortChannelID

	// GetChainHash returns the hash of the chain which this channel's
	// funding transaction is confirmed in.
	GetChainHash() chainhash.Hash

	// Node1KeyBytes returns the bytes representing the public key of node
	// 1 in the channel.
	Node1KeyBytes() [33]byte

	// Node2KeyBytes returns the bytes representing the public key of node
	// 2 in the channel.
	Node2KeyBytes() [33]byte

	Message
}

type ChannelUpdate interface { //nolint:interfacebloat
	Decode(r io.Reader, pver uint32) error
	SCID() ShortChannelID
	IsNode1() bool
	SetDisabled(bool)
	IsDisabled() bool
	HasMaxHTLCSet() bool
	GetChainHash() chainhash.Hash
	SetSig(signature input.Signature) error
	SetSCID(scid ShortChannelID)
	GetTimeLock() uint16
	GetBaseFee() MilliSatoshi
	GetFeeRate() MilliSatoshi
	GetMinHTLC() MilliSatoshi
	GetMaxHTLC() MilliSatoshi
	GetSignature() Sig
	CmpAge(update ChannelUpdate) (int, error)

	Message
}
