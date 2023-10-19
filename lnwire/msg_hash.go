package lnwire

import (
	"bytes"
	"crypto/sha256"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

const MsgHashTag = "lightning"

func MsgHash(msgName, fieldName string, msg []byte) *chainhash.Hash {
	tag := []byte(MsgHashTag)
	tag = append(tag, []byte(msgName)...)
	tag = append(tag, []byte(fieldName)...)

	return chainhash.TaggedHash(tag, msg)
}

func MsgHashPreFinalHash(msgName, fieldName string, msg []byte) []byte {
	tag := []byte(MsgHashTag)
	shaTag := sha256.Sum256(tag)

	var b bytes.Buffer

	b.Write(shaTag[:])
	b.Write(shaTag[:])
	b.Write(msg)

	return b.Bytes()
}
