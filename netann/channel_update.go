package netann

import (
	"bytes"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
)

// ErrUnableToExtractChanUpdate is returned when a channel update cannot be
// found for one of our active channels.
var ErrUnableToExtractChanUpdate = fmt.Errorf("unable to extract ChannelUpdate")

// ChannelUpdateModifier is a closure that makes in-place modifications to an
// lnwire.ChannelUpdate.
type ChannelUpdateModifier func(update lnwire.ChanUpdate)

// ChanUpdSetDisable is a functional option that sets the disabled channel flag
// if disabled is true, and clears the bit otherwise.
func ChanUpdSetDisable(disabled bool) ChannelUpdateModifier {
	return func(update lnwire.ChanUpdate) {
		update.SetDisabled(disabled)
	}
}

// ChanUpdSetTimestamp is a functional option that sets the timestamp of the
// update to the current time, or increments it if the timestamp is already in
// the future.
func ChanUpdSetTimestamp(height uint32) ChannelUpdateModifier {
	return func(update lnwire.ChanUpdate) {
		switch upd := update.(type) {
		case *lnwire.ChannelUpdate:
			newTimestamp := uint32(time.Now().Unix())
			if newTimestamp <= upd.Timestamp {
				// Increment the prior value to ensure the
				// timestamp monotonically increases, otherwise
				// the update won't propagate.
				newTimestamp = upd.Timestamp + 1
			}
			upd.Timestamp = newTimestamp

		case *lnwire.ChannelUpdate2:
			newHeight := height
			if height <= upd.BlockHeight {
				newHeight = upd.BlockHeight + 1
			}
			upd.BlockHeight = newHeight
		}
	}
}

// SignChannelUpdate applies the given modifiers to the passed
// lnwire.ChannelUpdate, then signs the resulting update. The provided update
// should be the most recent, valid update, otherwise the timestamp may not
// monotonically increase from the prior.
//
// NOTE: This method modifies the given update.
func SignChannelUpdate(signer lnwallet.MessageSigner, keyLoc keychain.KeyLocator,
	update lnwire.ChanUpdate, mods ...ChannelUpdateModifier) (
	input.Signature, error) {

	// Apply the requested changes to the channel update.
	for _, modifier := range mods {
		modifier(update)
	}

	var sig input.Signature

	switch upd := update.(type) {
	case *lnwire.ChannelUpdate:
		data, err := upd.DataToSign()
		if err != nil {
			return nil, fmt.Errorf("unable to get data to sign: %v",
				err)
		}

		sig, err := signer.SignMessage(keyLoc, data, true)
		if err != nil {
			return nil, err
		}

		upd.Signature, err = lnwire.NewSigFromSignature(sig)

	case *lnwire.ChannelUpdate2:
		panic("implement signing of chan update 2")
	default:
		return nil, fmt.Errorf("asdffd")
	}

	return sig, nil
}

// ExtractChannelUpdate attempts to retrieve a lnwire.ChannelUpdate message from
// an edge's info and a set of routing policies.
//
// NOTE: The passed policies can be nil.
func ExtractChannelUpdate(ownerPubKey []byte,
	info *channeldb.ChannelEdgeInfo, policies ...channeldb.ChanEdgePolicy) (
	lnwire.ChanUpdate, error) {

	// Helper function to extract the owner of the given policy.
	owner := func(edge channeldb.ChanEdgePolicy) []byte {
		var pubKey *btcec.PublicKey
		if edge.IsNode1() {
			pubKey, _ = info.NodeKey1()
		} else {
			pubKey, _ = info.NodeKey2()
		}

		// If pubKey was not found, just return nil.
		if pubKey == nil {
			return nil
		}

		return pubKey.SerializeCompressed()
	}

	// Extract the channel update from the policy we own, if any.
	for _, edge := range policies {
		if edge != nil && bytes.Equal(ownerPubKey, owner(edge)) {
			return ChannelUpdateFromEdge(info, edge)
		}
	}

	return nil, ErrUnableToExtractChanUpdate
}

// UnsignedChannelUpdateFromEdge reconstructs an unsigned ChannelUpdate from the
// given edge info and policy.
func UnsignedChannelUpdateFromEdge(info *channeldb.ChannelEdgeInfo,
	policy channeldb.ChanEdgePolicy) lnwire.ChanUpdate {

	switch p := policy.(type) {
	case *channeldb.ChannelEdgePolicy1:
		return &lnwire.ChannelUpdate{
			ChainHash:       info.ChainHash,
			ShortChannelID:  lnwire.NewShortChanIDFromInt(p.ChannelID),
			Timestamp:       uint32(p.LastUpdate.Unix()),
			ChannelFlags:    p.ChannelFlags,
			MessageFlags:    p.MessageFlags,
			TimeLockDelta:   p.TimeLockDelta,
			HtlcMinimumMsat: p.MinHTLC,
			HtlcMaximumMsat: p.MaxHTLC,
			BaseFee:         uint32(p.FeeBaseMSat),
			FeeRate:         uint32(p.FeeProportionalMillionths),
			ExtraOpaqueData: p.ExtraOpaqueData,
		}
	case *channeldb.ChanEdgePolicy2:
	}
	panic("implement me")
}

func UnsignedChannelUpdateFromEdge1(info *channeldb.ChannelEdgeInfo,
	policy *channeldb.ChannelEdgePolicy1) *lnwire.ChannelUpdate {

	return &lnwire.ChannelUpdate{
		ChainHash:       info.ChainHash,
		ShortChannelID:  lnwire.NewShortChanIDFromInt(policy.ChannelID),
		Timestamp:       uint32(policy.LastUpdate.Unix()),
		ChannelFlags:    policy.ChannelFlags,
		MessageFlags:    policy.MessageFlags,
		TimeLockDelta:   policy.TimeLockDelta,
		HtlcMinimumMsat: policy.MinHTLC,
		HtlcMaximumMsat: policy.MaxHTLC,
		BaseFee:         uint32(policy.FeeBaseMSat),
		FeeRate:         uint32(policy.FeeProportionalMillionths),
		ExtraOpaqueData: policy.ExtraOpaqueData,
	}
}

// UnsignedChannelUpdate2FromEdge reconstructs an unsigned ChannelUpdate from
// the given edge info and policy.
func UnsignedChannelUpdate2FromEdge(chainHash chainhash.Hash,
	policy *channeldb.ChanEdgePolicy2) *lnwire.ChannelUpdate2 {

	policy.ChainHash = chainHash

	return policy.ChannelUpdate2
}

// ChannelUpdateFromEdge reconstructs a signed ChannelUpdate from the given edge
// info and policy.
func ChannelUpdateFromEdge(info *channeldb.ChannelEdgeInfo,
	policy channeldb.ChanEdgePolicy) (lnwire.ChanUpdate, error) {

	switch p := policy.(type) {
	case *channeldb.ChannelEdgePolicy1:
		upd, err := ChannelUpdate1FromEdge(info, p)
		if err != nil {
			return nil, err
		}

		return upd, nil
	case *channeldb.ChanEdgePolicy2:
		upd := ChannelUpdate2FromEdge(info, p)

		return upd, nil
	default:
		return nil, fmt.Errorf("expected channel edge policy")
	}
}

func ChannelUpdate1FromEdge(info *channeldb.ChannelEdgeInfo,
	policy *channeldb.ChannelEdgePolicy1) (*lnwire.ChannelUpdate, error) {

	update := UnsignedChannelUpdateFromEdge1(info, policy)

	var err error
	update.Signature, err = lnwire.NewSigFromECDSARawSignature(
		policy.SigBytes,
	)
	if err != nil {
		return nil, err
	}

	return update, nil
}

// ChannelUpdate2FromEdge reconstructs a signed ChannelUpdate from the given
// edge info and policy.
func ChannelUpdate2FromEdge(info *channeldb.ChannelEdgeInfo,
	policy *channeldb.ChanEdgePolicy2) *lnwire.ChannelUpdate2 {

	update := UnsignedChannelUpdate2FromEdge(info.ChainHash, policy)

	return update
}
