package graphdb

import (
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/tor"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

var (
	testNow = time.Unix(1, 0)

	testIP4Addr = &net.TCPAddr{IP: testIP4, Port: 12345}
	testIP6Addr = &net.TCPAddr{IP: testIP6, Port: 6778}
	testTorAddr = &tor.OnionAddr{
		OnionService: "vww6ybal4bd7szmgncyruucpgfkqahzddi37ktceo3ah7ngmcopnpyyd.onion",
		Port:         9735,
	}

	testSourceNode = route.Vertex{2, 202, 1}

	testPub2 = route.Vertex{2, 202, 5}

	testChainHash = [chainhash.HashSize]byte{
		0x51, 0xb6, 0x37, 0xd8, 0xfc, 0xd2, 0xc6, 0xda,
		0x48, 0x59, 0xe6, 0x96, 0x31, 0x13, 0xa1, 0x17,
		0x2d, 0xe7, 0x93, 0xe4,
	}

	testChanPoint1 = wire.OutPoint{
		Hash:  testChainHash,
		Index: 1,
	}

	testChanPoint2 = wire.OutPoint{
		Hash:  testChainHash,
		Index: 2,
	}
)

func TestV2DB(t *testing.T) {
	testList := []struct {
		name string
		test func(t *testing.T, makeDB func(t *testing.T) GossipV2Store)
	}{
		{
			name: "Node CRUD",
			test: testNodeCRUD,
		},
		{
			name: "Channel CRUD",
			test: testChannelCRUD,
		},
	}

	// First create a shared Postgres instance so we don't spawn a new
	// docker container for each test.
	pgFixture := sqldb.NewTestPgFixture(
		t, sqldb.DefaultPostgresFixtureLifetime,
	)
	t.Cleanup(func() {
		pgFixture.TearDown(t)
	})

	makeSQLDB := func(t *testing.T, sqlite bool) GossipV2Store {
		var db *sqldb.BaseDB
		if sqlite {
			db = sqldb.NewTestSqliteDB(t).BaseDB
		} else {
			db = sqldb.NewTestPostgresDB(t, pgFixture).BaseDB
		}

		executor := sqldb.NewTransactionExecutor(
			db, func(tx *sql.Tx) SQLQueries {
				return db.WithTx(tx)
			},
		)

		testClock := clock.NewTestClock(testNow)

		return NewSQLStore(executor, testClock)
	}

	for _, test := range testList {
		t.Run(test.name+"_SQLite", func(t *testing.T) {
			test.test(t, func(t *testing.T) GossipV2Store {
				return makeSQLDB(t, true)
			})
		})

		t.Run(test.name+"_Postgres", func(t *testing.T) {
			test.test(t, func(t *testing.T) GossipV2Store {
				return makeSQLDB(t, false)
			})
		})
	}
}

func testChannelCRUD(t *testing.T, makeDB func(t *testing.T) GossipV2Store) {
	t.Parallel()

	var (
		ctx = context.Background()
		db  = makeDB(t)
	)
	// Make sure to set the source node.
	sourceNode := &models.Node2{
		PubKey: testSourceNode,
	}
	require.NoError(t, db.SetSourceNode(ctx, sourceNode))

	// Insert one of the channel peer's node announcements.
	node1 := &models.Node2{
		PubKey:      testPub,
		BlockHeight: 1000,
		Alias:       "kek",
		Features:    testFeatures,
		Addresses: []net.Addr{
			testIP4Addr, testIP6Addr,
		},
		SerialisedWireAnnouncement: []byte{1, 2, 3, 4},
	}
	require.NoError(t, db.AddNode(ctx, node1))

	chanID1 := uint64(1234)

	// Show at the correct error is returned if we query for the channel
	// before it exists in the DB.
	_, err := db.GetChannelByChanID(ctx, chanID1)
	require.ErrorIs(t, err, ErrEdgeNotFound)
	_, err = db.GetChannelByOutpoint(ctx, testChanPoint1)
	require.ErrorIs(t, err, ErrEdgeNotFound)

	// Also show that a node record for node 1 exists but not yet for node
	// 2.
	_, err = db.GetNode(ctx, testPub)
	require.NoError(t, err)
	_, err = db.GetNode(ctx, testPub2)
	require.ErrorIs(t, err, ErrGraphNodeNotFound)

	// Now, add the channel. This should also insert a shell node for the
	// second peer.
	channel1 := &models.Channel2{
		ChannelID:                  chanID1,
		Outpoint:                   testChanPoint1,
		Node1Key:                   testPub,
		Node2Key:                   testPub2,
		Features:                   testFeatures,
		Announced:                  true,
		SerialisedWireAnnouncement: []byte{1, 2, 3},
	}
	require.NoError(t, db.AddChannel(ctx, channel1))

	// Show that a record for the second node has been inserted.
	_, err = db.GetNode(ctx, testPub2)
	require.NoError(t, err)

	// Now, fetch the channel and ensure that it matches the one we
	// inserted.
	fetchedChannel, err := db.GetChannelByChanID(ctx, chanID1)
	require.NoError(t, err)
	require.Equal(t, channel1, fetchedChannel)

	// Trying to update this channel (where neither node is the source node)
	// should not be allowed.
	require.ErrorContains(t, db.AddChannel(ctx, channel1), "channel "+
		"updates only allowed for channels owned by the source node")

	// Now we will do some test for a channel that is owned by the source
	// node.
	chanID2 := uint64(5556)
	channel2 := &models.Channel2{
		ChannelID: chanID2,
		Outpoint:  testChanPoint2,
		Node1Key:  testPub,
		Node2Key:  testSourceNode,
		Features:  testFeatures,
		Announced: false,
	}
	require.NoError(t, db.AddChannel(ctx, channel2))

	// Fetch the channel and check that it matches the one we inserted.
	fetchedChannel, err = db.GetChannelByChanID(ctx, chanID2)
	require.NoError(t, err)
	require.Equal(t, channel2, fetchedChannel)

	// Now, update some of the channel fields and insert it again. This
	// should he allowed sine the source node is one of the channel owners.
	channel2.Announced = true
	channel2.SerialisedWireAnnouncement = []byte{1, 2, 3}
	require.NoError(t, db.AddChannel(ctx, channel2))

	// Check that the channel was updated as expected.
	fetchedChannel, err = db.GetChannelByChanID(ctx, chanID2)
	require.NoError(t, err)
	require.Equal(t, channel2, fetchedChannel)

	// Delete the added channels and assert that they can no longer be
	// found.
	require.NoError(t, db.DeleteChannels(ctx, chanID1, chanID2))

	_, err = db.GetChannelByChanID(ctx, chanID1)
	require.ErrorIs(t, err, ErrEdgeNotFound)
	_, err = db.GetChannelByChanID(ctx, chanID2)
	require.ErrorIs(t, err, ErrEdgeNotFound)
}

func testNodeCRUD(t *testing.T, makeDB func(t *testing.T) GossipV2Store) {
	t.Parallel()

	var (
		ctx = context.Background()
		db  = makeDB(t)
	)

	// Create a test node to insert.
	node := &models.Node2{
		PubKey:      testPub,
		BlockHeight: 1000,
		Alias:       "kek",
		Features:    testFeatures,
		Addresses: []net.Addr{
			testIP4Addr, testIP6Addr,
		},
		SerialisedWireAnnouncement: []byte{1, 2, 3, 4},
	}

	// Try to fetch a node that doesn't exist yet.
	_, err := db.GetNode(ctx, testPub)
	require.ErrorIs(t, err, ErrGraphNodeNotFound)

	// HasNode should return false for a node that doesn't exist yet.
	_, hasNode, err := db.HasNode(ctx, testPub)
	require.NoError(t, err)
	require.False(t, hasNode)

	// Now, insert the node.
	require.NoError(t, db.AddNode(ctx, node))

	// HasNode should now return true with the updated block height.
	lastUpdatedBlock, hasNode, err := db.HasNode(ctx, testPub)
	require.NoError(t, err)
	require.True(t, hasNode)
	require.Equal(t, node.BlockHeight, lastUpdatedBlock)

	// Fetch the node and ensure that it matches the one we inserted.
	fetchedNode, err := db.GetNode(ctx, testPub)
	require.NoError(t, err)
	assertNodesEqual(t, node, fetchedNode)

	// Update the node such that one of its address is removed and another
	// one added. We also update its features and its last updated block.
	node.Addresses = []net.Addr{testIP6Addr, testTorAddr}
	node.Features = lnwire.NewFeatureVector(
		lnwire.NewRawFeatureVector(lnwire.AnchorsRequired),
		lnwire.Features,
	)
	node.BlockHeight++
	require.NoError(t, db.AddNode(ctx, node))

	// Fetch the node and ensure that the addresses and features were
	// updated as expected.
	fetchedNode, err = db.GetNode(ctx, testPub)
	require.NoError(t, err)
	assertNodesEqual(t, node, fetchedNode)

	// Ensure that the updated block height is returned by HasNode.
	lastUpdatedBlock, hasNode, err = db.HasNode(ctx, testPub)
	require.NoError(t, err)
	require.True(t, hasNode)
	require.Equal(t, node.BlockHeight, lastUpdatedBlock)

	// Test alias lookup.
	alias, err := db.LookupAlias(ctx, testPub)
	require.NoError(t, err)
	require.Equal(t, node.Alias, alias)

	// Delete the node and ensure that it's no longer found.
	require.NoError(t, db.DeleteNode(ctx, testPub))
	err = db.DeleteNode(ctx, testPub)
	require.ErrorIs(t, err, ErrGraphNodeNotFound)

	// HasNode should now return false.
	_, hasNode, err = db.HasNode(ctx, testPub)
	require.NoError(t, err)
	require.False(t, hasNode)

	// We also test adding a partial/shell node for which we have not yet
	// received an announcement.
	shellNode := &models.Node2{
		PubKey: testPub,
	}
	err = db.AddNode(ctx, shellNode)
	require.NoError(t, err)

	_, hasNode, err = db.HasNode(ctx, testPub)
	require.NoError(t, err)
	require.True(t, hasNode)

	fetchedNode, err = db.GetNode(ctx, testPub)
	require.NoError(t, err)
	assertNodesEqual(t, shellNode, fetchedNode)

	// Delete the node once more and ensure that it's no longer found.
	require.NoError(t, db.DeleteNode(ctx, testPub))
	err = db.DeleteNode(ctx, testPub)
	require.ErrorIs(t, err, ErrGraphNodeNotFound)

	// We'll also test the setting of the source node.
	require.NoError(t, db.SetSourceNode(ctx, node))
	sourceNode, err := db.GetSourceNode(ctx)
	require.NoError(t, err)
	assertNodesEqual(t, node, sourceNode)

	// Setting the source node again should be ok as long as the public
	// key of the node is the same. We'll update some node fields to
	// ensure that the rest works as expected.
	node.Addresses = []net.Addr{testTorAddr}
	require.NoError(t, db.SetSourceNode(ctx, node))

	sourceNode, err = db.GetSourceNode(ctx)
	require.NoError(t, err)
	assertNodesEqual(t, node, sourceNode)
}

func assertNodesEqual(t *testing.T, n1, n2 *models.Node2) {
	if n1.Features == nil {
		n1.Features = lnwire.EmptyFeatureVector()
	}
	if n2.Features == nil {
		n2.Features = lnwire.EmptyFeatureVector()
	}

	require.ElementsMatch(t, n1.Addresses, n2.Addresses)
	n1.Addresses = nil
	n2.Addresses = nil
	require.Equal(t, n1, n2)
}
