package netann

import (
	"bytes"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnwire"
)

// ErrUnableToExtractChanUpdate is returned when a channel update cannot be
// found for one of our active channels.
var ErrUnableToExtractChanUpdate = fmt.Errorf("unable to extract ChannelUpdate1")

// ChannelUpdateModifier is a closure that makes in-place modifications to an
// lnwire.ChannelUpdate1.
type ChannelUpdateModifier func(lnwire.ChannelUpdate)

// ChanUpdSetDisable is a functional option that sets the disabled channel flag
// if disabled is true, and clears the bit otherwise.
func ChanUpdSetDisable(disabled bool) ChannelUpdateModifier {
	return func(update lnwire.ChannelUpdate) {
		update.SetDisabled(disabled)
	}
}

// ChanUpdSetTimestamp is a functional option that sets the timestamp of the
// update to the current time, or increments it if the timestamp is already in
// the future.
func ChanUpdSetTimestamp(update lnwire.ChannelUpdate) {
	switch upd := update.(type) {
	case *lnwire.ChannelUpdate1:
		newTimestamp := uint32(time.Now().Unix())
		if newTimestamp <= upd.Timestamp {
			// Increment the prior value to ensure the timestamp
			// monotonically increases, otherwise the update won't
			// propagate.
			newTimestamp = upd.Timestamp + 1
		}
		upd.Timestamp = newTimestamp

	default:
		log.Errorf("unhandled implementation of "+
			"lnwire.ChannelUpdate: %T", update)
	}
}

// SignChannelUpdate applies the given modifiers to the passed
// lnwire.ChannelUpdate1, then signs the resulting update. The provided update
// should be the most recent, valid update, otherwise the timestamp may not
// monotonically increase from the prior.
//
// NOTE: This method modifies the given update.
func SignChannelUpdate(signer keychain.MessageSignerRing,
	keyLoc keychain.KeyLocator, update lnwire.ChannelUpdate,
	mods ...ChannelUpdateModifier) error {

	// Apply the requested changes to the channel update.
	for _, modifier := range mods {
		modifier(update)
	}

	switch upd := update.(type) {
	case *lnwire.ChannelUpdate1:
		data, err := upd.DataToSign()
		if err != nil {
			return err
		}

		sig, err := signer.SignMessage(keyLoc, data, true)
		if err != nil {
			return err
		}

		// Parse the DER-encoded signature into a fixed-size 64-byte
		// array.
		upd.Signature, err = lnwire.NewSigFromSignature(sig)
		if err != nil {
			return err
		}

	case *lnwire.ChannelUpdate2:
		data, err := upd.DigestToSignNoHash()
		if err != nil {
			return err
		}

		sig, err := signer.SignMessageSchnorr(keyLoc, data, false, nil)
		if err != nil {
			return err
		}

		upd.Signature, err = lnwire.NewSigFromSignature(sig)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unhandled implementaion of "+
			"ChannelUpdate: %T", update)
	}

	return nil
}

// ExtractChannelUpdate attempts to retrieve a lnwire.ChannelUpdate1 message from
// an edge's info and a set of routing policies.
//
// NOTE: The passed policies can be nil.
func ExtractChannelUpdate(ownerPubKey []byte,
	info models.ChannelEdgeInfo,
	policies ...models.ChannelEdgePolicy) (
	*lnwire.ChannelUpdate1, error) {

	// Helper function to extract the owner of the given policy.
	owner := func(edge models.ChannelEdgePolicy) []byte {
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

// UnsignedChannelUpdateFromEdge reconstructs an unsigned ChannelUpdate1 from the
// given edge info and policy.
func UnsignedChannelUpdateFromEdge(chainHash chainhash.Hash,
	policy *channeldb.ChannelEdgePolicy1) *lnwire.ChannelUpdate1 {

	return &lnwire.ChannelUpdate1{
		ChainHash:       chainHash,
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

// ChannelUpdateFromEdge reconstructs a signed ChannelUpdate1 from the given edge
// info and policy.
func ChannelUpdateFromEdge(info models.ChannelEdgeInfo,
	policy models.ChannelEdgePolicy) (*lnwire.ChannelUpdate1, error) {

	p, ok := policy.(*channeldb.ChannelEdgePolicy1)
	if !ok {
		return nil, fmt.Errorf("expected chan edge policy 1")
	}

	update := UnsignedChannelUpdateFromEdge(info.GetChainHash(), p)

	var err error
	update.Signature, err = lnwire.NewSigFromECDSARawSignature(
		p.SigBytes,
	)
	if err != nil {
		return nil, err
	}

	return update, nil
}
