package lnwire

import "github.com/btcsuite/btcd/chaincfg/chainhash"

type ChannelAnnouncement interface {
	SCID() ShortChannelID
	GetChainHash() chainhash.Hash
	Node1KeyBytes() [33]byte
	Node2KeyBytes() [33]byte

	Message
}
