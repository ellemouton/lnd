package itest

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/stretchr/testify/require"
)

func testRemoteGraph(ht *lntest.HarnessTest) {
	var (
		alice        = ht.Alice
		bob          = ht.Bob
		descGraphReq = &lnrpc.ChannelGraphRequest{
			IncludeUnannounced: true,
		}
	)
	// Set up a network:
	// A <- B <- C
	carol := ht.NewNode("Carol", nil)
	setupNetwork(ht, carol)

	assertDescGraph := func(node *node.HarnessNode, numEdges int,
		nodes ...*node.HarnessNode) {

		descGraphResp := node.RPC.DescribeGraph(descGraphReq)
		require.Len(ht.T, descGraphResp.Edges, numEdges)

		knownNodes := map[string]bool{
			// A node will always know about itself.
			node.PubKeyStr: true,
		}

		for _, n := range descGraphResp.Nodes {
			knownNodes[n.PubKey] = true
		}
		require.Len(ht.T, knownNodes, len(nodes)+1)

		for _, n := range nodes {
			require.True(ht.T, knownNodes[n.PubKeyStr])
		}
	}

	// Alice should know about Alice, Bob and Carol along with the 2 public
	// channels.
	assertDescGraph(alice, 2, bob, carol)

	// Create graph provider node, Greg. Don't connect it to any nodes yet.
	greg := ht.NewNode("Greg", nil)

	// Greg should just know about himself right now. He should not know
	// about any channels yet.
	assertDescGraph(greg, 0)

	// Create a node, Zane, that uses Greg as its graph provider.
	zane := ht.NewNode("Zane", []string{
		"--gossip.no-sync",
		"--remotegraph.enable",
		fmt.Sprintf(
			"--remotegraph.rpchost=localhost:%d", greg.Cfg.RPCPort,
		),
		fmt.Sprintf(
			"--remotegraph.tlscertpath=%s", greg.Cfg.TLSCertPath,
		),
		fmt.Sprintf(
			"--remotegraph.macaroonpath=%s", greg.Cfg.AdminMacPath,
		),
	})

	// Zane should know about Zane and Greg. He should not know about any
	// channels yet.
	assertDescGraph(zane, 0, greg)

	// Connect Z to C. Open a private channel. Show that it still only
	// knows about itself, G and now C. Ie, this shows it doesn't sync
	// gossip.
	ht.FundCoins(btcutil.SatoshiPerBitcoin, zane)
	ht.EnsureConnected(zane, carol)

	// Even though zane is now connected to carol, he should not sync gossip
	// and so should still only know about himself and greg.
	assertDescGraph(zane, 0, greg)

	// Now open a private channel between Zane and Carol.
	chanPointZane := ht.OpenChannel(
		zane, carol, lntest.OpenChannelParams{
			Private: true,
			Amt:     btcutil.Amount(100000),
		},
	)
	ht.T.Cleanup(func() {
		ht.CloseChannel(zane, chanPointZane)
	})

	// Now, Zane should know about Zane, Greg, and Carol along with a single
	// channel.
	assertDescGraph(zane, 1, greg, carol)

	// Now, connect G to B. Wait for it to sync gossip. Show that Z knows
	// about everything G knows about. G doesn't know about Z's private
	// channel.
	ht.EnsureConnected(greg, bob)

	// Greg should know about the two public channels along with the public
	// nodes. It does not know about Zane since Zane's channel connecting it
	// to the graph is private.
	assertDescGraph(greg, 2, alice, bob, carol)

	// Since Zane is using Greg as its graph provider, it should know about
	// all the channels and nodes that Greg knows of and in addition should
	// know about its own private channel.
	assertDescGraph(zane, 3, alice, bob, carol, greg)

	// Let Alice generate an invoice. Let Z settle it. Should succeed.
	invoice := alice.RPC.AddInvoice(&lnrpc.Invoice{Value: 100})

	// Zane should be able to settle the invoice.
	ht.CompletePaymentRequests(zane, []string{invoice.PaymentRequest})

	// Just to prove that Zane did not persist any of the graph updates
	// required to make the payment above, delete all the data persisted by
	// Greg, restart Greg and this time dont connect it to the graph. Now
	// show that Zane once again only sees a limited graph.
	require.NoError(ht.T, os.RemoveAll(greg.Cfg.DBDir()))

	ht.RestartNode(greg)

	assertDescGraph(zane, 1, greg, carol)
}

func setupNetwork(ht *lntest.HarnessTest, carol *node.HarnessNode) {
	const chanAmt = btcutil.Amount(100000)
	var networkChans []*lnrpc.ChannelPoint

	// Open a channel with 100k satoshis between Alice and Bob with Bob
	// being the sole funder of the channel.
	chanPointAlice := ht.OpenChannel(
		ht.Bob, ht.Alice, lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)
	networkChans = append(networkChans, chanPointAlice)

	// Create a channel between Carol and Bob.
	ht.FundCoins(btcutil.SatoshiPerBitcoin, carol)
	ht.EnsureConnected(ht.Bob, carol)
	chanPointBob := ht.OpenChannel(
		carol, ht.Bob, lntest.OpenChannelParams{
			Amt: chanAmt,
		},
	)
	networkChans = append(networkChans, chanPointBob)

	// Wait for all nodes to have seen all channels.
	nodes := []*node.HarnessNode{ht.Alice, ht.Bob, carol}
	for _, chanPoint := range networkChans {
		for _, node := range nodes {
			ht.AssertTopologyChannelOpen(node, chanPoint)
		}
	}

	ht.T.Cleanup(func() {
		ht.CloseChannel(ht.Alice, chanPointAlice)
		ht.CloseChannel(ht.Bob, chanPointBob)
	})
}
