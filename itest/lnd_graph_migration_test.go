package itest

import (
	"context"
	"database/sql"
	"path"

	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/kvdb/postgres"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
)

func testGraphMigration(ht *lntest.HarnessTest) {
	alice := ht.NewNodeWithCoins("Alice", nil)

	// Make sure we run the test with SQLite or Postgres.
	if alice.Cfg.DBBackend != node.BackendSqlite &&
		alice.Cfg.DBBackend != node.BackendPostgres {

		ht.Skip("node not running with SQLite or Postgres")
	}

	// Skip the test if the node is already running with native SQL.
	if alice.Cfg.NativeSQL {
		ht.Skip("node already running with native SQL")
	}

	// 1) Spin up a mini network and then connect Alice to it.
	_, nodes := ht.CreateSimpleNetwork(
		[][]string{nil, nil, nil, nil},
		lntest.OpenChannelParams{Amt: chanAmt},
	)

	// Connect Alice to one of the nodes. Alice should now perform a graph
	// sync with the node.
	ht.EnsureConnected(alice, nodes[0])

	// Wait for Alice to have a full view of the graph.
	ht.AssertNumEdges(alice, 3, false)

	// Now stop Alice so we can open the DB for examination.
	require.NoError(ht, alice.Stop())

	// Open the KV store channel graph DB.
	db := openKVStoreChanGraph(ht, alice)

	// Assert that it contains the expected number of nodes and edges.
	ctx := context.Background()
	assertDBState := func(db graphdb.V1Store) {
		var (
			numNodes int
			edges    = make(map[uint64]bool)
		)
		err := db.ForEachNode(ctx, func(tx graphdb.NodeRTx) error {
			numNodes++

			// For each node, also count the number of edges.
			return tx.ForEachChannel(func(info *models.ChannelEdgeInfo,
				_ *models.ChannelEdgePolicy,
				_ *models.ChannelEdgePolicy) error {

				edges[info.ChannelID] = true
				return nil
			})
		})
		require.NoError(ht, err)
		require.Equal(ht, 5, numNodes)
		require.Equal(ht, 3, len(edges))
	}
	assertDBState(db)

	alice.SetExtraArgs([]string{"--db.use-native-sql"})

	// Now run the migration flow three times to ensure that each run is
	// idempotent.
	for i := 0; i < 3; i++ {
		// Start alice with the native SQL flag set. This will trigger
		// the migration to run.
		require.NoError(ht, alice.Start(ht.Context()))

		// At this point the migration should have completed and the
		// node should be running with native SQL. Now we'll stop alice
		// again so we can safely examine the database.
		require.NoError(ht, alice.Stop())

		// Now we'll open the database with the native SQL backend and
		// fetch the invoices again to ensure that they were migrated
		// correctly.
		sqlInvoiceDB := openNativeSQLGraphDB(ht, alice)
		assertDBState(sqlInvoiceDB)
	}

	// Now restart Alice without the --db.use-native-sql flag so we can check
	// that the KV tombstone was set and that Bob will fail to start.
	require.NoError(ht, alice.Stop())
	alice.SetExtraArgs(nil)

	// alice should now fail to start due to the tombstone being set.
	require.NoError(ht, alice.StartLndCmd(ht.Context()))
	require.Error(ht, alice.WaitForProcessExit())

	// Start alice again so the test can complete.
	alice.SetExtraArgs([]string{"--db.use-native-sql"})
	require.NoError(ht, alice.Start(ht.Context()))
}

func openKVStoreChanGraph(ht *lntest.HarnessTest,
	hn *node.HarnessNode) graphdb.V1Store {

	sqlbase.Init(0)
	var (
		backend kvdb.Backend
		err     error
	)

	switch hn.Cfg.DBBackend {
	case node.BackendSqlite:
		backend, err = kvdb.Open(
			kvdb.SqliteBackendName,
			ht.Context(),
			&sqlite.Config{
				Timeout:     defaultTimeout,
				BusyTimeout: defaultTimeout,
			},
			hn.Cfg.DBDir(), lncfg.SqliteChannelDBName,
			lncfg.NSChannelDB,
		)
		require.NoError(ht, err)

	case node.BackendPostgres:
		backend, err = kvdb.Open(
			kvdb.PostgresBackendName, ht.Context(),
			&postgres.Config{
				Dsn:     hn.Cfg.PostgresDsn,
				Timeout: defaultTimeout,
			}, lncfg.NSChannelDB,
		)
		require.NoError(ht, err)
	}

	store, err := graphdb.NewKVStore(backend)
	require.NoError(ht, err)

	return store
}

func openNativeSQLGraphDB(ht *lntest.HarnessTest,
	hn *node.HarnessNode) graphdb.V1Store {

	var db *sqldb.BaseDB

	switch hn.Cfg.DBBackend {
	case node.BackendSqlite:
		sqliteStore, err := sqldb.NewSqliteStore(
			&sqldb.SqliteConfig{
				Timeout:     defaultTimeout,
				BusyTimeout: defaultTimeout,
			},
			path.Join(
				hn.Cfg.DBDir(),
				lncfg.SqliteNativeDBName,
			),
		)
		require.NoError(ht, err)
		db = sqliteStore.BaseDB

	case node.BackendPostgres:
		postgresStore, err := sqldb.NewPostgresStore(
			&sqldb.PostgresConfig{
				Dsn:     hn.Cfg.PostgresDsn,
				Timeout: defaultTimeout,
			},
		)
		require.NoError(ht, err)
		db = postgresStore.BaseDB
	}

	executor := sqldb.NewTransactionExecutor(
		db, func(tx *sql.Tx) graphdb.SQLQueries {
			return db.WithTx(tx)
		},
	)

	store, err := graphdb.NewSQLStore(
		&graphdb.SQLStoreConfig{
			ChainHash: *ht.Miner().ActiveNet.GenesisHash,
		},
		executor,
	)
	require.NoError(ht, err)

	return store
}
