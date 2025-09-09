package models

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// ChannelEdgeInfo represents a fully authenticated channel along with all its
// unique attributes. Once an authenticated channel announcement has been
// processed on the network, then an instance of ChannelEdgeInfo encapsulating
// the channels attributes is stored. The other portions relevant to routing
// policy of a channel are stored within a ChannelEdgePolicy for each direction
// of the channel.
type ChannelEdgeInfo struct {
	Version lnwire.GossipVersion

	// ChannelID is the unique channel ID for the channel. The first 3
	// bytes are the block height, the next 3 the index within the block,
	// and the last 2 bytes are the output index for the channel.
	ChannelID uint64

	// ChainHash is the hash that uniquely identifies the chain that this
	// channel was opened within.
	ChainHash chainhash.Hash

	// NodeKey1Bytes is the raw public key of the first node.
	NodeKey1Bytes route.Vertex
	nodeKey1      *btcec.PublicKey

	// NodeKey2Bytes is the raw public key of the first node.
	NodeKey2Bytes route.Vertex
	nodeKey2      *btcec.PublicKey

	// BitcoinKey1Bytes is the raw public key of the first node.
	BitcoinKey1Bytes route.Vertex

	// BitcoinKey2Bytes is the raw public key of the first node.
	BitcoinKey2Bytes route.Vertex

	// Features is the list of protocol features supported by this channel
	// edge.
	Features *lnwire.FeatureVector

	// AuthProof is the authentication proof for this channel. This proof
	// contains a set of signatures binding four identities, which attests
	// to the legitimacy of the advertised channel.
	AuthProof *ChannelAuthProof

	// ChannelPoint is the funding outpoint of the channel. This can be
	// used to uniquely identify the channel within the channel graph.
	ChannelPoint wire.OutPoint

	// Capacity is the total capacity of the channel, this is determined by
	// the value output in the outpoint that created this channel.
	Capacity btcutil.Amount

	// FundingScript holds the script of the channel's funding transaction.
	//
	// NOTE: this is not currently persisted and so will not be present if
	// the edge object is loaded from the database.
	FundingScript fn.Option[[]byte]

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData []byte

	ExtraSignedFields map[uint64][]byte
}

type EdgeModifier func(*ChannelEdgeInfo)

func WithChannelPoint(cp wire.OutPoint) EdgeModifier {
	return func(e *ChannelEdgeInfo) {
		e.ChannelPoint = cp
	}
}

func WithCapacity(cap btcutil.Amount) EdgeModifier {
	return func(e *ChannelEdgeInfo) {
		e.Capacity = cap
	}
}

// NodeKey1 is the identity public key of the "first" node that was involved in
// the creation of this channel. A node is considered "first" if the
// lexicographical ordering the its serialized public key is "smaller" than
// that of the other node involved in channel creation.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) NodeKey1() (*btcec.PublicKey, error) {
	if c.nodeKey1 != nil {
		return c.nodeKey1, nil
	}

	key, err := btcec.ParsePubKey(c.NodeKey1Bytes[:])
	if err != nil {
		return nil, err
	}
	c.nodeKey1 = key

	return key, nil
}

// NodeKey2 is the identity public key of the "second" node that was involved in
// the creation of this channel. A node is considered "second" if the
// lexicographical ordering the its serialized public key is "larger" than that
// of the other node involved in channel creation.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) NodeKey2() (*btcec.PublicKey, error) {
	if c.nodeKey2 != nil {
		return c.nodeKey2, nil
	}

	key, err := btcec.ParsePubKey(c.NodeKey2Bytes[:])
	if err != nil {
		return nil, err
	}
	c.nodeKey2 = key

	return key, nil
}

// OtherNodeKeyBytes returns the node key bytes of the other end of the channel.
func (c *ChannelEdgeInfo) OtherNodeKeyBytes(thisNodeKey []byte) (
	[33]byte, error) {

	switch {
	case bytes.Equal(c.NodeKey1Bytes[:], thisNodeKey):
		return c.NodeKey2Bytes, nil
	case bytes.Equal(c.NodeKey2Bytes[:], thisNodeKey):
		return c.NodeKey1Bytes, nil
	default:
		return [33]byte{}, fmt.Errorf("node not participating in " +
			"this channel")
	}
}

func EdgeFromWireAnnouncement(ann *lnwire.ChannelAnnouncement1,
	proof *ChannelAuthProof, opts ...EdgeModifier) *ChannelEdgeInfo {

	edge := &ChannelEdgeInfo{
		Version:          lnwire.GossipVersion1,
		ChannelID:        ann.ShortChannelID.ToUint64(),
		ChainHash:        ann.ChainHash,
		NodeKey1Bytes:    ann.NodeID1,
		NodeKey2Bytes:    ann.NodeID2,
		BitcoinKey1Bytes: ann.BitcoinKey1,
		BitcoinKey2Bytes: ann.BitcoinKey2,
		Features: lnwire.NewFeatureVector(
			ann.Features, lnwire.Features,
		),
		AuthProof:       proof,
		ExtraOpaqueData: ann.ExtraOpaqueData,
	}

	for _, opt := range opts {
		opt(edge)
	}

	return edge
}

// DirectedChannel is a type that stores the channel information as seen from
// one side of the channel.
type DirectedChannel struct {
	// ChannelID is the unique identifier of this channel.
	ChannelID uint64

	// IsNode1 indicates if this is the node with the smaller public key.
	IsNode1 bool

	// OtherNode is the public key of the node on the other end of this
	// channel.
	OtherNode route.Vertex

	// Capacity is the announced capacity of this channel in satoshis.
	Capacity btcutil.Amount

	// OutPolicySet is a boolean that indicates whether the node has an
	// outgoing policy set. For pathfinding only the existence of the policy
	// is important to know, not the actual content.
	OutPolicySet bool

	// InPolicy is the incoming policy *from* the other node to this node.
	// In path finding, we're walking backward from the destination to the
	// source, so we're always interested in the edge that arrives to us
	// from the other node.
	InPolicy *CachedEdgePolicy

	// Inbound fees of this node.
	InboundFee lnwire.Fee
}

// DeepCopy creates a deep copy of the channel, including the incoming policy.
func (c *DirectedChannel) DeepCopy() *DirectedChannel {
	channelCopy := *c

	if channelCopy.InPolicy != nil {
		inPolicyCopy := *channelCopy.InPolicy
		channelCopy.InPolicy = &inPolicyCopy

		// The fields for the ToNode can be overwritten by the path
		// finding algorithm, which is why we need a deep copy in the
		// first place. So we always start out with nil values, just to
		// be sure they don't contain any old data.
		channelCopy.InPolicy.ToNodePubKey = nil
		channelCopy.InPolicy.ToNodeFeatures = nil
	}

	return &channelCopy
}
