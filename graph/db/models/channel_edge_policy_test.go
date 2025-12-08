package models

import (
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

// TestNewV1Policy tests that the NewV1Policy constructor correctly sets all
// V1 fields.
func TestNewV1Policy(t *testing.T) {
	t.Parallel()

	// Setup test data.
	channelID := uint64(12345)
	toNode := route.Vertex{1, 2, 3}
	sigBytes := []byte{4, 5, 6}
	timeLockDelta := uint16(144)
	minHTLC := lnwire.MilliSatoshi(1000)
	maxHTLC := lnwire.MilliSatoshi(500000000)
	feeBase := lnwire.MilliSatoshi(1000)
	feePpm := lnwire.MilliSatoshi(100)
	inboundFee := fn.Some(lnwire.Fee{
		BaseFee: 500,
		FeeRate: 50,
	})
	lastUpdate := time.Unix(1234567890, 0)
	messageFlags := lnwire.ChanUpdateMsgFlags(1)
	channelFlags := lnwire.ChanUpdateChanFlags(0)
	extraOpaqueData := lnwire.ExtraOpaqueData{7, 8, 9}

	v1Fields := &PolicyV1Fields{
		LastUpdate:      lastUpdate,
		MessageFlags:    messageFlags,
		ChannelFlags:    channelFlags,
		ExtraOpaqueData: extraOpaqueData,
	}

	// Create the policy using the constructor.
	policy := NewV1Policy(
		channelID, toNode, sigBytes, timeLockDelta, minHTLC, maxHTLC,
		feeBase, feePpm, inboundFee, v1Fields,
	)

	// Verify all fields are set correctly.
	require.Equal(t, lnwire.GossipVersion1, policy.Version)
	require.Equal(t, sigBytes, policy.SigBytes)
	require.Equal(t, channelID, policy.ChannelID)
	require.Equal(t, toNode, policy.ToNode)
	require.Equal(t, lastUpdate, policy.LastUpdate)
	require.Equal(t, messageFlags, policy.MessageFlags)
	require.Equal(t, channelFlags, policy.ChannelFlags)
	require.Equal(t, timeLockDelta, policy.TimeLockDelta)
	require.Equal(t, minHTLC, policy.MinHTLC)
	require.Equal(t, maxHTLC, policy.MaxHTLC)
	require.Equal(t, feeBase, policy.FeeBaseMSat)
	require.Equal(t, feePpm, policy.FeeProportionalMillionths)
	require.Equal(t, inboundFee, policy.InboundFee)
	require.Equal(t, extraOpaqueData, policy.ExtraOpaqueData)
}

// TestNewV1PolicyWithoutInboundFee tests that the NewV1Policy constructor
// correctly handles the case where inbound fee is not set.
func TestNewV1PolicyWithoutInboundFee(t *testing.T) {
	t.Parallel()

	// Setup test data without inbound fee.
	channelID := uint64(12345)
	toNode := route.Vertex{1, 2, 3}
	sigBytes := []byte{4, 5, 6}
	timeLockDelta := uint16(144)
	minHTLC := lnwire.MilliSatoshi(1000)
	maxHTLC := lnwire.MilliSatoshi(500000000)
	feeBase := lnwire.MilliSatoshi(1000)
	feePpm := lnwire.MilliSatoshi(100)
	inboundFee := fn.None[lnwire.Fee]()

	v1Fields := &PolicyV1Fields{
		LastUpdate:      time.Unix(1234567890, 0),
		MessageFlags:    lnwire.ChanUpdateMsgFlags(1),
		ChannelFlags:    lnwire.ChanUpdateChanFlags(0),
		ExtraOpaqueData: lnwire.ExtraOpaqueData{7, 8, 9},
	}

	// Create the policy using the constructor.
	policy := NewV1Policy(
		channelID, toNode, sigBytes, timeLockDelta, minHTLC, maxHTLC,
		feeBase, feePpm, inboundFee, v1Fields,
	)

	// Verify that InboundFee is None.
	require.True(t, policy.InboundFee.IsNone())
}

// TestNewV1PolicyIsNode1 tests that the IsNode1 method works correctly for
// V1 policies.
func TestNewV1PolicyIsNode1(t *testing.T) {
	t.Parallel()

	channelID := uint64(12345)
	toNode := route.Vertex{1, 2, 3}
	sigBytes := []byte{4, 5, 6}
	timeLockDelta := uint16(144)
	minHTLC := lnwire.MilliSatoshi(1000)
	maxHTLC := lnwire.MilliSatoshi(500000000)
	feeBase := lnwire.MilliSatoshi(1000)
	feePpm := lnwire.MilliSatoshi(100)
	inboundFee := fn.None[lnwire.Fee]()

	// Test with ChannelFlags indicating node 1 (direction bit = 0).
	v1FieldsNode1 := &PolicyV1Fields{
		LastUpdate:      time.Unix(1234567890, 0),
		MessageFlags:    lnwire.ChanUpdateMsgFlags(1),
		ChannelFlags:    lnwire.ChanUpdateChanFlags(0),
		ExtraOpaqueData: lnwire.ExtraOpaqueData{},
	}

	policyNode1 := NewV1Policy(
		channelID, toNode, sigBytes, timeLockDelta, minHTLC, maxHTLC,
		feeBase, feePpm, inboundFee, v1FieldsNode1,
	)

	require.True(t, policyNode1.IsNode1())

	// Test with ChannelFlags indicating node 2 (direction bit = 1).
	v1FieldsNode2 := &PolicyV1Fields{
		LastUpdate:      time.Unix(1234567890, 0),
		MessageFlags:    lnwire.ChanUpdateMsgFlags(1),
		ChannelFlags:    lnwire.ChanUpdateDirection,
		ExtraOpaqueData: lnwire.ExtraOpaqueData{},
	}

	policyNode2 := NewV1Policy(
		channelID, toNode, sigBytes, timeLockDelta, minHTLC, maxHTLC,
		feeBase, feePpm, inboundFee, v1FieldsNode2,
	)

	require.False(t, policyNode2.IsNode1())
}

// TestNewV1PolicyIsDisabled tests that the IsDisabled method works correctly
// for V1 policies.
func TestNewV1PolicyIsDisabled(t *testing.T) {
	t.Parallel()

	channelID := uint64(12345)
	toNode := route.Vertex{1, 2, 3}
	sigBytes := []byte{4, 5, 6}
	timeLockDelta := uint16(144)
	minHTLC := lnwire.MilliSatoshi(1000)
	maxHTLC := lnwire.MilliSatoshi(500000000)
	feeBase := lnwire.MilliSatoshi(1000)
	feePpm := lnwire.MilliSatoshi(100)
	inboundFee := fn.None[lnwire.Fee]()

	// Test with ChannelFlags indicating enabled (disabled bit = 0).
	v1FieldsEnabled := &PolicyV1Fields{
		LastUpdate:      time.Unix(1234567890, 0),
		MessageFlags:    lnwire.ChanUpdateMsgFlags(1),
		ChannelFlags:    lnwire.ChanUpdateChanFlags(0),
		ExtraOpaqueData: lnwire.ExtraOpaqueData{},
	}

	policyEnabled := NewV1Policy(
		channelID, toNode, sigBytes, timeLockDelta, minHTLC, maxHTLC,
		feeBase, feePpm, inboundFee, v1FieldsEnabled,
	)

	require.False(t, policyEnabled.IsDisabled())

	// Test with ChannelFlags indicating disabled (disabled bit = 1).
	v1FieldsDisabled := &PolicyV1Fields{
		LastUpdate:      time.Unix(1234567890, 0),
		MessageFlags:    lnwire.ChanUpdateMsgFlags(1),
		ChannelFlags:    lnwire.ChanUpdateDisabled,
		ExtraOpaqueData: lnwire.ExtraOpaqueData{},
	}

	policyDisabled := NewV1Policy(
		channelID, toNode, sigBytes, timeLockDelta, minHTLC, maxHTLC,
		feeBase, feePpm, inboundFee, v1FieldsDisabled,
	)

	require.True(t, policyDisabled.IsDisabled())
}
