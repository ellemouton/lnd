package graphdb

import (
	"context"
	"database/sql"
	"errors"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/kvdb/postgres"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// Here we define various database paths, connection strings and file names that
// we will use to open the database connections. These should be changed to
// point to your actual local test databases.
const (
	bboltDBPath          = "testdata/kvdb"
	kvdbSqlitePath       = "testdata/kvdb"
	nativeSQLSqlitePath  = "testdata"
	kvdbPostgresDNS      = "postgres://test@localhost/graphbenchmark_kvdb"
	nativeSQLPostgresDNS = "postgres://test@localhost/graphbenchmark"

	kvdbSqliteFile      = "channel.sqlite"
	kvdbBBoltFile       = "channel.db"
	nativeSQLSqliteFile = "lnd.sqlite"

	testMaxSQLiteConnections   = 2
	testMaxPostgresConnections = 50
	testSQLBusyTimeout         = 5 * time.Second
)

// Here we define some variables that will be used to configure the graph stores
// we open for testing. These can be modified to suit your testing needs.
var (
	// dbTestChain is the chain hash used for initialising the test
	// databases. This should be changed to match the chain hash of the
	// database you are testing against.
	dbTestChain = *chaincfg.MainNetParams.GenesisHash

	// testStoreOptions is used to configure the graph stores we open for
	// testing.
	testStoreOptions = []StoreOptionModifier{
		WithBatchCommitInterval(500 * time.Millisecond),
	}

	// testSQLPaginationCfg is used to configure the pagination settings for
	// the SQL stores we open for testing.
	testSQLPaginationCfg = sqldb.DefaultPagedQueryConfig()

	// testSqlitePragmaOpts is a set of SQLite pragma options that we apply
	// to the SQLite databases we open for testing.
	testSqlitePragmaOpts = []string{
		"synchronous=full",
		"auto_vacuum=incremental",
		"fullfsync=true",
	}
)

// dbConnection is a struct that holds the name of the database connection
// and a function to open the connection.
type dbConnection struct {
	name string
	open func(testing.TB) V1Store
}

// This var block defines the various database connections that we will use
// for testing. Each connection is defined as a dbConnection struct that
// contains a name and an open function. The open function is used to create
// a new V1Store instance for the given database type.
var (
	// kvdbBBoltConn is a connection to a kvdb-bbolt database called
	// channel.db.
	kvdbBBoltConn = dbConnection{
		name: "kvdb-bbolt",
		open: func(b testing.TB) V1Store {
			return connectBBoltDB(b, bboltDBPath, kvdbBBoltFile)
		},
	}

	// kvdbSqliteConn is a connection to a kvdb-sqlite database called
	// channel.sqlite.
	kvdbSqliteConn = dbConnection{
		name: "kvdb-sqlite",
		open: func(b testing.TB) V1Store {
			return connectKVDBSqlite(
				b, kvdbSqlitePath, kvdbSqliteFile,
			)
		},
	}

	// nativeSQLSqliteConn is a connection to a native SQL sqlite database
	// called lnd.sqlite.
	nativeSQLSqliteConn = dbConnection{
		name: "native-sqlite",
		open: func(b testing.TB) V1Store {
			return connectNativeSQLite(
				b, nativeSQLSqlitePath, nativeSQLSqliteFile,
			)
		},
	}

	// kvdbPostgresConn is a connection to a kvdb-postgres database
	// using a postgres connection string.
	kvdbPostgresConn = dbConnection{
		name: "kvdb-postgres",
		open: func(b testing.TB) V1Store {
			return connectKVDBPostgres(b, kvdbPostgresDNS)
		},
	}

	// nativeSQLPostgresConn is a connection to a native SQL postgres
	// database using a postgres connection string.
	nativeSQLPostgresConn = dbConnection{
		name: "native-postgres",
		open: func(b testing.TB) V1Store {
			return connectNativePostgres(b, nativeSQLPostgresDNS)
		},
	}
)

// connectNativePostgres creates a V1Store instance backed by a native Postgres
// database for testing purposes.
func connectNativePostgres(t testing.TB, dsn string) V1Store {
	store, err := sqldb.NewPostgresStore(&sqldb.PostgresConfig{
		Dsn:            dsn,
		MaxConnections: testMaxPostgresConnections,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	return newSQLStore(t, store)
}

// connectNativeSQLite creates a V1Store instance backed by a native SQLite
// database for testing purposes.
func connectNativeSQLite(t testing.TB, dbPath, file string) V1Store {
	store, err := sqldb.NewSqliteStore(
		&sqldb.SqliteConfig{
			MaxConnections: testMaxSQLiteConnections,
			BusyTimeout:    testSQLBusyTimeout,
			PragmaOptions:  testSqlitePragmaOpts,
		},
		path.Join(dbPath, file),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close())
	})

	return newSQLStore(t, store)
}

// connectKVDBPostgres creates a V1Store instance backed by a kvdb-postgres
// database for testing purposes.
func connectKVDBPostgres(t testing.TB, dsn string) V1Store {
	kvStore, err := kvdb.Open(
		kvdb.PostgresBackendName, context.Background(),
		&postgres.Config{
			Dsn:            dsn,
			MaxConnections: testMaxPostgresConnections,
		},
		// NOTE: we use the raw string here else we get an
		// import cycle if we try to import lncfg.NSChannelDB.
		"channeldb",
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, kvStore.Close())
	})

	return newKVStore(t, kvStore)
}

// connectKVDBSqlite creates a V1Store instance backed by a kvdb-sqlite
// database for testing purposes.
func connectKVDBSqlite(t testing.TB, dbPath, fileName string) V1Store {
	sqlbase.Init(testMaxSQLiteConnections)
	kvStore, err := kvdb.Open(
		kvdb.SqliteBackendName, context.Background(),
		&sqlite.Config{
			BusyTimeout:    testSQLBusyTimeout,
			MaxConnections: testMaxSQLiteConnections,
			PragmaOptions:  testSqlitePragmaOpts,
		}, dbPath, fileName,
		// NOTE: we use the raw string here else we get an
		// import cycle if we try to import lncfg.NSChannelDB.
		"channeldb",
	)
	require.NoError(t, err)

	return newKVStore(t, kvStore)
}

// connectBBoltDB creates a new BBolt database connection for testing.
func connectBBoltDB(t testing.TB, dbPath, fileName string) V1Store {
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

	return newKVStore(t, kvStore)
}

// newKVStore creates a new KVStore instance for testing using a provided
// kvdb.Backend instance.
func newKVStore(t testing.TB, backend kvdb.Backend) V1Store {
	store, err := NewKVStore(backend, testStoreOptions...)
	require.NoError(t, err)

	return store
}

// newSQLStore creates a new SQLStore instance for testing using a provided
// sqldb.DB instance.
func newSQLStore(t testing.TB, db sqldb.DB) V1Store {
	err := db.ApplyAllMigrations(
		context.Background(), sqldb.GetMigrations(),
	)
	require.NoError(t, err)

	graphExecutor := sqldb.NewTransactionExecutor(
		db.GetBaseDB(), func(tx *sql.Tx) SQLQueries {
			return db.GetBaseDB().WithTx(tx)
		},
	)

	store, err := NewSQLStore(
		&SQLStoreConfig{
			ChainHash:     dbTestChain,
			PaginationCfg: testSQLPaginationCfg,
		},
		graphExecutor, testStoreOptions...,
	)
	require.NoError(t, err)

	return store
}

// TestPopulateDBs is a helper test that can be used to populate various local
// graph DBs from some source graph DB. This can then be used to run the
// various benchmark tests against the same graph data.
//
// TODO(elle): this test reveals that the batching logic we use might be
// problematic for postgres backends. This needs some investigation & it might
// make sense to only use LazyAdd for sqlite backends & it may make sense to
// also add a maximum batch size to avoid grouping too many updates at once.
// Observations:
//   - the LazyAdd options need to be turned off for channel/policy update calls
//     for both the native SQL postgres & kvdb postgres backend for this test to
//     succeed.
//   - The LazyAdd option must be added for the sqlite backends, else it takes
//     very long to sync the graph.
func TestPopulateDBs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// NOTE: uncomment the line below to run this test locally.
	t.Skipf("Skipping local helper test")

	// Set your desired source database.
	sourceDB := kvdbSqliteConn

	// Populate this list with the desired destination databases.
	destinations := []dbConnection{
		kvdbBBoltConn,
		nativeSQLSqliteConn,
		kvdbPostgresConn,
		nativeSQLPostgresConn,
	}

	// Open and start the source graph.
	src, err := NewChannelGraph(sourceDB.open(t))
	require.NoError(t, err)
	require.NoError(t, src.Start())
	t.Cleanup(func() {
		require.NoError(t, src.Stop())
	})

	// countNodes is a helper function to count the number of nodes in the
	// graph.
	countNodes := func(graph *ChannelGraph) int {
		numNodes := 0
		err := graph.ForEachNode(ctx, func(tx NodeRTx) error {
			numNodes++
			return nil
		}, func() {})
		require.NoError(t, err)

		return numNodes
	}

	// countChannels is a helper function to count the number of channels
	// in the graph.
	countChannels := func(graph *ChannelGraph) (int, int) {
		var (
			numChans    = 0
			numPolicies = 0
		)
		err := graph.ForEachChannel(
			ctx, func(info *models.ChannelEdgeInfo,
				policy,
				policy2 *models.ChannelEdgePolicy) error {

				numChans++
				if policy != nil {
					numPolicies++
				}
				if policy2 != nil {
					numPolicies++
				}

				return nil
			}, func() {})
		require.NoError(t, err)

		return numChans, numPolicies
	}

	t.Logf("Number of nodes in source graph (%s): %d", sourceDB.name,
		countNodes(src))
	numChan, numPol := countChannels(src)
	t.Logf("Number of channels & policies in source graph (%s): %d "+
		"channels, %d policies", sourceDB.name, numChan, numPol)

	for _, destDB := range destinations {
		t.Run(destDB.name, func(t *testing.T) {
			t.Parallel()

			// Open and start the destination graph.
			dest, err := NewChannelGraph(destDB.open(t))
			require.NoError(t, err)
			require.NoError(t, dest.Start())
			t.Cleanup(func() {
				require.NoError(t, dest.Stop())
			})

			t.Logf("Number of nodes in %s graph: %d", destDB.name,
				countNodes(dest))
			numChan, numPol := countChannels(dest)
			t.Logf("Number of channels in %s graph: %d, %d",
				destDB.name, numChan, numPol)

			// Sync the source graph to the destination graph.
			syncGraph(t, src, dest)

			t.Logf("Number of nodes in %s graph after sync: %d",
				destDB.name, countNodes(dest))
			numChan, numPol = countChannels(dest)
			t.Logf("Number of channels in %s graph after sync: "+
				"%d, %d", destDB.name, numChan, numPol)
		})
	}
}

// syncGraph synchronizes the source graph with the destination graph by
// copying all nodes and channels from the source to the destination.
func syncGraph(t *testing.T, src, dest *ChannelGraph) {
	ctx := context.Background()

	var (
		s = rate.Sometimes{
			Interval: 10 * time.Second,
		}
		t0 = time.Now()

		chunk = 0
		total = 0
		mu    sync.Mutex
	)

	reportNodeStats := func() {
		elapsed := time.Since(t0).Seconds()
		ratePerSec := float64(chunk) / elapsed
		t.Logf("Synced %d nodes (last chunk: %d) "+
			"(%.2f nodes/second)",
			total, chunk, ratePerSec)

		t0 = time.Now()
	}

	var wgNodes sync.WaitGroup
	err := src.ForEachNode(ctx, func(tx NodeRTx) error {
		wgNodes.Add(1)
		go func() {
			defer wgNodes.Done()

			err := dest.AddLightningNode(
				ctx, tx.Node(), batch.LazyAdd(),
			)
			require.NoError(t, err)

			mu.Lock()
			total++
			chunk++
			s.Do(func() {
				reportNodeStats()
				chunk = 0
			})
			mu.Unlock()
		}()

		return nil
	}, func() {})
	require.NoError(t, err)

	wgNodes.Wait()
	reportNodeStats()
	t.Logf("Done syncing %d nodes", total)

	total = 0
	chunk = 0
	t0 = time.Now()

	reportChanStats := func() {
		elapsed := time.Since(t0).Seconds()
		ratePerSec := float64(chunk) / elapsed
		t.Logf("Synced %d channels (and its "+
			"policies) (last chunk: %d) "+
			"(%.2f channels/second)",
			total, chunk, ratePerSec)

		t0 = time.Now()
	}

	var wgChans sync.WaitGroup
	err = src.ForEachChannel(ctx, func(info *models.ChannelEdgeInfo,
		policy1, policy2 *models.ChannelEdgePolicy) error {

		// Add each channel & policy. We do this in a goroutine to
		// take advantage of batch processing.
		wgChans.Add(1)
		go func() {
			defer wgChans.Done()

			err := dest.AddChannelEdge(
				ctx, info, batch.LazyAdd(),
			)
			if !errors.Is(err, ErrEdgeAlreadyExist) {
				require.NoError(t, err)
			}

			if policy1 != nil {
				err = dest.UpdateEdgePolicy(
					ctx, policy1, batch.LazyAdd(),
				)
				require.NoError(t, err)
			}

			if policy2 != nil {
				err = dest.UpdateEdgePolicy(
					ctx, policy2, batch.LazyAdd(),
				)
				require.NoError(t, err)
			}

			mu.Lock()
			total++
			chunk++
			s.Do(func() {
				reportChanStats()
				chunk = 0
			})
			mu.Unlock()
		}()

		return nil
	}, func() {})
	require.NoError(t, err)

	wgChans.Wait()
	reportChanStats()

	t.Logf("Done syncing %d channels", total)
}
