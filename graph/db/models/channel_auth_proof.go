package models

import (
	"fmt"

	"github.com/lightningnetwork/lnd/lnwire"
)

// ChannelAuthProof is the authentication proof (the signature portion) for a
// channel. Using the four signatures contained in the struct, and some
// auxiliary knowledge (the funding script, node identities, and outpoint) nodes
// on the network are able to validate the authenticity and existence of a
// channel. Each of these signatures signs the following digest: chanID ||
// nodeID1 || nodeID2 || bitcoinKey1|| bitcoinKey2 || 2-byte-feature-len ||
// features.
type ChannelAuthProof struct {
	// NodeSig1Bytes are the raw bytes of the first node signature encoded
	// in DER format.
	NodeSig1Bytes []byte

	// NodeSig2Bytes are the raw bytes of the second node signature
	// encoded in DER format.
	NodeSig2Bytes []byte

	// BitcoinSig1Bytes are the raw bytes of the first bitcoin signature
	// encoded in DER format.
	BitcoinSig1Bytes []byte

	// BitcoinSig2Bytes are the raw bytes of the second bitcoin signature
	// encoded in DER format.
	BitcoinSig2Bytes []byte

	Signature []byte
}

func ProofFromWireMsg(ann lnwire.ChannelAnnouncement) (*ChannelAuthProof,
	error) {

	switch msg := ann.(type) {
	case *lnwire.ChannelAnnouncement1:
		return &ChannelAuthProof{
			NodeSig1Bytes:    msg.NodeSig1.ToSignatureBytes()[:],
			NodeSig2Bytes:    msg.NodeSig2.ToSignatureBytes()[:],
			BitcoinSig1Bytes: msg.BitcoinSig1.ToSignatureBytes()[:],
			BitcoinSig2Bytes: msg.BitcoinSig2.ToSignatureBytes()[:],
		}, nil
	case *lnwire.ChannelAnnouncement2:
		return &ChannelAuthProof{
			Signature: msg.Signature.Val.ToSignatureBytes(),
		}, nil
	default:
		return nil, fmt.Errorf("unknown lnwire.ChannelAnnouncement type: %T",
			ann)
	}
}

// IsEmpty check is the authentication proof is empty Proof is empty if at
// least one of the signatures are equal to nil.
//
// TODO(elle): update for v2 sig or remove since only used in kv store.
func (c *ChannelAuthProof) IsEmpty() bool {
	return len(c.NodeSig1Bytes) == 0 ||
		len(c.NodeSig2Bytes) == 0 ||
		len(c.BitcoinSig1Bytes) == 0 ||
		len(c.BitcoinSig2Bytes) == 0
}
