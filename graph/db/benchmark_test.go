package graphdb

import (
	"database/sql"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/kvdb/postgres"
	"github.com/lightningnetwork/lnd/kvdb/sqlbase"
	"github.com/lightningnetwork/lnd/kvdb/sqlite"
	"github.com/lightningnetwork/lnd/sqldb"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"path"
	"testing"
	"time"
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

	// Here we define various database paths and connection strings
	// that we will use to open the database connections. These should be
	// changed to point to your actual test databases.
	//
	//nolint:ll
	const (
		bboltDBPath          = "testdata/kvdb"
		kvdbSqlitePath       = "testdata/kvdb"
		nativeSQLSqlitePath  = "testdata/channel.sqlite"
		kvdbPostgresDNS      = "postgres://test@localhost/graphbenchmark_kvdb"
		nativeSQLPostgresDNS = "postgres://test@localhost/graphbenchmark"
	)

	tests := []struct {
		name string
		open func(b *testing.B) V1Store
	}{
		{
			name: "kvdb-bbolt",
			open: func(b *testing.B) V1Store {
				return connectBBoltDB(
					b, bboltDBPath, "channel.db",
				)
			},
		},
		{
			name: "kvdb-sqlite",
			open: func(b *testing.B) V1Store {
				return connectKVDBSqlite(
					b, kvdbSqlitePath, "channel.sqlite",
				)
			},
		},
		{
			name: "native-sqlite",
			open: func(b *testing.B) V1Store {
				return connectNativeSQLite(
					b, nativeSQLSqlitePath,
					"channel.sqlite",
				)
			},
		},
		{
			name: "kvdb-postgres",
			open: func(b *testing.B) V1Store {
				return connectKVDBPostgres(b, kvdbPostgresDNS)
			},
		},
		{
			name: "native-postgres",
			open: func(b *testing.B) V1Store {
				return connectNativePostgres(
					b, nativeSQLPostgresDNS,
				)
			},
		},
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
			MaxConnections: 2,
			BusyTimeout:    5 * time.Second,
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
	postgresBackend, err := kvdb.Open(
		kvdb.PostgresBackendName, context.Background(),
		&postgres.Config{
			Dsn:            dsn,
			MaxConnections: 50,
		}, "channeldb",
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		postgresBackend.Close()
	})

	graphStore, err := NewKVStore(
		postgresBackend, WithBatchCommitInterval(500*time.Millisecond),
	)
	require.NoError(t, err)

	return graphStore
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
	store, err := NewKVStore(
		backend, WithBatchCommitInterval(500*time.Millisecond),
	)
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
			ChainHash:     *chaincfg.MainNetParams.GenesisHash,
			PaginationCfg: sqldb.DefaultPagedQueryConfig(),
		},
		graphExecutor, WithBatchCommitInterval(500*time.Millisecond),
	)
	require.NoError(t, err)

	return store
}
