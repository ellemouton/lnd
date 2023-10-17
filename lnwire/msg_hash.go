package lnwire

import "github.com/btcsuite/btcd/chaincfg/chainhash"

const MsgHashTag = "lightning"

func MsgHash(msgName, fieldName string, msg []byte) *chainhash.Hash {
	tag := []byte(MsgHashTag)
	tag = append(tag, []byte(msgName)...)
	tag = append(tag, []byte(fieldName)...)

	return chainhash.TaggedHash(tag, msg)
}
