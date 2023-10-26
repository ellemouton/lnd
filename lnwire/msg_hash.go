package lnwire

import (
	"bytes"
	"crypto/sha256"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// MsgHashTag is will prefix the message name and the field name in order to
// construct the message tag.
const MsgHashTag = "lightning"

// MsgHash computes the tagged hash of the given message as follows:
//
//	tag = "lightning"||"msg_name"||"field_name"
//	hash = sha256(sha246(tag) || sha256(tag) || msg)
func MsgHash(msgName, fieldName string, msg []byte) *chainhash.Hash {
	tag := []byte(MsgHashTag)
	tag = append(tag, []byte(msgName)...)
	tag = append(tag, []byte(fieldName)...)

	return chainhash.TaggedHash(tag, msg)
}

// MsgHashPreFinalHash performs the same function as MsgHash expect that the
// final sha256 is not performed. It thus computes the following digest:
//
//	tag = "lightning"||"msg_name"||"field_name"
//	digest = sha246(tag) || sha256(tag) || msg
//
// NOTE: this is a work-around required so that the MessageSingerRing's
// SignMessageSchnorr method can be used as is by setting "doubleHash" to false.
// Once a new "tagged-hash" option is provided, this can be removed.
func MsgHashPreFinalHash(msgName, fieldName string, msg []byte) []byte {
	tag := []byte(MsgHashTag)
	tag = append(tag, []byte(msgName)...)
	tag = append(tag, []byte(fieldName)...)

	shaTag := sha256.Sum256(tag)

	var b bytes.Buffer

	b.Write(shaTag[:])
	b.Write(shaTag[:])
	b.Write(msg)

	return b.Bytes()
}
