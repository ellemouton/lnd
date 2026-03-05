package netann_test

import (
	"image/color"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/netann"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

// TestSignNodeAnnouncementV1 tests that SignNodeAnnouncement correctly applies
// options and produces a valid ECDSA signature for a NodeAnnouncement1.
func TestSignNodeAnnouncementV1(t *testing.T) {
	t.Parallel()

	signer := netann.NewNodeSigner(
		keychain.NewPrivKeyMessageSigner(privKey, testKeyLoc),
	)

	ann := &lnwire.NodeAnnouncement1{
		Timestamp: uint32(time.Now().Unix()),
		Features:  lnwire.NewRawFeatureVector(),
	}
	copy(ann.NodeID[:], pubKey.SerializeCompressed())

	alias, err := lnwire.NewNodeAlias("test-node-v1")
	require.NoError(t, err)

	testColor := color.RGBA{R: 255, G: 128, B: 0}

	err = netann.SignNodeAnnouncement(
		signer, testKeyLoc, ann,
		netann.NodeAnnSetAlias(alias),
		netann.NodeAnnSetColor(testColor),
		netann.NodeAnnSetTimestamp(time.Now()),
	)
	require.NoError(t, err)

	// Verify the options were applied.
	require.Equal(t, alias, ann.Alias)
	require.Equal(t, testColor, ann.RGBColor)

	// Verify the signature validates.
	err = netann.ValidateNodeAnnSignature(ann)
	require.NoError(t, err)
}

// TestSignNodeAnnouncementV2 tests that SignNodeAnnouncement correctly applies
// options and produces a valid Schnorr signature for a NodeAnnouncement2.
func TestSignNodeAnnouncementV2(t *testing.T) {
	t.Parallel()

	signer := netann.NewNodeSigner(
		keychain.NewPrivKeyMessageSigner(privKey, testKeyLoc),
	)

	ann := &lnwire.NodeAnnouncement2{
		BlockHeight: tlv.NewPrimitiveRecord[tlv.TlvType2](
			uint32(100),
		),
		ExtraSignedFields: make(lnwire.ExtraSignedFields),
	}
	copy(ann.NodeID.Val[:], pubKey.SerializeCompressed())

	alias, err := lnwire.NewNodeAlias("test-node-v2")
	require.NoError(t, err)

	testColor := color.RGBA{R: 0, G: 255, B: 128}

	err = netann.SignNodeAnnouncement(
		signer, testKeyLoc, ann,
		netann.NodeAnnSetAlias(alias),
		netann.NodeAnnSetColor(testColor),
		netann.NodeAnnSetBlockHeight(200),
	)
	require.NoError(t, err)

	// Verify alias was applied.
	ann.Alias.WhenSome(
		func(r tlv.RecordT[tlv.TlvType3, lnwire.NodeAlias2]) {
			require.Equal(t, lnwire.NodeAlias2("test-node-v2"),
				r.Val)
		},
	)
	require.True(t, ann.Alias.IsSome())

	// Verify color was applied.
	ann.Color.WhenSome(
		func(r tlv.RecordT[tlv.TlvType1, lnwire.Color]) {
			require.Equal(t, lnwire.Color(testColor), r.Val)
		},
	)
	require.True(t, ann.Color.IsSome())

	// Verify block height was updated (200 > 100).
	require.Equal(t, uint32(200), ann.BlockHeight.Val)

	// Verify the Schnorr signature validates.
	err = netann.ValidateNodeAnnSignature(ann)
	require.NoError(t, err)
}

// TestNodeAnnBlockHeightMonotonic tests that the block height on a v2 node
// announcement is always monotonically increasing.
func TestNodeAnnBlockHeightMonotonic(t *testing.T) {
	t.Parallel()

	signer := netann.NewNodeSigner(
		keychain.NewPrivKeyMessageSigner(privKey, testKeyLoc),
	)

	ann := &lnwire.NodeAnnouncement2{
		BlockHeight: tlv.NewPrimitiveRecord[tlv.TlvType2](
			uint32(500),
		),
		ExtraSignedFields: make(lnwire.ExtraSignedFields),
	}
	copy(ann.NodeID.Val[:], pubKey.SerializeCompressed())

	// Setting a height lower than current should bump to current+1.
	err := netann.SignNodeAnnouncement(
		signer, testKeyLoc, ann,
		netann.NodeAnnSetBlockHeight(100),
	)
	require.NoError(t, err)
	require.Equal(t, uint32(501), ann.BlockHeight.Val)

	// Setting a height higher than current should use the new height.
	err = netann.SignNodeAnnouncement(
		signer, testKeyLoc, ann,
		netann.NodeAnnSetBlockHeight(1000),
	)
	require.NoError(t, err)
	require.Equal(t, uint32(1000), ann.BlockHeight.Val)
}
