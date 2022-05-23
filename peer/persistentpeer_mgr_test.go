package peer

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/lightningnetwork/lnd/lntest/channels"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/stretchr/testify/require"
)

const defaultTimeout = 30 * time.Second

var (
	testAddr1 = &net.TCPAddr{IP: (net.IP)([]byte{0xA, 0x0, 0x0, 0x1}),
		Port: 9000}

	testAddr2 = &net.TCPAddr{IP: (net.IP)([]byte{0xA, 0x0, 0x0, 0x1}),
		Port: 9001}

	testAddr3 = &net.TCPAddr{IP: (net.IP)([]byte{0xA, 0x0, 0x0, 0x1}),
		Port: 9003}

	connReq1 = &connmgr.ConnReq{Addr: testAddr1}
	connReq2 = &connmgr.ConnReq{Addr: testAddr2}
	connReq3 = &connmgr.ConnReq{Addr: testAddr3}
)

// TestPersistentPeerManager tests that the PersistentPeerManager correctly
// manages the persistent peers.
func TestPersistentPeerManager(t *testing.T) {
	_, alicePubKey := btcec.PrivKeyFromBytes(channels.AlicesPrivKey)
	_, bobPubKey := btcec.PrivKeyFromBytes(channels.BobsPrivKey)

	m := NewPersistentPeerManager(&PersistentPeerMgrConfig{
		MinBackoff: time.Millisecond * 10,
		MaxBackoff: time.Millisecond * 100,
	})
	defer m.Stop()

	// Alice should not initially be a persistent peer.
	require.False(t, m.IsPersistentPeer(alicePubKey))

	// Now add Alice as a non-permanent persistent peer.
	m.AddPeer(alicePubKey, false)
	require.True(t, m.IsPersistentPeer(alicePubKey))
	require.True(t, m.IsNonPermPersistentPeer(alicePubKey))

	// Bob should not yet be a persistent peer.
	require.False(t, m.IsPersistentPeer(bobPubKey))

	// Now add Bob as a permanent persistent peer.
	m.AddPeer(bobPubKey, true)
	require.True(t, m.IsPersistentPeer(bobPubKey))
	require.False(t, m.IsNonPermPersistentPeer(bobPubKey))

	// Both Alice and Bob should be listed as persistent peers.
	peers := m.PersistentPeers()
	require.Len(t, peers, 2)

	if peers[0].IsEqual(alicePubKey) {
		require.True(t, peers[1].IsEqual(bobPubKey))
	} else {
		require.True(t, peers[0].IsEqual(bobPubKey))
		require.True(t, peers[1].IsEqual(alicePubKey))
	}

	// Delete Alice.
	m.DelPeer(alicePubKey)
	require.False(t, m.IsPersistentPeer(alicePubKey))

	peers = m.PersistentPeers()
	require.Len(t, peers, 1)
	require.True(t, peers[0].IsEqual(bobPubKey))

	// Add an address for Bob.
	m.AddPeerAddresses(bobPubKey, &lnwire.NetAddress{
		IdentityKey: bobPubKey,
		Address:     testAddr1,
	})

	// Add another address for Bob.
	m.AddPeerAddresses(bobPubKey, &lnwire.NetAddress{
		IdentityKey: bobPubKey,
		Address:     testAddr2,
	})

	// Both addresses should appear in Bob's address list.
	var addrs []*lnwire.NetAddress
	for _, addr := range m.conns[route.NewVertex(bobPubKey)].addrs {
		addrs = append(addrs, addr)
	}
	require.Len(t, addrs, 2)
	if addrs[0].Address.String() == testAddr1.String() {
		require.Equal(t, addrs[1].Address.String(), testAddr2.String())
	} else {
		require.Equal(t, addrs[0].Address.String(), testAddr2.String())
		require.Equal(t, addrs[1].Address.String(), testAddr1.String())
	}

	// If SetAddresses is used, however, then this should overwrite any
	// previous addresses stored for Bob.
	m.SetPeerAddresses(bobPubKey, &lnwire.NetAddress{
		IdentityKey: bobPubKey,
		Address:     testAddr3,
	})
	addrs = []*lnwire.NetAddress{}
	for _, addr := range m.conns[route.NewVertex(bobPubKey)].addrs {
		addrs = append(addrs, addr)
	}
	require.Len(t, addrs, 1)
	require.Equal(t, addrs[0].Address.String(), testAddr3.String())

	// Add a connection request for Bob.
	m.AddPeerConnReq(bobPubKey, connReq1)
	require.Equal(t, m.NumPeerConnReqs(bobPubKey), 1)

	// Add another connection request for Bob.
	m.AddPeerConnReq(bobPubKey, connReq2)
	require.Equal(t, m.NumPeerConnReqs(bobPubKey), 2)

	// Both connection requests should appear in Bob's connection request
	// list.
	reqs := m.GetPeerConnReqs(bobPubKey)
	require.Len(t, reqs, 2)
	if reqs[0].String() == connReq1.String() {
		require.Equal(t, reqs[1].String(), connReq2.String())
	} else {
		require.Equal(t, reqs[0].String(), connReq2.String())
		require.Equal(t, reqs[1].String(), connReq1.String())
	}

	// Delete Bob.
	m.DelPeer(bobPubKey)
	peers = m.PersistentPeers()
	require.Len(t, peers, 0)
}

// TestRetryCanceller tests that the peer retry canceller channel logic works
// as expected.
func TestRetryCanceller(t *testing.T) {
	m := NewPersistentPeerManager(&PersistentPeerMgrConfig{
		MinBackoff: time.Millisecond * 10,
		MaxBackoff: time.Millisecond * 100,
	})
	defer m.Stop()

	_, alicePubKey := btcec.PrivKeyFromBytes(channels.AlicesPrivKey)
	m.AddPeer(alicePubKey, false)

	rc := m.GetRetryCanceller(alicePubKey)

	// retryFunction represents a function that should be canceled if the
	// retry canceller channel is closed.
	errs := make(chan error, 3)
	retryFunction := func() {
		select {
		case <-time.After(time.Second * 10):
			errs <- fmt.Errorf("retry canceller was not cancelled")
		case <-rc:
			errs <- nil
		}
	}

	// Spin off a few retry functions.
	go retryFunction()
	go retryFunction()
	go retryFunction()

	// Cancel the retries. This should cause all the retry functions to
	// exit.
	m.CancelRetries(alicePubKey)

	for i := 0; i < 3; i++ {
		err := <-errs
		if err != nil {
			t.Fatalf(err.Error())
		}
	}

	// Calling cancel again should not cause any closing-of-nil-channel
	// panics.
	m.CancelRetries(alicePubKey)
}

// TestConnectPeer tests that the PersistentPeerManager's ConnectPeer function
// correctly creates and cancels connection requests.
func TestConnectPeer(t *testing.T) {
	// Create and a new mock connection manager.
	cm := newMockConnMgr(t)
	defer cm.stop()

	// Create a new PersistentPeerManager.
	m := NewPersistentPeerManager(&PersistentPeerMgrConfig{
		ConnMgr:    cm,
		MinBackoff: time.Millisecond * 10,
		MaxBackoff: time.Millisecond * 100,
	})
	defer m.Stop()

	_, alicePubKey := btcec.PrivKeyFromBytes(channels.AlicesPrivKey)

	// Add Alice as a persistent peer.
	m.AddPeer(alicePubKey, false)

	// There are currently no addresses stored for Alice, so calling
	// ConnectPeer should not result in any connection requests.
	m.ConnectPeer(alicePubKey)
	require.Equal(t, cm.totalNumConnReqs(), 0)

	// Now we add an address for Alice and attempt to connect again. This
	// should result in 1 connection request for the given address.
	m.AddPeerAddresses(alicePubKey, &lnwire.NetAddress{
		IdentityKey: alicePubKey,
		Address:     testAddr1,
	})
	m.ConnectPeer(alicePubKey)
	assertOneConnReqPerAddress(t, cm, testAddr1)

	// If we now add a second address for Alice, calling ConnectPeer again
	// should result in one more connection request for the new address.
	// The connection for the first address should remain intact.
	m.AddPeerAddresses(alicePubKey, &lnwire.NetAddress{
		IdentityKey: alicePubKey,
		Address:     testAddr2,
	})
	m.ConnectPeer(alicePubKey)
	assertOneConnReqPerAddress(t, cm, testAddr1, testAddr2)

	// If we use SetAddresses to overwrite the current list of addresses
	// stored for Alice and then call ConnectPeer again, the appropriate
	// connection requests should be added and removed. We will set
	// addresses 2 and 3. We then expect the connection request for address
	// 1 to be removed, the connReq for 2 to remain and a connReq for 3
	// to be added.
	m.SetPeerAddresses(
		alicePubKey,
		&lnwire.NetAddress{
			IdentityKey: alicePubKey,
			Address:     testAddr2,
		}, &lnwire.NetAddress{
			IdentityKey: alicePubKey,
			Address:     testAddr3,
		},
	)
	m.ConnectPeer(alicePubKey)
	assertOneConnReqPerAddress(t, cm, testAddr2, testAddr3)
	assertNoConnReqs(t, cm, testAddr1)
}

var _ connMgr = (*mockConnMgr)(nil)

// mockConnMgr mocks the connmgr.
type mockConnMgr struct {
	// reqs holds the active connection requests. It is a map from conn req
	// ID to conn req object.
	reqs map[uint64]*connmgr.ConnReq

	// cm is a real ConnManager. It is used (but not started) so that ID's
	// can be assigned to connmgr.ConnReq objects. We cannot manually
	// assign IDs since the id field is an unexported field.
	cm *connmgr.ConnManager

	sync.Mutex
}

// newMockConnMgr constructs a new mockConnMgr.
func newMockConnMgr(t *testing.T) *mockConnMgr {
	cm, err := connmgr.New(&connmgr.Config{
		Dial: func(addr net.Addr) (net.Conn, error) {
			return nil, nil
		},
	})
	require.NoError(t, err)
	cm.Start()

	return &mockConnMgr{
		reqs: make(map[uint64]*connmgr.ConnReq),
		cm:   cm,
	}
}

// stop cleans up any resources managed by the mockConnMgr.
func (m *mockConnMgr) stop() {
	m.cm.Stop()
}

// totalNumConnReqs returns the number of connection requests that the
// mockConnMgr is keeping track of.
func (m *mockConnMgr) totalNumConnReqs() int {
	m.Lock()
	defer m.Unlock()

	return len(m.reqs)
}

// numConnReqs returns the number of active connection requests to the given
// address.
func (m *mockConnMgr) numConnReqs(addr net.Addr) int {
	m.Lock()
	defer m.Unlock()

	count := 0
	for _, cr := range m.reqs {
		if cr.Addr.(*lnwire.NetAddress).Address.String() ==
			addr.String() {

			count++
		}
	}

	return count
}

// Connect adds the given connection request to the active set and generates
// a unique ID for it.
func (m *mockConnMgr) Connect(c *connmgr.ConnReq) {
	m.Lock()
	defer m.Unlock()

	m.cm.Connect(c)
	m.reqs[c.ID()] = c
}

// Remove removes the connection request with the given ID from the active set.
func (m *mockConnMgr) Remove(id uint64) {
	m.Lock()
	defer m.Unlock()

	m.cm.Remove(id)
	delete(m.reqs, id)
}

var _ connMgr = (*mockConnMgr)(nil)

// assertNoConnReqs ensures that the connection manager has no connection
// requests for any of the given addresses.
func assertNoConnReqs(t *testing.T, cm *mockConnMgr, addrs ...net.Addr) {
	err := wait.Predicate(func() bool {
		for _, addr := range addrs {
			if cm.numConnReqs(addr) != 0 {
				return false
			}
		}
		return true
	}, defaultTimeout)
	require.NoError(t, err)
}

// assertOneConnReqPerAddress ensures that the mock connection manager has one
// connection request for each address given.
func assertOneConnReqPerAddress(t *testing.T, cm *mockConnMgr,
	addrs ...net.Addr) {

	err := wait.Predicate(func() bool {
		for _, addr := range addrs {
			if cm.numConnReqs(addr) != 1 {
				return false
			}
		}
		return true
	}, defaultTimeout)
	require.NoError(t, err)
}
