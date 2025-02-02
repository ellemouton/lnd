package graphdb

import (
	"context"
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/lightningnetwork/lnd/tor"
	"github.com/stretchr/testify/require"
)

var (
	testNow = time.Unix(1, 0)

	testIP4Addr = &net.TCPAddr{IP: testIP4, Port: 12345}
	testIP6Addr = &net.TCPAddr{IP: testIP6, Port: 6778}
	testTorAddr = &tor.OnionAddr{
		OnionService: "vww6ybal4bd7szmgncyruucpgfkqahzddi37ktceo3ah7ngmcopnpyyd.onion",
		Port:         9735,
	}

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

	testPub2 = route.Vertex{2, 202, 5}
)

func TestV2Store(t *testing.T) {
	testList := []struct {
		name string
		test func(t *testing.T, makeDB func(t *testing.T,
			clock clock.Clock) GossipV2Store)
	}{
		{
			name: "Node CRUD",
			test: testNodeCRUD,
		},
		{
			name: "Channel CRUD",
			test: testChannelCRUD,
		},
		{
			name: "Channel Policy CRUD",
			test: testChannelPolicyCRUD,
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

	makeSQLDB := func(t *testing.T, clock clock.Clock,
		sqlite bool) GossipV2Store {

		var db *sqldb.BaseDB
		if sqlite {
			db = sqldb.NewTestSqliteDB(t).BaseDB
		} else {
			db = sqldb.NewTestPostgresDB(t, pgFixture).BaseDB
		}

		executor := sqldb.NewTransactionExecutor(
			db, func(tx *sql.Tx) V2Queries {
				return db.WithTx(tx)
			},
		)

		return NewV2Store(executor, clock)
	}

	for _, test := range testList {
		t.Run(test.name+"_SQLite", func(t *testing.T) {
			test.test(t, func(t *testing.T,
				clock clock.Clock) GossipV2Store {

				return makeSQLDB(t, clock, true)
			})
		})

		t.Run(test.name+"_Postgres", func(t *testing.T) {
			test.test(t, func(t *testing.T,
				clock clock.Clock) GossipV2Store {

				return makeSQLDB(t, clock, false)
			})
		})
	}
}

func testChannelCRUD(t *testing.T, makeDB func(t *testing.T,
	clock clock.Clock) GossipV2Store) {

	t.Parallel()

	var (
		ctx   = context.Background()
		clock = clock.NewTestClock(testNow)
		db    = makeDB(t, clock)
	)

	// Insert one of the channel peer's node announcements.
	node1 := &models.Node2{
		PubKey:      testPub,
		BlockHeight: 1000,
		Alias:       fn.Some[string]("kek"),
		Features:    testFeatures,
		Addresses: []net.Addr{
			testIP4Addr, testIP6Addr,
		},
		Signature: fn.Some([]byte("sig")),
	}
	require.NoError(t, db.AddNode(ctx, node1))

	// At this point, the node should not be seen as public since it
	// doesn't have any channels yet.
	isPublic, err := db.IsNodePublic(ctx, testPub)
	require.NoError(t, err)
	require.False(t, isPublic)

	chanID1 := uint64(1234)

	// Show at the correct error is returned if we query for the channel
	// before it exists in the DB.
	_, _, _, err = db.GetChannelByChanID(ctx, chanID1)
	require.ErrorIs(t, err, ErrEdgeNotFound)
	_, _, _, err = db.GetChannelByOutpoint(ctx, testChanPoint1)
	require.ErrorIs(t, err, ErrEdgeNotFound)
	_, _, known, zombie, err := db.HasChannel(ctx, chanID1)
	require.NoError(t, err)
	require.False(t, known)
	require.False(t, zombie)

	// Also show that a node record for node 1 exists but not yet for node
	// 2.
	_, err = db.GetNode(ctx, testPub)
	require.NoError(t, err)
	_, err = db.GetNode(ctx, testPub2)
	require.ErrorIs(t, err, ErrGraphNodeNotFound)

	// Now, add the channel. This should also insert a shell node for the
	// second peer.
	channel1 := &models.Channel2{
		ChannelID: chanID1,
		Outpoint:  testChanPoint1,
		Node1Key:  testPub,
		Node2Key:  testPub2,
		Features:  testFeatures,
	}
	require.NoError(t, db.AddChannel(ctx, channel1))

	// Show that a record for the second node has been inserted.
	_, err = db.GetNode(ctx, testPub2)
	require.NoError(t, err)

	// HasChannel should also now reflect the channel.
	_, _, known, zombie, err = db.HasChannel(ctx, chanID1)
	require.NoError(t, err)
	require.True(t, known)
	require.False(t, zombie)

	// Now, fetch the channel and ensure that it matches the one we
	// inserted.
	fetchedChannel, _, _, err := db.GetChannelByChanID(ctx, chanID1)
	require.NoError(t, err)
	assertChannelsEqual(t, channel1, fetchedChannel)

	// Before we add the proof, show that node 1 is still not public since
	// it does not have an announced channel.
	isPublic, err = db.IsNodePublic(ctx, testPub)
	require.NoError(t, err)
	require.False(t, isPublic)

	// Now add the channel proof.
	err = db.UpdateAnnouncedChannel(
		ctx, chanID1, []byte("sig"), map[uint64][]byte{
			1: []byte("custom field"),
		},
	)
	require.NoError(t, err)

	// Update our in-memory expected channel.
	channel1.Signature = fn.Some([]byte("sig"))
	channel1.ExtraSignedFields = map[uint64][]byte{
		1: []byte("custom field"),
	}

	// Fetch the channel and ensure that the proof was updated as expected.
	fetchedChannel, _, _, err = db.GetChannelByChanID(ctx, chanID1)
	require.NoError(t, err)
	assertChannelsEqual(t, channel1, fetchedChannel)

	// Try to once again update the channel proof, this should not be
	// allowed since the channel signature has already been set.
	err = db.UpdateAnnouncedChannel(
		ctx, chanID1, []byte("sig"), map[uint64][]byte{
			1: []byte("custom field"),
		},
	)
	require.ErrorContains(t, err, "already has an announcement")

	// Now, show that node 1 is now public since it has an announced
	// channel.
	isPublic, err = db.IsNodePublic(ctx, testPub)
	require.NoError(t, err)
	require.True(t, isPublic)

	// Attempting to update a channel that doesn't exist yet should also
	// fail.
	err = db.UpdateAnnouncedChannel(
		ctx, 12345, []byte("sig"), map[uint64][]byte{
			1: []byte("custom field"),
		},
	)
	require.ErrorIs(t, err, ErrEdgeNotFound)

	// Listing a node's channels should also work.
	channels, err := db.ListNodeChannels(ctx, testPub)
	require.NoError(t, err)
	require.Len(t, channels, 1)
	assertChannelsEqual(t, channel1, channels[0])

	// Delete the added channel and assert that it can no longer be found.
	require.NoError(t, db.DeleteChannels(ctx, false, true, chanID1))

	_, _, _, err = db.GetChannelByChanID(ctx, chanID1)
	require.ErrorIs(t, err, ErrEdgeNotFound)

	// The node is no longer public.
	isPublic, err = db.IsNodePublic(ctx, testPub)
	require.NoError(t, err)
	require.False(t, isPublic)

	// HasChannel should reflect that the channel is unknown and a zombie.
	_, _, known, zombie, err = db.HasChannel(ctx, chanID1)
	require.NoError(t, err)
	require.False(t, known)
	require.True(t, zombie)
}

func testNodeCRUD(t *testing.T, makeDB func(t *testing.T,
	clock clock.Clock) GossipV2Store) {

	t.Parallel()

	var (
		ctx   = context.Background()
		clock = clock.NewTestClock(testNow)
		db    = makeDB(t, clock)
	)

	// Create a test node to insert.
	node := &models.Node2{
		PubKey:      testPub,
		BlockHeight: 1000,
		Alias:       fn.Some[string]("kek"),
		Features:    testFeatures,
		Addresses: []net.Addr{
			testIP4Addr, testIP6Addr,
		},
		ExtraSignedFields: map[uint64][]byte{
			1: []byte("custom field"),
			2: []byte("spv proof"),
		},
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
	node.Signature = fn.Some[[]byte]([]byte("sig"))
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
	alias, err := db.LookupNodeAlias(ctx, testPub)
	require.NoError(t, err)
	require.True(t, node.Alias.IsSome())
	node.Alias.WhenSome(func(a string) {
		require.Equal(t, a, alias)
	})

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

func testChannelPolicyCRUD(t *testing.T,
	makeDB func(t *testing.T, clock clock.Clock) GossipV2Store) {

	t.Parallel()

	var (
		ctx = context.Background()
		db  = makeDB(t, clock.NewTestClock(testNow))
	)

	// Prepare a channel and channel policies for it. Don't insert them yet.
	chanID1 := uint64(1234)
	channel1 := &models.Channel2{
		ChannelID: chanID1,
		Outpoint:  testChanPoint1,
		Node1Key:  testPub,
		Node2Key:  testPub2,
		Features:  testFeatures,
	}
	chan1P1 := &models.ChannelPolicy2{
		ChannelID:                 chanID1,
		BlockHeight:               22222,
		TimeLockDelta:             20,
		MinHTLC:                   1,
		MaxHTLC:                   3,
		FeeBaseMSat:               56,
		FeeProportionalMillionths: 66,
		SecondPeer:                false,
		ToNode:                    testPub2,
		Flags:                     lnwire.ChanUpdateDisableIncoming,
		Signature:                 fn.Some([]byte{1, 2, 3, 4}),
	}
	chan1P2 := &models.ChannelPolicy2{
		ChannelID:                 chanID1,
		BlockHeight:               6633,
		TimeLockDelta:             1,
		MinHTLC:                   1,
		MaxHTLC:                   5,
		FeeBaseMSat:               66,
		FeeProportionalMillionths: 99,
		SecondPeer:                true,
		ToNode:                    testPub,
		Flags:                     lnwire.ChanUpdateDisableIncoming,
	}

	// Attempting to insert a channel policy for a channel that doesn't
	// exist should fail.
	err := db.UpdateChannelPolicy(ctx, chan1P1)
	require.ErrorIs(t, err, ErrEdgeNotFound)

	// Now insert the channel and the first channel policy.
	require.NoError(t, db.AddChannel(ctx, channel1))
	require.NoError(t, db.UpdateChannelPolicy(ctx, chan1P1))

	// Fetching the channel info should return the channel, the first
	// channel policy and nil for the second channel policy.
	fetchedChannel, p1, p2, err := db.GetChannelByChanID(ctx, chanID1)
	require.NoError(t, err)
	assertChannelsEqual(t, channel1, fetchedChannel)
	assertChannelPoliciesEqual(t, chan1P1, p1)
	require.Nil(t, p2)

	// Now insert the second channel policy.
	require.NoError(t, db.UpdateChannelPolicy(ctx, chan1P2))

	// Fetching the channel info should return the channel, the first
	// channel policy and the second channel policy.
	fetchedChannel, p1, p2, err = db.GetChannelByChanID(ctx, chanID1)
	require.NoError(t, err)
	assertChannelsEqual(t, channel1, fetchedChannel)
	assertChannelPoliciesEqual(t, chan1P1, p1)
	assertChannelPoliciesEqual(t, chan1P2, p2)

	// Now, update the first channel policy.
	chan1P1.TimeLockDelta = 30
	chan1P1.MinHTLC = 2
	chan1P1.MaxHTLC = 4
	chan1P1.Flags = 0
	chan1P1.ExtraSignedFields = map[uint64][]byte{
		1: []byte("custom field"),
	}
	chan1P1.Signature = fn.Some([]byte{4, 3, 2, 1})

	require.NoError(t, db.UpdateChannelPolicy(ctx, chan1P1))

	fetchedChannel, p1, p2, err = db.GetChannelByChanID(ctx, chanID1)
	require.NoError(t, err)
	assertChannelsEqual(t, channel1, fetchedChannel)
	assertChannelPoliciesEqual(t, chan1P1, p1)
	assertChannelPoliciesEqual(t, chan1P2, p2)
}

func assertNodesEqual(t *testing.T, n1, n2 *models.Node2) {
	if n1.Features == nil {
		n1.Features = lnwire.EmptyFeatureVector()
	}
	if n2.Features == nil {
		n2.Features = lnwire.EmptyFeatureVector()
	}

	if n1.ExtraSignedFields == nil {
		n1.ExtraSignedFields = make(map[uint64][]byte)
	}
	if n2.ExtraSignedFields == nil {
		n2.ExtraSignedFields = make(map[uint64][]byte)
	}

	require.ElementsMatch(t, n1.Addresses, n2.Addresses)
	n1.Addresses = nil
	n2.Addresses = nil
	require.Equal(t, n1, n2)
}

func assertChannelsEqual(t *testing.T, n1, n2 *models.Channel2) {
	if n1.Features == nil {
		n1.Features = lnwire.EmptyFeatureVector()
	}
	if n2.Features == nil {
		n2.Features = lnwire.EmptyFeatureVector()
	}

	if n1.ExtraSignedFields == nil {
		n1.ExtraSignedFields = make(map[uint64][]byte)
	}
	if n2.ExtraSignedFields == nil {
		n2.ExtraSignedFields = make(map[uint64][]byte)
	}

	require.Equal(t, n1, n2)
}

func assertChannelPoliciesEqual(t *testing.T, n1, n2 *models.ChannelPolicy2) {
	if n1.ExtraSignedFields == nil {
		n1.ExtraSignedFields = make(map[uint64][]byte)
	}
	if n2.ExtraSignedFields == nil {
		n2.ExtraSignedFields = make(map[uint64][]byte)
	}

	require.Equal(t, n1, n2)
}
