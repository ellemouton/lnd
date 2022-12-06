package itest

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/funding"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/watchtowerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/wtclientrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/stretchr/testify/require"
)

// testWatchtowerSessionManagement tests that session deletion is done
// correctly.
func testWatchtowerSessionManagement(net *lntest.NetworkHarness,
	t *harnessTest) {

	ctx := context.Background()
	const (
		chanAmt           = funding.MaxBtcFundingAmount
		paymentAmt        = 100
		numInvoices       = 5
		maxUpdates        = numInvoices * 2
		externalIP        = "1.2.3.4"
		sessionCloseRange = 1
	)

	// Set up Wallis the watchtower who will be used by Dave to watch over
	// his channel commitment transactions.
	wallis := net.NewNode(t.t, "Wallis", []string{
		"--watchtower.active",
		"--watchtower.externalip=" + externalIP,
	})
	defer shutdownAndAssert(net, t, wallis)

	ctxt, _ := context.WithTimeout(ctx, defaultTimeout)
	wallisInfo, err := wallis.Watchtower.GetInfo(
		ctxt, &watchtowerrpc.GetInfoRequest{},
	)
	require.NoError(t.t, err)

	// Assert that Wallis has one listener and it is 0.0.0.0:9911 or
	// [::]:9911. Since no listener is explicitly specified, one of these
	// should be the default depending on whether the host supports IPv6 or
	// not.
	require.Len(t.t, wallisInfo.Listeners, 1)
	listener := wallisInfo.Listeners[0]
	require.True(t.t, listener == "0.0.0.0:9911" || listener == "[::]:9911")

	// Assert the Wallis's URIs properly display the chosen external IP.
	require.Len(t.t, wallisInfo.Uris, 1)
	require.Contains(t.t, wallisInfo.Uris[0], externalIP)

	// Dave will be the tower client.
	daveArgs := []string{
		"--wtclient.active",
		fmt.Sprintf("--wtclient.max-updates=%d", maxUpdates),
		fmt.Sprintf(
			"--wtclient.session-close-range=%d", sessionCloseRange,
		),
	}
	dave := net.NewNode(t.t, "Dave", daveArgs)
	defer shutdownAndAssert(net, t, dave)

	ctxt, _ = context.WithTimeout(ctx, defaultTimeout)
	addTowerReq := &wtclientrpc.AddTowerRequest{
		Pubkey:  wallisInfo.Pubkey,
		Address: listener,
	}
	_, err = dave.WatchtowerClient.AddTower(ctxt, addTowerReq)
	require.NoError(t.t, err)

	// Assert that there exists a session between Dave and Wallis.
	var numSessions uint32
	err = wait.Predicate(func() bool {
		ctxt, _ = context.WithTimeout(ctx, defaultTimeout)
		info, err := dave.WatchtowerClient.GetTowerInfo(
			ctxt, &wtclientrpc.GetTowerInfoRequest{
				Pubkey:          wallisInfo.Pubkey,
				IncludeSessions: true,
			},
		)
		require.NoError(t.t, err)

		numSessions = info.NumSessions

		return numSessions > 0
	}, defaultTimeout)
	require.NoError(t.t, err)

	// Open a channel between Dave and Alice.
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, dave)
	net.ConnectNodes(t.t, dave, net.Alice)
	chanPoint := openChannelAndAssert(
		t, net, dave, net.Alice, lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)

	// Since there are 2 updates made for every payment and the maximum
	// number of updates per session has been set to 10, make 5 payments
	// between the pair so that the session is exhausted.
	alicePayReqs, _, _, err := createPayReqs(
		net.Alice, paymentAmt, numInvoices,
	)
	require.NoError(t.t, err)

	send := func(node *lntest.HarnessNode, payReq string) {
		ctxt, _ = context.WithTimeout(
			ctx, lntest.AsyncBenchmarkTimeout,
		)
		stream, err := node.RouterClient.SendPaymentV2(
			ctxt,
			&routerrpc.SendPaymentRequest{
				PaymentRequest: payReq,
				TimeoutSeconds: 60,
				FeeLimitMsat:   noFeeLimitMsat,
			},
		)
		require.NoError(t.t, err)

		result, err := getPaymentResult(stream)
		require.NoError(t.t, err)

		require.Equal(t.t, result.Status, lnrpc.Payment_SUCCEEDED)
	}

	for i := 0; i < numInvoices; i++ {
		send(dave, alicePayReqs[i])
	}

	// Assert that one of the sessions now has 10 backups.
	err = wait.Predicate(func() bool {
		ctxt, _ = context.WithTimeout(ctx, defaultTimeout)
		info, err := dave.WatchtowerClient.GetTowerInfo(
			ctxt, &wtclientrpc.GetTowerInfoRequest{
				Pubkey:          wallisInfo.Pubkey,
				IncludeSessions: true,
			},
		)
		require.NoError(t.t, err)

		var numBackups uint32
		for _, session := range info.Sessions {
			numBackups += session.NumBackups
		}

		return numBackups == 10
	}, defaultTimeout)
	require.NoError(t.t, err)

	// Now close the channel.
	closeChannelAndAssert(t, net, dave, chanPoint, true)

	// Mine enough blocks to surpass the session-close-range. This should
	// trigger the session to be deleted.
	mineBlocks(t, net, sessionCloseRange+1, 0)

	// Wait for the session to be deleted. We know it has been deleted once
	// the number of sessions is 1 less than it was initially and when the
	// number of backups is zero.
	err = wait.Predicate(func() bool {
		ctxt, _ = context.WithTimeout(ctx, defaultTimeout)
		info, err := dave.WatchtowerClient.GetTowerInfo(
			ctxt, &wtclientrpc.GetTowerInfoRequest{
				Pubkey:          wallisInfo.Pubkey,
				IncludeSessions: true,
			},
		)
		require.NoError(t.t, err)

		if len(info.Sessions) != int(numSessions-1) {
			return false
		}

		var (
			numBackups uint32
		)
		for _, session := range info.Sessions {
			numBackups += session.NumBackups
		}

		return numBackups == 0
	}, defaultTimeout)
	require.NoError(t.t, err)
}
