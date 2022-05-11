package peer

import (
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// PersistentPeerManager manages persistent peers.
type PersistentPeerManager struct {
	// conns maps a peer's public key string to a persistentPeer object.
	conns map[route.Vertex]*persistentPeer

	sync.RWMutex
}

// persistentPeer holds all the info about a peer that the
// PersistentPeerManager needs.
type persistentPeer struct {
	// pubKey is the public key identifier of the peer.
	pubKey *btcec.PublicKey

	// perm indicates if we should maintain a connection with a peer even
	// if we have no channels with the peer.
	perm bool

	// addrs is all the addresses we know about for this peer. It is a map
	// from the address string to the address struct.
	addrs map[string]*lnwire.NetAddress

	// connReqs holds all the active connection requests that we have for
	// the peer.
	connReqs []*connmgr.ConnReq
}

// NewPersistentPeerManager creates a new PersistentPeerManager instance.
func NewPersistentPeerManager() *PersistentPeerManager {
	return &PersistentPeerManager{
		conns: make(map[route.Vertex]*persistentPeer),
	}
}

// AddPeer adds a new persistent peer for the PersistentPeerManager to keep
// track of.
func (m *PersistentPeerManager) AddPeer(pubKey *btcec.PublicKey, perm bool) {
	m.Lock()
	defer m.Unlock()

	peerKey := route.NewVertex(pubKey)

	m.conns[peerKey] = &persistentPeer{
		pubKey: pubKey,
		perm:   perm,
		addrs:  make(map[string]*lnwire.NetAddress),
	}
}

// IsPersistentPeer returns true if the given peer is a peer that the
// PersistentPeerManager manages.
func (m *PersistentPeerManager) IsPersistentPeer(pubKey *btcec.PublicKey) bool {
	m.RLock()
	defer m.RUnlock()

	_, ok := m.conns[route.NewVertex(pubKey)]
	return ok
}

// IsNonPermPersistentPeer returns true if the peer is a persistent peer but
// has been marked as non-permanent.
func (m *PersistentPeerManager) IsNonPermPersistentPeer(
	pubKey *btcec.PublicKey) bool {

	m.RLock()
	defer m.RUnlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return false
	}

	return !peer.perm
}

// DelPeer removes a peer from the list of persistent peers that the
// PersistentPeerManager will manage.
func (m *PersistentPeerManager) DelPeer(pubKey *btcec.PublicKey) {
	m.Lock()
	defer m.Unlock()

	delete(m.conns, route.NewVertex(pubKey))
}

// PersistentPeers returns the list of pub key strings of the peers it is
// currently keeping track of.
func (m *PersistentPeerManager) PersistentPeers() []*btcec.PublicKey {
	m.RLock()
	defer m.RUnlock()

	peers := make([]*btcec.PublicKey, 0, len(m.conns))
	for _, p := range m.conns {
		peers = append(peers, p.pubKey)
	}

	return peers
}

// SetPeerAddresses can be used to manually set the addresses for the persistent
// peer that will then be used during connection request creation. This function
// overwrites any previously stored addresses for the peer.
func (m *PersistentPeerManager) SetPeerAddresses(pubKey *btcec.PublicKey,
	addrs ...*lnwire.NetAddress) {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.addrs = make(map[string]*lnwire.NetAddress)
	for _, addr := range addrs {
		peer.addrs[addr.String()] = addr
	}
}

// AddPeerAddresses is used to add addresses to a peers list of addresses.
func (m *PersistentPeerManager) AddPeerAddresses(pubKey *btcec.PublicKey,
	addrs ...*lnwire.NetAddress) {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	for _, addr := range addrs {
		peer.addrs[addr.String()] = addr
	}
}

// GetPeerAddresses returns all the addresses stored for the peer.
func (m *PersistentPeerManager) GetPeerAddresses(
	pubKey *btcec.PublicKey) []*lnwire.NetAddress {

	m.RLock()
	defer m.RUnlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return nil
	}

	addrs := make([]*lnwire.NetAddress, 0, len(peer.addrs))
	for _, addr := range peer.addrs {
		addrs = append(addrs, addr)
	}

	return addrs
}

// GetConnReqs returns all the connection requests of the given peer.
func (m *PersistentPeerManager) GetConnReqs(
	pubKey *btcec.PublicKey) []*connmgr.ConnReq {

	m.RLock()
	defer m.RUnlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return nil
	}

	return peer.connReqs
}

// DelConnReqs deletes all the connection requests for the given peer.
func (m *PersistentPeerManager) DelConnReqs(pubKey *btcec.PublicKey) {
	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.connReqs = nil
}

// SetConnReqs sets the connection requests for the given peer. Note that it
// overrides any previously stored connection requests for the peer.
func (m *PersistentPeerManager) SetConnReqs(pubKey *btcec.PublicKey,
	connReqs ...*connmgr.ConnReq) {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.connReqs = connReqs
}

// AddConnReq appends the given connection request to the give peers list of
// connection requests.
func (m *PersistentPeerManager) AddConnReq(pubKey *btcec.PublicKey,
	connReq *connmgr.ConnReq) {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.connReqs = append(peer.connReqs, connReq)
}
