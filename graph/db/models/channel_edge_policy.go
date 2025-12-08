package models

import (
	"fmt"
	"time"

	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// ChannelEdgePolicy represents a *directed* edge within the channel graph. For
// each channel in the database, there are two distinct edges: one for each
// possible direction of travel along the channel. The edges themselves hold
// information concerning fees, and minimum time-lock information which is
// utilized during path finding.
type ChannelEdgePolicy struct {
	// Version is the gossip version that this update was advertised on.
	Version lnwire.GossipVersion

	// SigBytes is the raw bytes of the signature of the channel edge
	// policy. We'll only parse these if the caller needs to access the
	// signature for validation purposes. Do not set SigBytes directly, but
	// use SetSigBytes instead to make sure that the cache is invalidated.
	SigBytes []byte

	// ChannelID is the unique channel ID for the channel. The first 3
	// bytes are the block height, the next 3 the index within the block,
	// and the last 2 bytes are the output index for the channel.
	ChannelID uint64

	// LastUpdate is the last time an authenticated edge for this channel
	// was received.
	//
	// NOTE: this is for v1 channel announcements only.
	LastUpdate time.Time

	// LastBlockHeight is the block height that timestamps the last update
	// we received for this node.
	//
	// NOTE: This is only used if this is a V2 node announcement.
	LastBlockHeight uint32

	// MessageFlags is a bitfield which indicates the presence of optional
	// fields (like max_htlc) in the policy.
	MessageFlags lnwire.ChanUpdateMsgFlags

	// ChannelFlags is a bitfield which signals the capabilities of the
	// channel as well as the directed edge this update applies to.
	//
	// NOTE: only used for version 1 channels.
	ChannelFlags lnwire.ChanUpdateChanFlags

	// SecondPeer is true if this was produced by the channel's
	// lexographically second peer.
	//
	// NOTE: only used for version 2 channels.
	SecondPeer bool

	// DisableFlags is a bitfield which signals info about which direction
	// of the channel is disabled.
	//
	// NOTE: This field is only used for version 2 channels.
	DisableFlags lnwire.ChanUpdateDisableFlags

	// TimeLockDelta is the number of blocks this node will subtract from
	// the expiry of an incoming HTLC. This value expresses the time buffer
	// the node would like to HTLC exchanges.
	TimeLockDelta uint16

	// MinHTLC is the smallest value HTLC this node will forward, expressed
	// in millisatoshi.
	MinHTLC lnwire.MilliSatoshi

	// MaxHTLC is the largest value HTLC this node will forward, expressed
	// in millisatoshi.
	MaxHTLC lnwire.MilliSatoshi

	// FeeBaseMSat is the base HTLC fee that will be charged for forwarding
	// ANY HTLC, expressed in mSAT's.
	FeeBaseMSat lnwire.MilliSatoshi

	// FeeProportionalMillionths is the rate that the node will charge for
	// HTLCs for each millionth of a satoshi forwarded.
	FeeProportionalMillionths lnwire.MilliSatoshi

	// ToNode is the public key of the node that this directed edge leads
	// to. Using this pub key, the channel graph can further be traversed.
	ToNode route.Vertex

	// InboundFee is the fee that must be paid for incoming HTLCs.
	//
	// NOTE: for our kvdb implementation of the graph store, inbound fees
	// are still only persisted as part of extra opaque data and so this
	// field is not explicitly stored but is rather populated from the
	// ExtraOpaqueData field on deserialization. For our SQL implementation,
	// this field will be explicitly persisted in the database.
	InboundFee fn.Option[lnwire.Fee]

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	//
	// NOTE: only used for version 1 channels.
	ExtraOpaqueData lnwire.ExtraOpaqueData

	// ExtraSignedFields is a map of extra fields that are covered by the
	// node announcement's signature that we have not explicitly parsed.
	//
	// NOTE: This is only used for version 2 node announcements and beyond.
	ExtraSignedFields map[uint64][]byte
}

// PolicyV1Fields houses V1-specific channel update fields.
type PolicyV1Fields struct {
	// LastUpdate is the last time an authenticated edge for this channel
	// was received.
	LastUpdate time.Time

	// MessageFlags is a bitfield which indicates the presence of optional
	// fields (like max_htlc) in the policy.
	MessageFlags lnwire.ChanUpdateMsgFlags

	// ChannelFlags is a bitfield which signals the capabilities of the
	// channel as well as the directed edge this update applies to.
	ChannelFlags lnwire.ChanUpdateChanFlags

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData lnwire.ExtraOpaqueData
}

// PolicyModifier is a functional option that modifies a ChannelEdgePolicy.
type PolicyModifier func(*ChannelEdgePolicy)

// WithToNode sets the ToNode field on the policy.
func WithToNode(toNode route.Vertex) PolicyModifier {
	return func(p *ChannelEdgePolicy) {
		p.ToNode = toNode
	}
}

// NewV1Policy creates a new version 1 channel edge policy.
func NewV1Policy(channelID uint64, sigBytes []byte, timeLockDelta uint16,
	minHTLC, maxHTLC, feeBase, feePpm lnwire.MilliSatoshi,
	inboundFee fn.Option[lnwire.Fee], v1Fields *PolicyV1Fields,
	opts ...PolicyModifier) *ChannelEdgePolicy {

	policy := &ChannelEdgePolicy{
		Version:                   lnwire.GossipVersion1,
		SigBytes:                  sigBytes,
		ChannelID:                 channelID,
		LastUpdate:                v1Fields.LastUpdate,
		MessageFlags:              v1Fields.MessageFlags,
		ChannelFlags:              v1Fields.ChannelFlags,
		TimeLockDelta:             timeLockDelta,
		MinHTLC:                   minHTLC,
		MaxHTLC:                   maxHTLC,
		FeeBaseMSat:               feeBase,
		FeeProportionalMillionths: feePpm,
		InboundFee:                inboundFee,
		ExtraOpaqueData:           v1Fields.ExtraOpaqueData,
	}

	for _, opt := range opts {
		opt(policy)
	}

	return policy
}

// SetSigBytes updates the signature and invalidates the cached parsed
// signature.
func (c *ChannelEdgePolicy) SetSigBytes(sig []byte) {
	c.SigBytes = sig
}

// IsNode1 determines whether this edge policy is for the channel's
// lexicographically first node.
func (c *ChannelEdgePolicy) IsNode1() bool {
	if c.Version == lnwire.GossipVersion1 {
		return c.ChannelFlags&lnwire.ChanUpdateDirection == 0
	}

	return !c.SecondPeer
}

// IsDisabled determines whether the edge has the disabled bit set.
func (c *ChannelEdgePolicy) IsDisabled() bool {
	if c.Version == lnwire.GossipVersion1 {
		return c.ChannelFlags.IsDisabled()
	}

	return !c.DisableFlags.IsEnabled()
}

// ComputeFee computes the fee to forward an HTLC of `amt` milli-satoshis over
// the passed active payment channel. This value is currently computed as
// specified in BOLT07, but will likely change in the near future.
func (c *ChannelEdgePolicy) ComputeFee(
	amt lnwire.MilliSatoshi) lnwire.MilliSatoshi {

	return c.FeeBaseMSat + (amt*c.FeeProportionalMillionths)/feeRateParts
}

// String returns a human-readable version of the channel edge policy.
func (c *ChannelEdgePolicy) String() string {
	if c.Version == lnwire.GossipVersion1 {
		return fmt.Sprintf("ChannelID=%v, MessageFlags=%v, "+
			"ChannelFlags=%v, LastUpdate=%v", c.ChannelID,
			c.MessageFlags, c.ChannelFlags, c.LastUpdate)
	}

	return fmt.Sprintf("ChannelID=%v, Node1=%v, DisableFlags=%v, "+
		"BlockHeight=%v", c.ChannelID, !c.SecondPeer,
		c.DisableFlags, c.LastBlockHeight)
}

// ChanEdgePolicyFromWire constructs a ChannelEdgePolicy from a wire
// ChannelUpdate message.
func ChanEdgePolicyFromWire(scid uint64,
	update lnwire.ChannelUpdate) (*ChannelEdgePolicy, error) {

	switch upd := update.(type) {
	case *lnwire.ChannelUpdate1:
		return NewV1Policy(
			scid,
			upd.Signature.ToSignatureBytes(),
			upd.TimeLockDelta,
			upd.HtlcMinimumMsat,
			upd.HtlcMaximumMsat,
			lnwire.MilliSatoshi(upd.BaseFee),
			lnwire.MilliSatoshi(upd.FeeRate),
			upd.InboundFee.ValOpt(),
			&PolicyV1Fields{
				LastUpdate:      time.Unix(int64(upd.Timestamp), 0),
				MessageFlags:    upd.MessageFlags,
				ChannelFlags:    upd.ChannelFlags,
				ExtraOpaqueData: upd.ExtraOpaqueData,
			},
			// ToNode - set by caller.
		), nil

	default:
		return nil, fmt.Errorf("unsupported channel update "+
			"version: %T", update)
	}
}
