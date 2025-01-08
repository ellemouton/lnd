package autopilot

import (
	"bytes"
	"context"
	"errors"
	prand "math/rand"
	"net"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/graph/sources"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

type genGraphFunc func(t *testing.T) (testGraph, error)

type testGraph interface {
	ChannelGraph

	addRandChannel(*btcec.PublicKey, *btcec.PublicKey,
		btcutil.Amount) (*ChannelEdge, *ChannelEdge, error)

	addRandNode() (*btcec.PublicKey, error)
}

func newDiskChanGraph(t *testing.T) (testGraph, error) {
	backend, err := kvdb.GetBoltBackend(&kvdb.BoltBackendConfig{
		DBPath:            t.TempDir(),
		DBFileName:        "graph.db",
		NoFreelistSync:    true,
		AutoCompact:       false,
		AutoCompactMinAge: kvdb.DefaultBoltAutoCompactMinAge,
		DBTimeout:         kvdb.DefaultDBTimeout,
	})
	require.NoError(t, err)

	graphDB, err := graphdb.NewChannelGraph(backend)
	require.NoError(t, err)

	source := sources.NewDBGSource(graphDB)

	return &dbTestGraph{
		databaseChannelGraph: &databaseChannelGraph{
			db: source,
		},
		db: graphDB,
	}, nil
}

func newMemChanGraph(_ *testing.T) (testGraph, error) {
	return newMemChannelGraph(), nil
}

var _ testGraph = (*memChannelGraph)(nil)

var chanGraphs = []struct {
	name    string
	genFunc genGraphFunc
}{
	{
		name:    "disk_graph",
		genFunc: newDiskChanGraph,
	},
	{
		name:    "mem_graph",
		genFunc: newMemChanGraph,
	},
}

// TestPrefAttachmentSelectEmptyGraph ensures that when passed an
// empty graph, the NodeSores function always returns a score of 0.
func TestPrefAttachmentSelectEmptyGraph(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	prefAttach := NewPrefAttachment()

	// Create a random public key, which we will query to get a score for.
	pub, err := randKey()
	require.NoError(t, err, "unable to generate key")

	nodes := map[NodeID]struct{}{
		NewNodeID(pub): {},
	}

	for _, chanGraph := range chanGraphs {
		chanGraph := chanGraph
		graph, err := chanGraph.genFunc(t)
		require.NoError(t, err, "unable to create graph")

		success := t.Run(chanGraph.name, func(t1 *testing.T) {
			// With the necessary state initialized, we'll now
			// attempt to get the score for this one node.
			const walletFunds = btcutil.SatoshiPerBitcoin
			scores, err := prefAttach.NodeScores(
				ctx, graph, nil, walletFunds, nodes,
			)
			require.NoError(t1, err)

			// Since the graph is empty, we expect the score to be
			// 0, giving an empty return map.
			require.Empty(t1, scores)
		})
		if !success {
			break
		}
	}
}

// TestPrefAttachmentSelectTwoVertexes ensures that when passed a
// graph with only two eligible vertexes, then both are given the same score,
// and the funds are appropriately allocated across each peer.
func TestPrefAttachmentSelectTwoVertexes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	prand.Seed(time.Now().Unix())

	const (
		maxChanSize = btcutil.Amount(btcutil.SatoshiPerBitcoin)
	)

	for _, chanGraph := range chanGraphs {
		chanGraph := chanGraph
		graph, err := chanGraph.genFunc(t)
		require.NoError(t, err, "unable to create graph")

		success := t.Run(chanGraph.name, func(t1 *testing.T) {
			prefAttach := NewPrefAttachment()

			// For this set, we'll load the memory graph with two
			// nodes, and a random channel connecting them.
			const chanCapacity = btcutil.SatoshiPerBitcoin
			edge1, edge2, err := graph.addRandChannel(
				nil, nil, chanCapacity,
			)
			require.NoError(t1, err)

			// We also add a third, non-connected node to the graph.
			_, err = graph.addRandNode()
			require.NoError(t1, err)

			// Get the score for all nodes found in the graph at
			// this point.
			nodes := make(map[NodeID]struct{})
			err = graph.ForEachNode(ctx, func(n Node) error {
				nodes[n.PubKey()] = struct{}{}
				return nil
			})
			require.NoError(t1, err)

			require.Len(t1, nodes, 3)

			// With the necessary state initialized, we'll now
			// attempt to get our candidates channel score given
			// the current state of the graph.
			candidates, err := prefAttach.NodeScores(
				ctx, graph, nil, maxChanSize, nodes,
			)
			require.NoError(t1, err)

			// We expect two candidates, since one of the nodes
			// doesn't have any channels.
			require.Len(t1, candidates, 2)

			// The candidates should be amongst the two edges
			// created above.
			for nodeID, candidate := range candidates {
				edge1Pub := edge1.Peer.PubKey()
				edge2Pub := edge2.Peer.PubKey()

				switch {
				case bytes.Equal(nodeID[:], edge1Pub[:]):
				case bytes.Equal(nodeID[:], edge2Pub[:]):
				default:
					t1.Fatalf("attached to unknown node: %x",
						nodeID[:])
				}

				// Since each of the nodes has 1 channel, out
				// of only one channel in the graph, we expect
				// their score to be 1.0.
				require.EqualValues(t1, 1, candidate.Score)
			}
		})
		if !success {
			break
		}
	}
}

// TestPrefAttachmentSelectGreedyAllocation tests that if upon
// returning node scores, the NodeScores method will attempt to greedily
// allocate all funds to each vertex (up to the max channel size).
func TestPrefAttachmentSelectGreedyAllocation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	prand.Seed(time.Now().Unix())

	const (
		maxChanSize = btcutil.Amount(btcutil.SatoshiPerBitcoin)
	)

	for _, chanGraph := range chanGraphs {
		chanGraph := chanGraph
		graph, err := chanGraph.genFunc(t)
		require.NoError(t, err, "unable to create graph")

		success := t.Run(chanGraph.name, func(t1 *testing.T) {
			prefAttach := NewPrefAttachment()

			const chanCapacity = btcutil.SatoshiPerBitcoin

			// Next, we'll add 3 nodes to the graph, creating an
			// "open triangle topology".
			edge1, _, err := graph.addRandChannel(
				nil, nil, chanCapacity,
			)
			require.NoError(t1, err)

			peerPubBytes := edge1.Peer.PubKey()
			peerPub, err := btcec.ParsePubKey(peerPubBytes[:])
			require.NoError(t1, err)

			_, _, err = graph.addRandChannel(
				peerPub, nil, chanCapacity,
			)
			require.NoError(t1, err)

			// At this point, there should be three nodes in the
			// graph, with node having two edges.
			numNodes := 0
			twoChans := false
			nodes := make(map[NodeID]struct{})
			err = graph.ForEachNode(ctx, func(n Node) error {
				numNodes++
				nodes[n.PubKey()] = struct{}{}
				numChans := 0
				err := n.ForEachChannel(
					ctx, func(c ChannelEdge) error {
						numChans++
						return nil
					},
				)
				if err != nil {
					return err
				}

				twoChans = twoChans || (numChans == 2)

				return nil
			})
			require.NoError(t1, err)

			require.EqualValues(t1, 3, numNodes)
			require.True(t1, twoChans, "have two chans")

			// We'll now begin our test, modeling the available
			// wallet balance to be 5.5 BTC. We're shooting for a
			// 50/50 allocation, and have 3 BTC in channels. As a
			// result, the heuristic should try to greedily
			// allocate funds to channels.
			scores, err := prefAttach.NodeScores(
				ctx, graph, nil, maxChanSize, nodes,
			)
			require.NoError(t1, err)

			require.Equal(t1, len(nodes), len(scores))

			// The candidates should have a non-zero score, and
			// have the max chan size funds recommended channel
			// size.
			for _, candidate := range scores {
				require.NotZero(t1, candidate.Score)
			}

			// Imagine a few channels are being opened, and there's
			// only 0.5 BTC left. That should leave us with channel
			// candidates of that size.
			const remBalance = btcutil.SatoshiPerBitcoin * 0.5
			scores, err = prefAttach.NodeScores(
				ctx, graph, nil, remBalance, nodes,
			)
			require.NoError(t1, err)

			require.Equal(t1, len(nodes), len(scores))

			// Check that the recommended channel sizes are now the
			// remaining channel balance.
			for _, candidate := range scores {
				require.NotZero(t1, candidate.Score)
			}
		})
		if !success {
			break
		}
	}
}

// TestPrefAttachmentSelectSkipNodes ensures that if a node was
// already selected as a channel counterparty, then that node will get a score
// of zero during scoring.
func TestPrefAttachmentSelectSkipNodes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	prand.Seed(time.Now().Unix())

	const (
		maxChanSize = btcutil.Amount(btcutil.SatoshiPerBitcoin)
	)

	for _, chanGraph := range chanGraphs {
		chanGraph := chanGraph
		graph, err := chanGraph.genFunc(t)
		require.NoError(t, err, "unable to create graph")

		success := t.Run(chanGraph.name, func(t1 *testing.T) {
			prefAttach := NewPrefAttachment()

			// Next, we'll create a simple topology of two nodes,
			// with a single channel connecting them.
			const chanCapacity = btcutil.SatoshiPerBitcoin
			_, _, err = graph.addRandChannel(nil, nil, chanCapacity)
			require.NoError(t1, err)

			nodes := make(map[NodeID]struct{})
			err = graph.ForEachNode(ctx, func(n Node) error {
				nodes[n.PubKey()] = struct{}{}
				return nil
			})
			require.NoError(t1, err)

			require.Len(t1, nodes, 2)

			// With our graph created, we'll now get the scores for
			// all nodes in the graph.
			scores, err := prefAttach.NodeScores(
				ctx, graph, nil, maxChanSize, nodes,
			)
			require.NoError(t1, err)

			require.Equal(t1, len(nodes), len(scores))

			// THey should all have a score, and a maxChanSize
			// channel size recommendation.
			for _, candidate := range scores {
				require.NotZero(t1, candidate.Score)
			}

			// We'll simulate a channel update by adding the nodes
			// to our set of channels.
			var chans []LocalChannel
			for _, candidate := range scores {
				chans = append(chans,
					LocalChannel{
						Node: candidate.NodeID,
					},
				)
			}

			// If we attempt to make a call to the NodeScores
			// function, without providing any new information,
			// then all nodes should have a score of zero, since we
			// already got channels to them.
			scores, err = prefAttach.NodeScores(
				ctx, graph, chans, maxChanSize, nodes,
			)
			require.NoError(t1, err)

			// Since all should be given a score of 0, the map
			// should be empty.
			require.Empty(t1, scores)
		})
		if !success {
			break
		}
	}
}

type dbTestGraph struct {
	db *graphdb.ChannelGraph
	*databaseChannelGraph
	source *sources.DBSource
}

// addRandChannel creates a new channel two target nodes. This function is
// meant to aide in the generation of random graphs for use within test cases
// the exercise the autopilot package.
func (d *dbTestGraph) addRandChannel(node1, node2 *btcec.PublicKey,
	capacity btcutil.Amount) (*ChannelEdge, *ChannelEdge, error) {

	fetchNode := func(pub *btcec.PublicKey) (sources.NodeTx, error) {
		if pub != nil {
			vertex, err := route.NewVertexFromBytes(
				pub.SerializeCompressed(),
			)
			if err != nil {
				return nil, err
			}

			dbNode, err := d.db.FetchLightningNode(vertex)
			switch {
			case errors.Is(err, graphdb.ErrGraphNodeNotFound):
				fallthrough
			case errors.Is(err, graphdb.ErrGraphNotFound):
				graphNode := &models.LightningNode{
					HaveNodeAnnouncement: true,
					Addresses: []net.Addr{
						&net.TCPAddr{
							IP: bytes.Repeat([]byte("a"), 16),
						},
					},
					Features: lnwire.NewFeatureVector(
						nil, lnwire.Features,
					),
					AuthSigBytes: testSig.Serialize(),
				}
				graphNode.AddPubKey(pub)
				if err := d.db.AddLightningNode(graphNode); err != nil {
					return nil, err
				}
			case err != nil:
				return nil, err
			}

			return newDBNodeWithTx(d.db, dbNode), nil
		}

		nodeKey, err := randKey()
		if err != nil {
			return nil, err
		}
		dbNode := &models.LightningNode{
			HaveNodeAnnouncement: true,
			Addresses: []net.Addr{
				&net.TCPAddr{
					IP: bytes.Repeat([]byte("a"), 16),
				},
			},
			Features: lnwire.NewFeatureVector(
				nil, lnwire.Features,
			),
			AuthSigBytes: testSig.Serialize(),
		}
		dbNode.AddPubKey(nodeKey)
		if err := d.db.AddLightningNode(dbNode); err != nil {
			return nil, err
		}

		return newDBNodeWithTx(d.db, dbNode), nil
	}

	vertex1, err := fetchNode(node1)
	if err != nil {
		return nil, nil, err
	}

	vertex2, err := fetchNode(node2)
	if err != nil {
		return nil, nil, err
	}

	var lnNode1, lnNode2 *btcec.PublicKey
	if bytes.Compare(vertex1.Node().PubKeyBytes[:],
		vertex2.Node().PubKeyBytes[:]) == -1 {

		lnNode1, _ = vertex1.Node().PubKey()
		lnNode2, _ = vertex2.Node().PubKey()
	} else {
		lnNode1, _ = vertex2.Node().PubKey()
		lnNode2, _ = vertex1.Node().PubKey()
	}

	chanID := randChanID()
	edge := &models.ChannelEdgeInfo{
		ChannelID: chanID.ToUint64(),
		Capacity:  capacity,
	}
	edge.AddNodeKeys(lnNode1, lnNode2, lnNode1, lnNode2)
	if err := d.db.AddChannelEdge(edge); err != nil {
		return nil, nil, err
	}
	edgePolicy := &models.ChannelEdgePolicy{
		SigBytes:                  testSig.Serialize(),
		ChannelID:                 chanID.ToUint64(),
		LastUpdate:                time.Now(),
		TimeLockDelta:             10,
		MinHTLC:                   1,
		MaxHTLC:                   lnwire.NewMSatFromSatoshis(capacity),
		FeeBaseMSat:               10,
		FeeProportionalMillionths: 10000,
		MessageFlags:              1,
		ChannelFlags:              0,
	}

	if err := d.db.UpdateEdgePolicy(edgePolicy); err != nil {
		return nil, nil, err
	}
	edgePolicy = &models.ChannelEdgePolicy{
		SigBytes:                  testSig.Serialize(),
		ChannelID:                 chanID.ToUint64(),
		LastUpdate:                time.Now(),
		TimeLockDelta:             10,
		MinHTLC:                   1,
		MaxHTLC:                   lnwire.NewMSatFromSatoshis(capacity),
		FeeBaseMSat:               10,
		FeeProportionalMillionths: 10000,
		MessageFlags:              1,
		ChannelFlags:              1,
	}
	if err := d.db.UpdateEdgePolicy(edgePolicy); err != nil {
		return nil, nil, err
	}

	return &ChannelEdge{
			ChanID:   chanID,
			Capacity: capacity,
			Peer: &dbNode{
				node: vertex1,
			},
		},
		&ChannelEdge{
			ChanID:   chanID,
			Capacity: capacity,
			Peer: &dbNode{
				node: vertex2,
			},
		},
		nil
}

func (d *dbTestGraph) addRandNode() (*btcec.PublicKey, error) {
	nodeKey, err := randKey()
	if err != nil {
		return nil, err
	}
	dbNode := &models.LightningNode{
		HaveNodeAnnouncement: true,
		Addresses: []net.Addr{
			&net.TCPAddr{
				IP: bytes.Repeat([]byte("a"), 16),
			},
		},
		Features: lnwire.NewFeatureVector(
			nil, lnwire.Features,
		),
		AuthSigBytes: testSig.Serialize(),
	}
	dbNode.AddPubKey(nodeKey)
	if err := d.db.AddLightningNode(dbNode); err != nil {
		return nil, err
	}

	return nodeKey, nil
}

type testNodeWithTx struct {
	db   *graphdb.ChannelGraph
	node *models.LightningNode
}

func newDBNodeWithTx(db *graphdb.ChannelGraph,
	node *models.LightningNode) *testNodeWithTx {

	return &testNodeWithTx{
		db:   db,
		node: node,
	}
}

func (n *testNodeWithTx) Node() *models.LightningNode {
	return n.node
}

func (n *testNodeWithTx) ForEachNodeChannel(_ context.Context,
	nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	return n.db.ForEachNodeChannelTx(nil, nodePub, func(tx kvdb.RTx,
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		return cb(info, policy, policy2)
	})
}

func (n *testNodeWithTx) FetchLightningNode(_ context.Context,
	nodePub route.Vertex) (sources.NodeTx, error) {

	node, err := n.db.FetchLightningNodeTx(nil, nodePub)
	if err != nil {
		return nil, err
	}

	return newDBNodeWithTx(n.db, node), nil
}

var _ sources.NodeTx = (*testNodeWithTx)(nil)
