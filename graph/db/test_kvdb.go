package graphdb

import (
	"testing"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/stretchr/testify/require"
)

// NewTestDB is a helper function that creates an BBolt database for testing.
func NewTestDB(t testing.TB) *KVStore {
	return NewTestDBFromPath(t, t.TempDir())
}

// NewTestDBFromPath is a helper function that creates a new BoltStore with a
// connection to an existing BBolt database for testing.
func NewTestDBFromPath(t testing.TB, dbPath string) *KVStore {
	backend, backendCleanup, err := kvdb.GetTestBackend(dbPath, "cgr")
	require.NoError(t, err)

	t.Cleanup(backendCleanup)

	graphStore, err := NewKVStore(backend)
	require.NoError(t, err)

	return graphStore
}
