package aliasmgr

import (
	"math/rand"
	"path/filepath"
	"testing"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// TestAliasStorePeerAlias tests that putting and retrieving a peer's alias
// works properly.
func TestAliasStorePeerAlias(t *testing.T) {
	t.Parallel()

	// Create the backend database and use this to create the aliasStore.
	dbPath := filepath.Join(t.TempDir(), "testdb")
	db, err := kvdb.Create(
		kvdb.BoltBackendName, dbPath, true, kvdb.DefaultDBTimeout,
		false,
	)
	require.NoError(t, err)
	defer db.Close()

	linkUpdater := func(shortID lnwire.ShortChannelID) error {
		return nil
	}

	aliasStore, err := NewManager(db, linkUpdater)
	require.NoError(t, err)

	var chanID1 [32]byte
	_, err = rand.Read(chanID1[:])
	require.NoError(t, err)

	// Test that we can put the (chanID, alias) mapping in the database.
	// Also check that we retrieve exactly what we put in.
	err = aliasStore.PutPeerAlias(chanID1, StartingAlias)
	require.NoError(t, err)

	storedAlias, err := aliasStore.GetPeerAlias(chanID1)
	require.NoError(t, err)
	require.Equal(t, StartingAlias, storedAlias)
}

// TestAliasStoreRequest tests that the aliasStore delivers the expected SCID.
func TestAliasStoreRequest(t *testing.T) {
	t.Parallel()

	// Create the backend database and use this to create the aliasStore.
	dbPath := filepath.Join(t.TempDir(), "testdb")
	db, err := kvdb.Create(
		kvdb.BoltBackendName, dbPath, true, kvdb.DefaultDBTimeout,
		false,
	)
	require.NoError(t, err)
	defer db.Close()

	linkUpdater := func(shortID lnwire.ShortChannelID) error {
		return nil
	}

	aliasStore, err := NewManager(db, linkUpdater)
	require.NoError(t, err)

	// We'll assert that the very first alias we receive is StartingAlias.
	alias1, err := aliasStore.RequestAlias()
	require.NoError(t, err)
	require.Equal(t, StartingAlias, alias1)

	// The next alias should be the result of passing in StartingAlias to
	// getNextScid.
	nextAlias := getNextScid(alias1)
	alias2, err := aliasStore.RequestAlias()
	require.NoError(t, err)
	require.Equal(t, nextAlias, alias2)
}

// TestAliasLifecycle tests that the aliases can be created and deleted.
func TestAliasLifecycle(t *testing.T) {
	t.Parallel()

	// Create the backend database and use this to create the aliasStore.
	dbPath := filepath.Join(t.TempDir(), "testdb")
	db, err := kvdb.Create(
		kvdb.BoltBackendName, dbPath, true, kvdb.DefaultDBTimeout,
		false,
	)
	require.NoError(t, err)
	defer db.Close()

	updateChan := make(chan struct{}, 1)

	linkUpdater := func(shortID lnwire.ShortChannelID) error {
		updateChan <- struct{}{}
		return nil
	}

	aliasStore, err := NewManager(db, linkUpdater)
	require.NoError(t, err)

	const (
		base  = uint64(123123123)
		alias = uint64(456456456)
	)

	// Parse the aliases and base to short channel ID format.
	baseScid := lnwire.NewShortChanIDFromInt(base)
	aliasScid := lnwire.NewShortChanIDFromInt(alias)
	aliasScid2 := lnwire.NewShortChanIDFromInt(alias + 1)

	// Add the first alias.
	err = aliasStore.AddLocalAlias(aliasScid, baseScid, false, true)
	require.NoError(t, err)

	// The link updater should be called.
	<-updateChan

	// Query the aliases and verify the results.
	aliasList := aliasStore.GetAliases(baseScid)
	require.Len(t, aliasList, 1)
	require.Contains(t, aliasList, aliasScid)

	// Add the second alias.
	err = aliasStore.AddLocalAlias(aliasScid2, baseScid, false, true)
	require.NoError(t, err)

	// The link updater should be called.
	<-updateChan

	// Query the aliases and verify the results.
	aliasList = aliasStore.GetAliases(baseScid)
	require.Len(t, aliasList, 2)
	require.Contains(t, aliasList, aliasScid)
	require.Contains(t, aliasList, aliasScid2)

	// Delete the first alias.
	err = aliasStore.DeleteLocalAlias(aliasScid, baseScid)
	require.NoError(t, err)

	// The link updater should be called.
	<-updateChan

	// We expect to get an error if we attempt to delete the same alias
	// again.
	err = aliasStore.DeleteLocalAlias(aliasScid, baseScid)
	require.ErrorIs(t, err, ErrAliasNotFound)

	// The link updater should _not_ be called.
	select {
	case <-updateChan:
		t.Fatal("link alias updater should not have been called")
	default:
	}

	// Query the aliases and verify that first one doesn't exist anymore.
	aliasList = aliasStore.GetAliases(baseScid)
	require.Len(t, aliasList, 1)
	require.Contains(t, aliasList, aliasScid2)
	require.NotContains(t, aliasList, aliasScid)

	// Delete the second alias.
	err = aliasStore.DeleteLocalAlias(aliasScid2, baseScid)
	require.NoError(t, err)

	// The link updater should be called.
	<-updateChan

	// Query the aliases and verify that none exists.
	aliasList = aliasStore.GetAliases(baseScid)
	require.Len(t, aliasList, 0)

	// We now request an alias generated by the aliasStore. This should give
	// the first from the pre-defined list of allocated aliases.
	firstRequested, err := aliasStore.RequestAlias()
	require.NoError(t, err)
	require.Equal(t, StartingAlias, firstRequested)

	// We now manually add the next alias from the range as a custom alias.
	secondAlias := getNextScid(firstRequested)
	err = aliasStore.AddLocalAlias(secondAlias, baseScid, false, true)
	require.NoError(t, err)

	// When we now request another alias from the allocation list, we expect
	// the third one (tx position 2) to be returned.
	thirdRequested, err := aliasStore.RequestAlias()
	require.NoError(t, err)
	require.Equal(t, getNextScid(secondAlias), thirdRequested)
	require.EqualValues(t, 2, thirdRequested.TxPosition)
}

// TestGetNextScid tests that given a current lnwire.ShortChannelID,
// getNextScid returns the expected alias to use next.
func TestGetNextScid(t *testing.T) {
	tests := []struct {
		name     string
		current  lnwire.ShortChannelID
		expected lnwire.ShortChannelID
	}{
		{
			name:    "starting alias",
			current: StartingAlias,
			expected: lnwire.ShortChannelID{
				BlockHeight: AliasStartBlockHeight,
				TxIndex:     0,
				TxPosition:  1,
			},
		},
		{
			name: "txposition rollover",
			current: lnwire.ShortChannelID{
				BlockHeight: 16_100_000,
				TxIndex:     15,
				TxPosition:  65535,
			},
			expected: lnwire.ShortChannelID{
				BlockHeight: 16_100_000,
				TxIndex:     16,
				TxPosition:  0,
			},
		},
		{
			name: "txindex max no rollover",
			current: lnwire.ShortChannelID{
				BlockHeight: 16_100_000,
				TxIndex:     16777215,
				TxPosition:  15,
			},
			expected: lnwire.ShortChannelID{
				BlockHeight: 16_100_000,
				TxIndex:     16777215,
				TxPosition:  16,
			},
		},
		{
			name: "txindex rollover",
			current: lnwire.ShortChannelID{
				BlockHeight: 16_100_000,
				TxIndex:     16777215,
				TxPosition:  65535,
			},
			expected: lnwire.ShortChannelID{
				BlockHeight: 16_100_001,
				TxIndex:     0,
				TxPosition:  0,
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			nextScid := getNextScid(test.current)
			require.Equal(t, test.expected, nextScid)
		})
	}
}
