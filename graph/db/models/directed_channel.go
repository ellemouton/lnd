package models

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

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
