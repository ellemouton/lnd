package wtclient

import (
	"net"
	"testing"

	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/stretchr/testify/require"
)

// TestAddrIterator tests the behaviour of the AddressIterator.
func TestAddrIterator(t *testing.T) {
	// Assert that an iterator can't be initialised with an empty address
	// list.
	_, err := newAddressIterator()
	require.ErrorContains(t, err, "must have at least one address")

	addr1, err := net.ResolveTCPAddr("tcp", "1.2.3.4:8000")
	require.NoError(t, err)

	// Initialise the iterator with addr1.
	iter, err := newAddressIterator(addr1)
	require.NoError(t, err)

	// Attempting to remove addr1 should fail now since it is the only
	// address in the iterator.
	err = iter.Remove(addr1)
	require.ErrorIs(t, err, wtdb.ErrLastTowerAddr)

	addr2, err := net.ResolveTCPAddr("tcp", "1.2.3.4:8001")
	require.NoError(t, err)

	// Add addr2 to the iterator.
	iter.Add(addr2)

	// Check that peek returns addr1.
	a1 := iter.Peek(false)
	require.NoError(t, err)
	require.Equal(t, addr1, a1)

	// Calling peek multiple times should return the same result.
	a1 = iter.Peek(false)
	require.Equal(t, addr1, a1)

	// Calling Next should now return addr2.
	a2, err := iter.Next(false)
	require.NoError(t, err)
	require.Equal(t, addr2, a2)

	// Assert that Peek now returns addr2.
	a2 = iter.Peek(false)
	require.NoError(t, err)
	require.Equal(t, addr2, a2)

	// Calling Next should result in reaching the end of th list.
	_, err = iter.Next(false)
	require.ErrorIs(t, err, ErrAddressesExhausted)

	// Calling Peek now should reset the queue and return addr1.
	a1 = iter.Peek(false)
	require.Equal(t, addr1, a1)

	// Wind the list to the end again so that we can test the Reset func.
	_, err = iter.Next(false)
	require.NoError(t, err)

	_, err = iter.Next(false)
	require.ErrorIs(t, err, ErrAddressesExhausted)

	iter.Reset()

	// Now Next should return addr 2.
	a2, err = iter.Next(false)
	require.NoError(t, err)
	require.Equal(t, addr2, a2)

	addr3, err := net.ResolveTCPAddr("tcp", "1.2.3.4:8002")
	require.NoError(t, err)

	// Add addr3 now to ensure that the iteration works even if we are
	// midway through the queue.
	iter.Add(addr3)

	// Now Next should return addr 3.
	a3, err := iter.Next(false)
	require.NoError(t, err)
	require.Equal(t, addr3, a3)

	// Let's now remove addr3.
	err = iter.Remove(addr3)
	require.NoError(t, err)

	// Since addr3 is gone, Peek should return addr1.
	a1 = iter.Peek(false)
	require.Equal(t, addr1, a1)

	// Lastly, we will test the "locking" of addresses.
	a1 = iter.Peek(true)
	require.Equal(t, addr1, a1)
	require.True(t, iter.HasLocked())

	err = iter.Remove(addr1)
	require.ErrorIs(t, err, ErrAddrInUse)

	iter.ReleaseLock(addr1)
	require.False(t, iter.HasLocked())

	err = iter.Remove(addr1)
	require.NoError(t, err)
}
