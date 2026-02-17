package netann_test

import (
	"errors"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/netann"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

type mockSigner struct {
	err error
}

func (m *mockSigner) SignMessage(_ keychain.KeyLocator,
	_ []byte, _ bool) (*ecdsa.Signature, error) {

	if m.err != nil {
		return nil, m.err
	}

	return nil, nil
}

var _ lnwallet.MessageSigner = (*mockSigner)(nil)

type mockSchnorrSigner struct {
	err     error
	privKey *btcec.PrivateKey
}

func (m *mockSchnorrSigner) SignMessageSchnorr(_ keychain.KeyLocator,
	msg []byte, doubleHash bool, _ []byte,
	tag []byte) (*schnorr.Signature, error) {

	if m.err != nil {
		return nil, m.err
	}

	if m.privKey == nil {
		return nil, nil
	}

	var digest []byte
	switch {
	case len(tag) > 0:
		hash := chainhash.TaggedHash(tag, msg)
		digest = hash[:]

	case doubleHash:
		digest = chainhash.DoubleHashB(msg)

	default:
		digest = chainhash.HashB(msg)
	}

	return schnorr.Sign(m.privKey, digest)
}

var _ netann.ChannelUpdate2Signer = (*mockSchnorrSigner)(nil)

var (
	privKey, _    = btcec.NewPrivateKey()
	privKeySigner = keychain.NewPrivKeyMessageSigner(privKey, testKeyLoc)

	pubKey = privKey.PubKey()

	errFailedToSign = errors.New("unable to sign message")
)

type updateDisableTest struct {
	name         string
	startEnabled bool
	disable      bool
	startTime    time.Time
	signer       lnwallet.MessageSigner
	expErr       error
}

var updateDisableTests = []updateDisableTest{
	{
		name:         "working signer enabled to disabled",
		startEnabled: true,
		disable:      true,
		startTime:    time.Now(),
		signer:       netann.NewNodeSigner(privKeySigner),
	},
	{
		name:         "working signer enabled to enabled",
		startEnabled: true,
		disable:      false,
		startTime:    time.Now(),
		signer:       netann.NewNodeSigner(privKeySigner),
	},
	{
		name:         "working signer disabled to enabled",
		startEnabled: false,
		disable:      false,
		startTime:    time.Now(),
		signer:       netann.NewNodeSigner(privKeySigner),
	},
	{
		name:         "working signer disabled to disabled",
		startEnabled: false,
		disable:      true,
		startTime:    time.Now(),
		signer:       netann.NewNodeSigner(privKeySigner),
	},
	{
		name:         "working signer future monotonicity",
		startEnabled: true,
		disable:      true,
		startTime:    time.Now().Add(time.Hour), // must increment
		signer:       netann.NewNodeSigner(privKeySigner),
	},
	{
		name:      "failing signer",
		startTime: time.Now(),
		signer:    &mockSigner{err: errFailedToSign},
		expErr:    errFailedToSign,
	},
	{
		name:      "invalid sig from signer",
		startTime: time.Now(),
		signer:    &mockSigner{}, // returns a nil signature
		expErr:    errors.New("cannot decode empty signature"),
	},
}

// TestUpdateDisableFlag checks the behavior of UpdateDisableFlag, asserting
// that the proper channel flags are set, the timestamp always increases
// monotonically, and that the correct errors are returned in the event that the
// signer is unable to produce a signature.
func TestUpdateDisableFlag(t *testing.T) {
	t.Parallel()

	for _, tc := range updateDisableTests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Create the initial update, the only fields we are
			// concerned with in this test are the timestamp and the
			// channel flags.
			ogUpdate := &lnwire.ChannelUpdate1{
				Timestamp: uint32(tc.startTime.Unix()),
			}
			if !tc.startEnabled {
				ogUpdate.ChannelFlags |= lnwire.ChanUpdateDisabled
			}

			// Create new update to sign using the same fields as
			// the original. UpdateDisableFlag will mutate the
			// passed channel update, so we keep the old one to test
			// against.
			newUpdate := &lnwire.ChannelUpdate1{
				Timestamp:    ogUpdate.Timestamp,
				ChannelFlags: ogUpdate.ChannelFlags,
			}

			// Attempt to update and sign the new update, specifying
			// disabled or enabled as prescribed in the test case.
			err := netann.SignChannelUpdate(
				tc.signer, testKeyLoc, newUpdate,
				netann.ChanUpdSetDisable(tc.disable),
				netann.ChanUpdSetTimestamp,
			)

			if tc.expErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, tc.expErr.Error())
			}

			// Exit early if the test expected a failure.
			if tc.expErr != nil {
				return
			}

			// Verify that the timestamp has increased from the
			// original update.
			require.Greater(
				t, newUpdate.Timestamp, ogUpdate.Timestamp,
				"update timestamp should be monotonically "+
					"increasing",
			)

			// Verify that the disabled flag is properly set.
			disabled := newUpdate.ChannelFlags&
				lnwire.ChanUpdateDisabled != 0
			require.Equal(t, tc.disable, disabled)

			// Finally, validate the signature using the router's
			// verification logic.
			err = netann.VerifyChannelUpdateSignature(
				newUpdate, pubKey,
			)
			require.NoError(t, err)
		})
	}
}

func TestSignChannelUpdate2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		signer netann.ChannelUpdate2Signer
		expErr error
	}{
		{
			name: "working signer",
			signer: &mockSchnorrSigner{
				privKey: privKey,
			},
		},
		{
			name: "failing signer",
			signer: &mockSchnorrSigner{
				err: errFailedToSign,
			},
			expErr: errFailedToSign,
		},
	}

	for _, tc := range tests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			upd := &lnwire.ChannelUpdate2{
				ChainHash: tlv.NewPrimitiveRecord[tlv.TlvType0](
					chainhash.Hash{},
				),
				ShortChannelID: tlv.NewRecordT[tlv.TlvType2](
					lnwire.ShortChannelID{
						BlockHeight: 500,
						TxIndex:     2,
						TxPosition:  1,
					},
				),
				BlockHeight: tlv.NewPrimitiveRecord[tlv.TlvType4](
					uint32(600),
				),
				DisabledFlags: tlv.NewPrimitiveRecord[tlv.TlvType6](
					lnwire.ChanUpdateDisableFlags(0),
				),
				CLTVExpiryDelta: tlv.NewPrimitiveRecord[tlv.TlvType10](
					uint16(80),
				),
				HTLCMinimumMsat: tlv.NewPrimitiveRecord[tlv.TlvType12](
					lnwire.MilliSatoshi(1000),
				),
				HTLCMaximumMsat: tlv.NewPrimitiveRecord[tlv.TlvType14](
					lnwire.MilliSatoshi(2_000_000),
				),
				FeeBaseMsat: tlv.NewPrimitiveRecord[tlv.TlvType16](
					uint32(1000),
				),
				FeeProportionalMillionths: tlv.NewPrimitiveRecord[tlv.TlvType18](uint32(100)),
			}

			err := netann.SignChannelUpdate2(
				tc.signer, testKeyLoc, upd,
				func(update *lnwire.ChannelUpdate2) {
					update.ShortChannelID.Val = lnwire.ShortChannelID{
						BlockHeight: 700,
						TxIndex:     3,
						TxPosition:  0,
					}
				},
			)

			if tc.expErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.EqualError(t, err, tc.expErr.Error())
			}

			if tc.expErr != nil {
				return
			}

			require.Equal(t, uint32(700), upd.ShortChannelID.Val.BlockHeight)

			err = netann.VerifyChannelUpdateSignature(upd, pubKey)
			require.NoError(t, err)
		})
	}
}
