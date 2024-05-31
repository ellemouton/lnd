package channeldb

import (
	"bytes"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/channeldb/models"
	invpkg "github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/require"
)

var testNow = time.Unix(1, 0)

func genPubKey(t *testing.T) route.Vertex {
	pk, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	return route.NewVertex(pk.PubKey())
}

func TestEncodeDecodeBlindedPathMap(t *testing.T) {
	t.Parallel()

	blindedPath := map[route.Vertex]*models.MCRoute{
		genPubKey(t): {
			SourcePubKey: genPubKey(t),
			TotalAmount:  10000,
			Hops: []*models.MCHop{
				{
					PubKeyBytes:  genPubKey(t),
					AmtToForward: 60,
					ChannelID:    40,
				},
				{
					PubKeyBytes:      genPubKey(t),
					AmtToForward:     50,
					ChannelID:        10,
					HasBlindingPoint: true,
				},
			},
		},
		genPubKey(t): {
			SourcePubKey: genPubKey(t),
			TotalAmount:  30,
			Hops: []*models.MCHop{
				{
					PubKeyBytes:      genPubKey(t),
					AmtToForward:     50,
					ChannelID:        10,
					HasBlindingPoint: true,
				},
			},
		},
	}

	// We'll now make a sample invoice stream, and use that to encode the
	// amp state we created above.
	tlvStream, err := tlv.NewStream(
		tlv.MakeDynamicRecord(
			blindedPathMapType, &blindedPath,
			blindedPathMapRecordSize(blindedPath),
			blindedPathRouteEncoder, blindedPathRouteDecoder,
		),
	)
	require.Nil(t, err)

	// Next encode the stream into a set of raw bytes.
	var b bytes.Buffer
	err = tlvStream.Encode(&b)
	require.Nil(t, err)

	// Now create a new blank ampState map, which we'll use to decode the
	// bytes into.
	blindedPath2 := make(map[route.Vertex]*models.MCRoute)

	// Decode from the raw stream into this blank mpa.
	tlvStream, err = tlv.NewStream(
		tlv.MakeDynamicRecord(
			blindedPathMapType, &blindedPath2, nil,
			blindedPathRouteEncoder, blindedPathRouteDecoder,
		),
	)
	require.Nil(t, err)

	err = tlvStream.Decode(&b)
	require.Nil(t, err)

	// The two states should match.
	require.Len(t, blindedPath2, len(blindedPath))

	for k, m1 := range blindedPath {
		m2, ok := blindedPath2[k]
		require.True(t, ok)

		require.Equal(t, m1, m2)
	}
}

// TestEncodeDecodeAmpInvoiceState asserts that the nested TLV
// encoding+decoding for the AMPInvoiceState struct works as expected.
func TestEncodeDecodeAmpInvoiceState(t *testing.T) {
	t.Parallel()

	setID1 := [32]byte{1}
	setID2 := [32]byte{2}
	setID3 := [32]byte{3}

	circuitKey1 := models.CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(1), HtlcID: 1,
	}
	circuitKey2 := models.CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(2), HtlcID: 2,
	}
	circuitKey3 := models.CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(2), HtlcID: 3,
	}

	// Make a sample invoice state map that we'll encode then decode to
	// assert equality of.
	ampState := invpkg.AMPInvoiceState{
		setID1: invpkg.InvoiceStateAMP{
			State:       invpkg.HtlcStateSettled,
			SettleDate:  testNow,
			SettleIndex: 1,
			InvoiceKeys: map[models.CircuitKey]struct{}{
				circuitKey1: {},
				circuitKey2: {},
			},
			AmtPaid: 5,
		},
		setID2: invpkg.InvoiceStateAMP{
			State:       invpkg.HtlcStateCanceled,
			SettleDate:  testNow,
			SettleIndex: 2,
			InvoiceKeys: map[models.CircuitKey]struct{}{
				circuitKey1: {},
			},
			AmtPaid: 6,
		},
		setID3: invpkg.InvoiceStateAMP{
			State:       invpkg.HtlcStateAccepted,
			SettleDate:  testNow,
			SettleIndex: 3,
			InvoiceKeys: map[models.CircuitKey]struct{}{
				circuitKey1: {},
				circuitKey2: {},
				circuitKey3: {},
			},
			AmtPaid: 7,
		},
	}

	// We'll now make a sample invoice stream, and use that to encode the
	// amp state we created above.
	tlvStream, err := tlv.NewStream(
		tlv.MakeDynamicRecord(
			invoiceAmpStateType, &ampState,
			ampRecordSize(&ampState), ampStateEncoder,
			ampStateDecoder,
		),
	)
	require.Nil(t, err)

	// Next encode the stream into a set of raw bytes.
	var b bytes.Buffer
	err = tlvStream.Encode(&b)
	require.Nil(t, err)

	// Now create a new blank ampState map, which we'll use to decode the
	// bytes into.
	ampState2 := make(invpkg.AMPInvoiceState)

	// Decode from the raw stream into this blank mpa.
	tlvStream, err = tlv.NewStream(
		tlv.MakeDynamicRecord(
			invoiceAmpStateType, &ampState2, nil,
			ampStateEncoder, ampStateDecoder,
		),
	)
	require.Nil(t, err)

	err = tlvStream.Decode(&b)
	require.Nil(t, err)

	// The two states should match.
	require.Equal(t, ampState, ampState2)
}
