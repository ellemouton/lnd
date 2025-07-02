//go:build test_db_postgres || test_db_sqlite

package graphdb

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"image/color"
	prand "math/rand"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btclog/v2"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

var (
	testChain         = *chaincfg.MainNetParams.GenesisHash
	testColor         = color.RGBA{R: 1, G: 2, B: 3}
	testTime          = time.Unix(11111, 0)
	testSigBytes      = testSig.Serialize()
	testExtraData     = []byte{1, 1, 1, 2, 2, 2, 2}
	testEmptyFeatures = lnwire.EmptyFeatureVector()
	testAuthProof     = &models.ChannelAuthProof{
		NodeSig1Bytes:    testSig.Serialize(),
		NodeSig2Bytes:    testSig.Serialize(),
		BitcoinSig1Bytes: testSig.Serialize(),
		BitcoinSig2Bytes: testSig.Serialize(),
	}
)

// TestMigrateGraphToSQL tests various deterministic cases that we want to test
// for to ensure that our migration from a graph store backed by a KV DB to a
// SQL database works as expected. At the end of each test, the DBs are compared
// and expected to have the exact same data in them.
func TestMigrateGraphToSQL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	writeUpdate := func(t *testing.T, db *KVStore, object any) {
		t.Helper()

		var err error
		switch obj := object.(type) {
		case *models.LightningNode:
			err = db.AddLightningNode(ctx, obj)
		case *models.ChannelEdgeInfo:
			err = db.AddChannelEdge(ctx, obj)
		case *models.ChannelEdgePolicy:
			_, _, err = db.UpdateEdgePolicy(ctx, obj)
		default:
			err = fmt.Errorf("unhandled object type: %T", obj)
		}
		require.NoError(t, err)
	}

	var (
		chanID1 = prand.Uint64()
		chanID2 = prand.Uint64()

		node1 = genPubKey(t)
		node2 = genPubKey(t)
	)

	tests := []struct {
		name          string
		write         func(t *testing.T, db *KVStore, object any)
		objects       []any
		expGraphStats graphStats
	}{
		{
			name: "empty",
		},
		{
			name:  "nodes",
			write: writeUpdate,
			//nolint:ll
			objects: []any{
				// Normal node with all fields.
				makeTestNode(t),
				// A node with no node announcement.
				makeTestShellNode(t),
				// A node with an announcement but no addresses.
				makeTestNode(t, func(n *models.LightningNode) {
					n.Addresses = nil
				}),
				// A node with all types of addresses.
				makeTestNode(t, func(n *models.LightningNode) {
					n.Addresses = []net.Addr{
						testAddr,
						testIPV4Addr,
						testIPV6Addr,
						anotherAddr,
						testOnionV2Addr,
						testOnionV3Addr,
						testOpaqueAddr,
					}
				}),
				// No extra opaque data.
				makeTestNode(t, func(n *models.LightningNode) {
					n.ExtraOpaqueData = nil
				}),
				// A node with no features.
				makeTestNode(t, func(n *models.LightningNode) {
					n.Features = lnwire.EmptyFeatureVector()
				}),
			},
			expGraphStats: graphStats{
				numNodes: 6,
			},
		},
		{
			name: "source node",
			write: func(t *testing.T, db *KVStore, object any) {
				node, ok := object.(*models.LightningNode)
				require.True(t, ok)

				err := db.SetSourceNode(ctx, node)
				require.NoError(t, err)
			},
			objects: []any{
				makeTestNode(t),
			},
			expGraphStats: graphStats{
				numNodes:   1,
				srcNodeSet: true,
			},
		},
		{
			name:  "channels and policies",
			write: writeUpdate,
			objects: []any{
				// A channel with unknown nodes. This will
				// result in two shell nodes being created.
				// 	- channel count += 1
				// 	- node count += 2
				makeTestChannel(t),

				// Insert some nodes.
				// 	- node count += 1
				makeTestNode(t, func(n *models.LightningNode) {
					n.PubKeyBytes = node1
				}),
				// 	- node count += 1
				makeTestNode(t, func(n *models.LightningNode) {
					n.PubKeyBytes = node2
				}),

				// A channel with known nodes.
				// - channel count += 1
				makeTestChannel(
					t, func(c *models.ChannelEdgeInfo) {
						c.ChannelID = chanID1

						c.NodeKey1Bytes = node1
						c.NodeKey2Bytes = node2
					},
				),

				// Insert a channel with no auth proof, no
				// extra opaque data, and empty features.
				// Use known nodes.
				// 	- channel count += 1
				makeTestChannel(
					t, func(c *models.ChannelEdgeInfo) {
						c.ChannelID = chanID2

						c.NodeKey1Bytes = node1
						c.NodeKey2Bytes = node2

						c.AuthProof = nil
						c.ExtraOpaqueData = nil
						c.Features = testEmptyFeatures
					},
				),

				// Now, insert a single update for the
				// first channel.
				// 	- channel policy count += 1
				makeTestPolicy(chanID1, node1),

				// Insert two updates for the second
				// channel, one for each direction.
				// 	- channel policy count += 1
				makeTestPolicy(
					chanID2, node1,
					func(p *models.ChannelEdgePolicy) {
						p.ChannelFlags = 0
					},
				),
				// This one also has no extra opaque data.
				// 	- channel policy count += 1
				makeTestPolicy(
					chanID2, node2,
					func(p *models.ChannelEdgePolicy) {
						p.ChannelFlags = 1
						p.ExtraOpaqueData = nil
					},
				),
			},
			expGraphStats: graphStats{
				numNodes:    4,
				numChannels: 3,
				numPolicies: 3,
			},
		},
		{
			name: "prune log",
			write: func(t *testing.T, db *KVStore, object any) {
				var hash chainhash.Hash
				_, err := rand.Read(hash[:])
				require.NoError(t, err)

				switch obj := object.(type) {
				case *models.LightningNode:
					err = db.SetSourceNode(ctx, obj)
				default:
					height, ok := obj.(uint32)
					require.True(t, ok)

					_, _, err = db.PruneGraph(
						nil, &hash, height,
					)
				}
				require.NoError(t, err)
			},
			objects: []any{
				// The PruneGraph call requires that the source
				// node be set. So that is the first object
				// we will write.
				&models.LightningNode{
					HaveNodeAnnouncement: false,
					PubKeyBytes:          testPub,
				},
				// Now we add some block heights to prune
				// the graph at.
				uint32(1), uint32(2), uint32(20), uint32(3),
				uint32(4),
			},
			expGraphStats: graphStats{
				numNodes:   1,
				srcNodeSet: true,
				pruneTip:   20,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Set up our source kvdb DB.
			kvDB := setUpKVStore(t)

			// Write the test objects to the kvdb store.
			for _, object := range test.objects {
				test.write(t, kvDB, object)
			}

			// Set up our destination SQL DB.
			sql, ok := NewTestDB(t).(*SQLStore)
			require.True(t, ok)

			// Run the migration.
			err := MigrateGraphToSQL(
				ctx, kvDB.db, sql.db, testChain,
			)
			require.NoError(t, err)

			// Validate that the two databases are now in sync.
			assertInSync(t, kvDB, sql, test.expGraphStats)
		})
	}
}

// graphStats holds expected statistics about the graph after migration.
type graphStats struct {
	numNodes    int
	srcNodeSet  bool
	numChannels int
	numPolicies int
	pruneTip    int
}

// assertInSync checks that the KVStore and SQLStore both contain the same
// graph data after migration.
func assertInSync(t *testing.T, kvDB *KVStore, sqlDB *SQLStore,
	stats graphStats) {

	// 1) Compare the nodes in the two stores.
	sqlNodes := fetchAllNodes(t, sqlDB)
	require.Len(t, sqlNodes, stats.numNodes)
	require.Equal(t, fetchAllNodes(t, kvDB), sqlNodes)

	// 2) Check that the source nodes match (if indeed source nodes have
	// been set).
	sqlSourceNode := fetchSourceNode(t, sqlDB)
	require.Equal(t, stats.srcNodeSet, sqlSourceNode != nil)
	require.Equal(t, fetchSourceNode(t, kvDB), sqlSourceNode)

	// 3) Compare the channels and policies in the two stores.
	sqlChannels := fetchAllChannelsAndPolicies(t, sqlDB)
	require.Len(t, sqlChannels, stats.numChannels)
	require.Equal(t, stats.numPolicies, sqlChannels.CountPolicies())
	require.Equal(t, fetchAllChannelsAndPolicies(t, kvDB), sqlChannels)

	// 4) Assert prune logs match. For this one, we iterate through the
	// prune log of the kvdb store and check that the entries match the
	// entries in the SQL store. Then we just do a final check to ensure
	// that the prune tip also matches.
	checkKVPruneLogEntries(t, kvDB, sqlDB, stats.pruneTip)
}

// fetchAllNodes retrieves all nodes from the given store and returns them
// sorted by their public key.
func fetchAllNodes(t *testing.T, store V1Store) []*models.LightningNode {
	nodes := make([]*models.LightningNode, 0)

	err := store.ForEachNode(func(tx NodeRTx) error {
		nodes = append(nodes, tx.Node())

		return nil
	})
	require.NoError(t, err)

	// Sort the nodes by their public key to ensure a consistent order.
	sort.Slice(nodes, func(i, j int) bool {
		return bytes.Compare(
			nodes[i].PubKeyBytes[:],
			nodes[j].PubKeyBytes[:],
		) < 0
	})

	return nodes
}

// fetchSourceNode retrieves the source node from the given store.
func fetchSourceNode(t *testing.T, store V1Store) *models.LightningNode {
	node, err := store.SourceNode(context.Background())
	if errors.Is(err, ErrSourceNodeNotSet) {
		return nil
	} else {
		require.NoError(t, err)
	}

	return node
}

// chanInfo holds information about a channel, including its edge info
// and the policies for both directions.
type chanInfo struct {
	edgeInfo *models.ChannelEdgeInfo
	policy1  *models.ChannelEdgePolicy
	policy2  *models.ChannelEdgePolicy
}

// chanSet is a slice of chanInfo
type chanSet []chanInfo

// CountPolicies counts the total number of policies in the channel set.
func (c chanSet) CountPolicies() int {
	var count int
	for _, info := range c {
		if info.policy1 != nil {
			count++
		}
		if info.policy2 != nil {
			count++
		}
	}
	return count
}

// fetchAllChannelsAndPolicies retrieves all channels and their policies
// from the given store and returns them sorted by their channel ID.
func fetchAllChannelsAndPolicies(t *testing.T, store V1Store) chanSet {
	channels := make(chanSet, 0)
	err := store.ForEachChannel(func(info *models.ChannelEdgeInfo,
		p1 *models.ChannelEdgePolicy,
		p2 *models.ChannelEdgePolicy) error {

		if len(info.ExtraOpaqueData) == 0 {
			info.ExtraOpaqueData = nil
		}
		if p1 != nil && len(p1.ExtraOpaqueData) == 0 {
			p1.ExtraOpaqueData = nil
		}
		if p2 != nil && len(p2.ExtraOpaqueData) == 0 {
			p2.ExtraOpaqueData = nil
		}

		channels = append(channels, chanInfo{
			edgeInfo: info,
			policy1:  p1,
			policy2:  p2,
		})

		return nil
	})
	require.NoError(t, err)

	// Sort the channels by their channel ID to ensure a consistent order.
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].edgeInfo.ChannelID <
			channels[j].edgeInfo.ChannelID
	})

	return channels
}

// checkKVPruneLogEntries iterates through the prune log entries in the
// KVStore and checks that there is an entry for each in the SQLStore. It then
// does a final check to ensure that the prune tips in both stores match.
func checkKVPruneLogEntries(t *testing.T, kv *KVStore, sql *SQLStore,
	expTip int) {

	// Iterate through the prune log entries in the KVStore and
	// check that each entry exists in the SQLStore.
	err := forEachPruneLogEntry(kv.db,
		func(height uint32, hash *chainhash.Hash) error {
			sqlHash, err := sql.db.GetPruneHashByHeight(
				context.Background(), int64(height),
			)
			require.NoError(t, err)
			require.Equal(t, hash[:], sqlHash)

			return nil
		},
	)
	require.NoError(t, err)

	kvPruneHash, kvPruneHeight, kvPruneErr := kv.PruneTip()
	sqlPruneHash, sqlPruneHeight, sqlPruneErr := sql.PruneTip()

	// If the prune error is ErrGraphNeverPruned, then we expect
	// the SQL prune error to also be ErrGraphNeverPruned.
	if errors.Is(kvPruneErr, ErrGraphNeverPruned) {
		require.ErrorIs(t, sqlPruneErr, ErrGraphNeverPruned)
		return
	}

	// Otherwise, we expect both prune errors to be nil and the
	// prune hashes and heights to match.
	require.NoError(t, kvPruneErr)
	require.NoError(t, sqlPruneErr)
	require.Equal(t, kvPruneHash[:], sqlPruneHash[:])
	require.Equal(t, kvPruneHeight, sqlPruneHeight)
	require.Equal(t, expTip, int(sqlPruneHeight))
}

// setUpKVStore initializes a new KVStore for testing.
func setUpKVStore(t *testing.T) *KVStore {
	kvDB, cleanup, err := kvdb.GetTestBackend(t.TempDir(), "graph")
	require.NoError(t, err)
	t.Cleanup(cleanup)

	kvStore, err := NewKVStore(kvDB)
	require.NoError(t, err)

	return kvStore
}

// genPubKey generates a new public key for testing purposes.
func genPubKey(t *testing.T) route.Vertex {
	key, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	var pub route.Vertex
	copy(pub[:], key.PubKey().SerializeCompressed())

	return pub
}

// testNodeOpt defines a functional option type that can be used to
// modify the attributes of a models.LightningNode crated by makeTestNode.
type testNodeOpt func(*models.LightningNode)

// makeTestNode can be used to create a test models.LightningNode. The
// functional options can be used to modify the node's attributes.
func makeTestNode(t *testing.T, opts ...testNodeOpt) *models.LightningNode {
	n := &models.LightningNode{
		HaveNodeAnnouncement: true,
		AuthSigBytes:         testSigBytes,
		LastUpdate:           testTime,
		Color:                testColor,
		Alias:                "kek",
		Features:             testFeatures,
		Addresses:            testAddrs,
		ExtraOpaqueData:      testExtraData,
		PubKeyBytes:          genPubKey(t),
	}

	for _, opt := range opts {
		opt(n)
	}

	return n
}

// makeTestShellNode creates a minimal models.LightningNode
// that only contains the public key and no other attributes.
func makeTestShellNode(t *testing.T) *models.LightningNode {
	return &models.LightningNode{
		HaveNodeAnnouncement: false,
		PubKeyBytes:          genPubKey(t),
	}
}

// modify the attributes of a models.ChannelEdgeInfo created by makeTestChannel.
type testChanOpt func(info *models.ChannelEdgeInfo)

// makeTestChannel creates a test models.ChannelEdgeInfo. The functional options
// can be used to modify the channel's attributes.
func makeTestChannel(t *testing.T,
	opts ...testChanOpt) *models.ChannelEdgeInfo {

	c := &models.ChannelEdgeInfo{
		ChannelID:        prand.Uint64(),
		ChainHash:        testChain,
		NodeKey1Bytes:    genPubKey(t),
		NodeKey2Bytes:    genPubKey(t),
		BitcoinKey1Bytes: genPubKey(t),
		BitcoinKey2Bytes: genPubKey(t),
		Features:         testFeatures,
		AuthProof:        testAuthProof,
		ChannelPoint: wire.OutPoint{
			Hash:  rev,
			Index: prand.Uint32(),
		},
		Capacity:        10000,
		ExtraOpaqueData: testExtraData,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// testPolicyOpt defines a functional option type that can be used to modify the
// attributes of a models.ChannelEdgePolicy created by makeTestPolicy.
type testPolicyOpt func(*models.ChannelEdgePolicy)

// makeTestPolicy creates a test models.ChannelEdgePolicy. The functional
// options can be used to modify the policy's attributes.
func makeTestPolicy(chanID uint64,
	toNode route.Vertex, opts ...testPolicyOpt) *models.ChannelEdgePolicy {

	p := &models.ChannelEdgePolicy{
		SigBytes:                  testSigBytes,
		ChannelID:                 chanID,
		LastUpdate:                nextUpdateTime(),
		MessageFlags:              1,
		ChannelFlags:              1,
		TimeLockDelta:             99,
		MinHTLC:                   2342135,
		MaxHTLC:                   13928598,
		FeeBaseMSat:               4352345,
		FeeProportionalMillionths: 90392423,
		ToNode:                    toNode,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// TestMigrationWithChannelDB tests the migration of the graph store from a
// bolt backed channel.db or a kvdb channel.sqlite to a SQL database. Note that
// this test does not attempt to be a complete migration test for all graph
// store types but rather is added as a tool for developers and users to debug
// graph migration issues with an actual channel.db/channel.sqlite file.
//
// NOTE: To use this test, place either of those files in the graph/db/testdata
// directory, uncomment the "Skipf" line, and set "chain" variable appropriately
// and set the "fileName" variable to the name of the channel database file you
// want to use for the migration test.
func TestMigrationWithChannelDB(t *testing.T) {
	ctx := context.Background()

	// NOTE: comment this line out to run the test.
	t.Skipf("skipping test meant for local debugging only")

	// NOTE: set this to the genesis hash of the chain that the store
	// was created on.
	chain := *chaincfg.MainNetParams.GenesisHash

	// NOTE: set this to the name of the channel database file you want
	// to use for the migration test. This may be either a bbolt ".db" file
	// or a SQLite ".sqlite" file. If you want to migrate from a
	// bbolt channel.db file, set this to "channel.db".
	const fileName = "channel.sqlite"

	// Set up logging for the test.
	UseLogger(btclog.NewSLogger(btclog.NewDefaultHandler(os.Stdout)))

	// migrate runs the migration from the kvdb store to the SQL store.
	migrate := func(t *testing.T, kvBackend kvdb.Backend) {
		graphStore := newBatchQuerier(t)

		err := graphStore.ExecTx(
			ctx, sqldb.WriteTxOpt(), func(tx SQLQueries) error {
				return MigrateGraphToSQL(
					ctx, kvBackend, tx, chain,
				)
			}, sqldb.NoOpReset,
		)
		require.NoError(t, err)
	}

	connectBBolt := func(t *testing.T, dbPath string) kvdb.Backend {
		cfg := &kvdb.BoltBackendConfig{
			DBPath:            dbPath,
			DBFileName:        fileName,
			NoFreelistSync:    true,
			AutoCompact:       false,
			AutoCompactMinAge: kvdb.DefaultBoltAutoCompactMinAge,
			DBTimeout:         kvdb.DefaultDBTimeout,
		}

		kvStore, err := kvdb.GetBoltBackend(cfg)
		require.NoError(t, err)

		return kvStore
	}

	connectSQLite := func(t *testing.T, dbPath string) kvdb.Backend {
		const (
			timeout  = 10 * time.Second
			maxConns = 50
		)
		sqlbase.Init(maxConns)

		cfg := &sqlite.Config{
			Timeout:        timeout,
			BusyTimeout:    timeout,
			MaxConnections: maxConns,
		}

		kvStore, err := kvdb.Open(
			kvdb.SqliteBackendName, ctx, cfg,
			dbPath, fileName,
			// NOTE: we use the raw string here else we get an
			// import cycle if we try to import lncfg.NSChannelDB.
			"channeldb",
		)
		require.NoError(t, err)

		return kvStore
	}

	tests := []struct {
		name   string
		dbPath string
	}{
		{
			"empty",
			t.TempDir(),
		},
		{
			"testdata",
			"testdata",
		},
	}

	// Determine if we are using a SQLite file or a Bolt DB file.
	var isSqlite bool
	if strings.HasSuffix(fileName, ".sqlite") {
		isSqlite = true
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chanDBPath := path.Join(test.dbPath, fileName)
			t.Logf("Connecting to channel DB at: %s", chanDBPath)

			connectDB := connectBBolt
			if isSqlite {
				connectDB = connectSQLite
			}

			migrate(t, connectDB(t, test.dbPath))
		})
	}
}
