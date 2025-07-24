package graphdb

import (
	"database/sql"
	"errors"
	"golang.org/x/time/rate"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/kvdb/postgres"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

// Here we define various database paths and connection strings that we will use
// to open the database connections. These should be changed to point to your
// actual test databases.
const (
	bboltDBPath          = "testdata/kvdb"
	kvdbSqlitePath       = "testdata/kvdb"
	nativeSQLSqlitePath  = "testdata"
	kvdbPostgresDNS      = "postgres://test@localhost/graphbenchmark_kvdb"
	nativeSQLPostgresDNS = "postgres://test@localhost/graphbenchmark"
)

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
)

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
			return connectBBoltDB(b, bboltDBPath, "channel.db")
		},
	}

	// kvdbSqliteConn is a connection to a kvdb-sqlite database called
	// channel.sqlite.
	kvdbSqliteConn = dbConnection{
		name: "kvdb-sqlite",
		open: func(b testing.TB) V1Store {
			return connectKVDBSqlite(
				b, kvdbSqlitePath, "channel.sqlite",
			)
		},
	}

	// nativeSQLSqliteConn is a connection to a native SQL sqlite database
	// called channel.sqlite.
	nativeSQLSqliteConn = dbConnection{
		name: "native-sqlite",
		open: func(b testing.TB) V1Store {
			return connectNativeSQLite(
				b, nativeSQLSqlitePath, "channel.sqlite",
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

// BenchmarkCacheLoading benchmarks how long it takes to load the in-memory
// graph cache from a populated database.
//
// NOTE: this is to be run against a local graph database. It can be run
// either against a kvdb-bbolt channel.db file, or a kvdb-sqlite channel.sqlite
// file or a postgres connection containing the channel graph in kvdb format and
// finally, it can be run against a native SQL sqlite or postgres database.
func BenchmarkCacheLoading(b *testing.B) {
	ctx := context.Background()

	tests := []dbConnection{
		kvdbBBoltConn,
		kvdbSqliteConn,
		nativeSQLSqliteConn,
		kvdbPostgresConn,
		nativeSQLPostgresConn,
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			store := test.open(b)

			// Reset timer to exclude setup time.
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				graph, err := NewChannelGraph(store)
				require.NoError(b, err)

				require.NoError(b, graph.populateCache(ctx))
			}
		})
	}
}

// TestPopulateDBs is a helper test that can be used to populate various local
// graph DBs from some source graph DB. This can then be used to run the
// various benchmark tests against the same graph data.
func TestPopulateDBs(t *testing.T) {
	t.Parallel()

	t.Skipf("Skipping local helper test")

	// Set your desired source database.
	sourceDB := kvdbBBoltConn

	// Populate this list with the desired destination databases.
	destinations := []dbConnection{
		kvdbSqliteConn,
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
		err := graph.ForEachNode(
			context.Background(), func(tx NodeRTx) error {
				numNodes++
				return nil
			}, func() {},
		)
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
			context.Background(), func(info *models.ChannelEdgeInfo,
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

	t.Logf("Number of nodes in source graph: %d", countNodes(src))
	numChan, numPol := countChannels(src)
	t.Logf("Number of channels in source graph: %d, %d", numChan, numPol)

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

	err := src.ForEachNode(ctx, func(tx NodeRTx) error {
		err := dest.AddLightningNode(ctx, tx.Node())
		require.NoError(t, err)

		mu.Lock()
		total++
		chunk++
		s.Do(func() {
			elapsed := time.Since(t0).Seconds()
			ratePerSec := float64(chunk) / elapsed
			t.Logf("Migrated %d nodes (last chunk: %d) "+
				"(%.2f nodes/second)",
				total, chunk, ratePerSec)

			t0 = time.Now()
			chunk = 0
		})
		mu.Unlock()

		return nil
	}, func() {})
	require.NoError(t, err)

	total = 0
	chunk = 0

	var wg sync.WaitGroup
	err = src.ForEachChannel(ctx, func(info *models.ChannelEdgeInfo,
		policy1, policy2 *models.ChannelEdgePolicy) error {

		// Add each channel & policy. We do this in a goroutine to
		// take advantage of batch processing.
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := dest.AddChannelEdge(ctx, info)
			if !errors.Is(err, ErrEdgeAlreadyExist) {
				require.NoError(t, err)
			}

			if policy1 != nil {
				err = dest.UpdateEdgePolicy(ctx, policy1)
				require.NoError(t, err)
			}

			if policy2 != nil {
				err = dest.UpdateEdgePolicy(ctx, policy2)
				require.NoError(t, err)
			}

			mu.Lock()
			total++
			chunk++
			s.Do(func() {
				elapsed := time.Since(t0).Seconds()
				ratePerSec := float64(chunk) / elapsed
				t.Logf("Migrated %d policies "+
					"(last chunk: %d) "+
					"(%.2f policies/second)",
					total, chunk, ratePerSec)

				t0 = time.Now()
				chunk = 0
			})
			mu.Unlock()
		}()

		return nil
	}, func() {})
	require.NoError(t, err)

	wg.Wait()
}

// connectNativePostgres creates a V1Store instance backed by a native Postgres
// database for testing purposes.
func connectNativePostgres(t testing.TB, dsn string) V1Store {
	store, err := sqldb.NewPostgresStore(&sqldb.PostgresConfig{
		Dsn:            dsn,
		MaxConnections: 50,
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
			MaxConnections: 50,
			BusyTimeout:    30 * time.Second,
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
			MaxConnections: 50,
		}, "channeldb",
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
	const (
		timeout  = 10 * time.Second
		maxConns = 50
	)
	sqlbase.Init(maxConns)

	kvStore, err := kvdb.Open(
		kvdb.SqliteBackendName, context.Background(),
		&sqlite.Config{
			Timeout:        timeout,
			BusyTimeout:    timeout,
			MaxConnections: maxConns,
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
			PaginationCfg: sqldb.DefaultPagedQueryConfig(),
		},
		graphExecutor, testStoreOptions...,
	)
	require.NoError(t, err)

	return store
}
