package itest

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/routerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/watchtowerrpc"
	"github.com/lightningnetwork/lnd/lnrpc/wtclientrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/watchtower/wtpolicy"
	"github.com/stretchr/testify/require"
)

//func createBigTowerClientDB(net *lntest.NetworkHarness, t *harnessTest) {
//	const (
//		externalIP  = "1.2.3.4"
//		chanAmt     = funding.MaxBtcFundingAmount
//		paymentAmt  = 10
//		numInvoices = 10
//	)
//
//	ctx := context.Background()
//	alice := net.Alice
//
//	// 1. Set up Wallis the watchtower.
//	wallis := net.NewNode(t.t, "Wallis", []string{
//		"--watchtower.active",
//		"--watchtower.externalip=" + externalIP,
//	})
//	defer shutdownAndAssert(net, t, wallis)
//
//	ctxt, _ := context.WithTimeout(ctx, defaultTimeout)
//	wallisInfo, err := wallis.Watchtower.GetInfo(
//		ctxt, &watchtowerrpc.GetInfoRequest{},
//	)
//	require.NoError(t.t, err)
//
//	// Assert that Wallis has one listener, and it is 0.0.0.0:9911 or
//	// [::]:9911. Since no listener is explicitly specified, one of these
//	// should be the default depending on whether the host supports IPv6 or
//	// not.
//	require.Len(t.t, wallisInfo.Listeners, 1)
//	listener := wallisInfo.Listeners[0]
//	require.True(t.t, listener == "0.0.0.0:9911" || listener == "[::]:9911")
//
//	// Assert the Wallis's URIs properly display the chosen external IP.
//	require.Len(t.t, wallisInfo.Uris, 1)
//	require.Contains(t.t, wallisInfo.Uris[0], externalIP)
//
//	// 2. Set up Dave as a tower client and connect dave to Wallis.
//	daveArgs := []string{
//		"--wtclient.active",
//		"--protocol.anchors",
//	}
//	dave := net.NewNode(t.t, "Dave", daveArgs)
//	defer shutdownAndAssert(net, t, dave)
//
//	// Send some money to Dave.
//	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, dave)
//
//	ctxt, _ = context.WithTimeout(ctx, defaultTimeout)
//	addTowerReq := &wtclientrpc.AddTowerRequest{
//		Pubkey:  wallisInfo.Pubkey,
//		Address: listener,
//	}
//	_, err = dave.WatchtowerClient.AddTower(ctxt, addTowerReq)
//	require.NoError(t.t, err)
//
//	net.ConnectNodes(t.t, dave, alice)
//
//	startTime := time.Now()
//
//	totalPayments := 0
//	totalExpected := 40 * 1000 * 40
//	var oneRun time.Time
//	for i := 0; i < 40; i++ {
//		// 3. Open a channel between Dave and Alice.
//		chanPoint := openChannelAndAssert(
//			t, net, dave, alice, lntest.OpenChannelParams{
//				Amt:     3 * (chanAmt / 4),
//				PushAmt: chanAmt / 4,
//			},
//		)
//
//		// 4. Send payments back and forth between them on this new
//		// channel.
//		for i := 0; i < 1000; i++ {
//			oneRun = time.Now()
//			alicePayReqs, _, _, err := createPayReqs(
//				alice, paymentAmt, numInvoices,
//			)
//			require.NoError(t.t, err)
//
//			err = completePaymentRequests(
//				dave, dave.RouterClient, alicePayReqs, false,
//			)
//			require.NoError(t.t, err)
//
//			davePayReqs, _, _, err := createPayReqs(
//				dave, paymentAmt, numInvoices,
//			)
//			require.NoError(t.t, err)
//
//			err = completePaymentRequests(
//				alice, alice.RouterClient, davePayReqs, false,
//			)
//			require.NoError(t.t, err)
//
//			totalPayments += numInvoices * 2
//
//			fractionCompleted := (float64(totalPayments)) / float64(totalExpected)
//			secondsTaken := time.Since(startTime).Seconds()
//
//			secondsPerPercent := secondsTaken / fractionCompleted
//
//			secondsTilCompletion := secondsPerPercent * (1 - fractionCompleted)
//
//			timeTilEnd := time.Duration(secondsTilCompletion) * time.Second
//
//			fmt.Printf("Stats: %d updates across %d sessions. Time for last 20 paymets: %s (percent complete: %f%%) (time left: %s) \n",
//				totalPayments,
//				int(math.Ceil(float64(totalPayments)/wtpolicy.DefaultMaxUpdates)),
//				time.Since(oneRun),
//				fractionCompleted*100, timeTilEnd.String(),
//			)
//		}
//
//		// 5. Close the channel.
//		closeChannelAndAssert(t, net, dave, chanPoint, true)
//	}
//
//	// 6. Repeat from step 3 to 5 a couple of times.
//
//	// 7. Make sure that DB is kept around & check what its size is.
//}

func createBigTowerClientDB(net *lntest.NetworkHarness, t *harnessTest) {
	const (
		externalIP = "1.2.3.4"
		paymentAmt = 1000

		numChans             = 5
		iterationsPerChannel = 100

		// numInvoices := int(input.MaxHTLCNumber / 2)
		numInvoices = int(200)
	)

	ctx := context.Background()
	alice := net.Alice

	// 1. Set up Wallis the watchtower.
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

	// Assert that Wallis has one listener, and it is 0.0.0.0:9911 or
	// [::]:9911. Since no listener is explicitly specified, one of these
	// should be the default depending on whether the host supports IPv6 or
	// not.
	require.Len(t.t, wallisInfo.Listeners, 1)
	listener := wallisInfo.Listeners[0]
	require.True(t.t, listener == "0.0.0.0:9911" || listener == "[::]:9911")

	// Assert the Wallis's URIs properly display the chosen external IP.
	require.Len(t.t, wallisInfo.Uris, 1)
	require.Contains(t.t, wallisInfo.Uris[0], externalIP)

	// 2. Set up Dave as a tower client and connect dave to Wallis.
	daveArgs := []string{
		"--wtclient.active",
		"--protocol.anchors",
	}
	dave := net.NewNode(t.t, "Dave", daveArgs)
	defer shutdownAndAssert(net, t, dave)

	// Send some money to Dave.
	net.SendCoins(t.t, btcutil.SatoshiPerBitcoin, dave)

	ctxt, _ = context.WithTimeout(ctx, defaultTimeout)
	addTowerReq := &wtclientrpc.AddTowerRequest{
		Pubkey:  wallisInfo.Pubkey,
		Address: listener,
	}
	_, err = dave.WatchtowerClient.AddTower(ctxt, addTowerReq)
	require.NoError(t.t, err)

	net.ConnectNodes(t.t, dave, alice)

	startTime := time.Now()

	totalPayments := 0
	totalExpected := numChans * iterationsPerChannel * (numInvoices * 2)

	var oneRun time.Time
	for i := 0; i < numChans; i++ {

		fmt.Println("NEW CHANNEL")
		// 3. Open a channel between Dave and Alice.
		chanPoint := openChannelAndAssert(
			t, net, dave, alice, lntest.OpenChannelParams{
				Amt:     paymentAmt * 2000,
				PushAmt: paymentAmt * 1000,
			},
		)

		// Wait for Alice to receive the channel edge from the funding manager.
		if err = alice.WaitForNetworkChannelOpen(chanPoint); err != nil {
			t.Fatalf("alice didn't see the alice->bob channel before "+
				"timeout: %v", err)
		}
		if err = dave.WaitForNetworkChannelOpen(chanPoint); err != nil {
			t.Fatalf("bob didn't see the bob->alice channel before "+
				"timeout: %v", err)
		}

		for i := 0; i < iterationsPerChannel; i++ {
			oneRun = time.Now()

			alicePayReqs, _, _, err := createPayReqs(
				alice, paymentAmt, numInvoices,
			)
			require.NoError(t.t, err)

			davePayReqs, _, _, err := createPayReqs(
				dave, paymentAmt, numInvoices,
			)
			require.NoError(t.t, err)

			// Send payments from Alice to Bob and from Bob to Alice in
			// async manner.
			errChan := make(chan error)
			statusChan := make(chan *lnrpc.Payment)

			send := func(node *lntest.HarnessNode, payReq string) {
				go func() {
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
					if err != nil {
						errChan <- err
					}
					result, err := getPaymentResult(stream)
					if err != nil {
						errChan <- err
					}

					statusChan <- result
				}()
			}

			for i := 0; i < numInvoices; i++ {
				send(dave, alicePayReqs[i])
				send(alice, davePayReqs[i])
			}

			// Expect all payments to succeed.
			for i := 0; i < 2*numInvoices; i++ {
				select {
				case result := <-statusChan:
					if result.Status != lnrpc.Payment_SUCCEEDED {
						t.Fatalf("payment error: %v", result.Status)
					}

				case err := <-errChan:
					t.Fatalf("payment error: %v", err)
				}
			}

			totalPayments += numInvoices * 2

			fractionCompleted := (float64(totalPayments)) / float64(totalExpected)
			secondsTaken := time.Since(startTime).Seconds()

			secondsPerPercent := secondsTaken / fractionCompleted

			secondsTilCompletion := secondsPerPercent * (1 - fractionCompleted)

			timeTilEnd := time.Duration(secondsTilCompletion) * time.Second

			fmt.Printf("Stats: %d updates across %d sessions. Time for last %d paymets: %s (percent complete: %f%%) (time left: %s) \n",
				totalPayments,
				int(math.Ceil(float64(totalPayments)/wtpolicy.DefaultMaxUpdates)),
				numInvoices*2,
				time.Since(oneRun),
				fractionCompleted*100, timeTilEnd.String(),
			)
		}

		// 5. Close the channel.
		closeChannelAndAssert(t, net, dave, chanPoint, false)
	}

	// 6. Repeat from step 3 to 5 a couple of times.

	// 7. Make sure that DB is kept around & check what its size is.
}
