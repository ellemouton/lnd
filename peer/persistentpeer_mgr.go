package peer

import (
	"crypto/rand"
	"math/big"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/connmgr"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

const (
	// defaultStableConnDuration is a floor under which all reconnection
	// attempts will apply exponential randomized backoff. Connections
	// durations exceeding this value will be eligible to have their
	// backoffs reduced.
	defaultStableConnDuration = 10 * time.Minute
)

// PersistentPeerMgrConfig holds the config of the PersistentPeerManager.
type PersistentPeerMgrConfig struct {
	// MinBackoff is the shortest backoff when reconnecting to a persistent
	// peer.
	MinBackoff time.Duration

	// MaxBackoff is the longest backoff when reconnecting to a persistent
	// peer.
	MaxBackoff time.Duration
}

// PersistentPeerManager manages persistent peers.
type PersistentPeerManager struct {
	// cfg holds the config of the manager.
	cfg *PersistentPeerMgrConfig

	// conns maps a peer's public key to a persistentPeer object.
	conns map[route.Vertex]*persistentPeer

	sync.RWMutex
}

// persistentPeer holds all the info about a peer that the
// PersistentPeerManager needs.
type persistentPeer struct {
	// pubKey is the public key identifier of the peer.
	pubKey *btcec.PublicKey

	// perm indicates if a connection to the peer should be maintained even
	// if there are no channels with the peer.
	perm bool

	// addrs is all the addresses we know about for this peer. It is a map
	// from the address string to the address object.
	addrs map[string]*lnwire.NetAddress

	// connReqs holds all the active connection requests that we have for
	// the peer.
	connReqs []*connmgr.ConnReq

	// backoff is the time to wait before trying to reconnect to a peer.
	backoff time.Duration
}

// NewPersistentPeerManager creates a new PersistentPeerManager instance.
func NewPersistentPeerManager(
	cfg *PersistentPeerMgrConfig) *PersistentPeerManager {

	return &PersistentPeerManager{
		cfg:   cfg,
		conns: make(map[route.Vertex]*persistentPeer),
	}
}

// AddPeer adds a new persistent peer for the PersistentPeerManager to keep
// track of.
func (m *PersistentPeerManager) AddPeer(pubKey *btcec.PublicKey, perm bool) {
	m.Lock()
	defer m.Unlock()

	peerKey := route.NewVertex(pubKey)

	backoff := m.cfg.MinBackoff
	if peer, ok := m.conns[peerKey]; ok {
		backoff = peer.backoff
	}

	m.conns[peerKey] = &persistentPeer{
		pubKey:  pubKey,
		perm:    perm,
		addrs:   make(map[string]*lnwire.NetAddress),
		backoff: backoff,
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

// PersistentPeers returns the list of public keys of the peers it is currently
// keeping track of.
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
// peer. These will then be used during connection request creation. This
// function overwrites any previously stored addresses for the peer.
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

// AddPeerAddresses is used to add addresses to a peers existing list of
// addresses.
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

// GetPeerConnReqs returns all the pending connection requests we have for the
// given peer.
func (m *PersistentPeerManager) GetPeerConnReqs(
	pubKey *btcec.PublicKey) []*connmgr.ConnReq {

	m.RLock()
	defer m.RUnlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return nil
	}

	return peer.connReqs
}

// DelPeerConnReqs deletes all the connection requests for the given peer.
func (m *PersistentPeerManager) DelPeerConnReqs(pubKey *btcec.PublicKey) {
	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.connReqs = nil
}

// SetPeerConnReqs sets the connection requests for the given peer. Note that it
// overrides any previously stored connection requests for the peer.
func (m *PersistentPeerManager) SetPeerConnReqs(pubKey *btcec.PublicKey,
	connReqs ...*connmgr.ConnReq) {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.connReqs = connReqs
}

// AddPeerConnReq appends the given connection request to the existing list for the
// given peer.
func (m *PersistentPeerManager) AddPeerConnReq(pubKey *btcec.PublicKey,
	connReq *connmgr.ConnReq) {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return
	}

	peer.connReqs = append(peer.connReqs, connReq)
}

// NumPeerConnReqs returns the number of connection requests for the given peer.
func (m *PersistentPeerManager) NumPeerConnReqs(pubKey *btcec.PublicKey) int {
	m.RLock()
	defer m.RUnlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return 0
	}

	return len(peer.connReqs)
}

// NextPeerBackoff calculates, sets and returns the next backoff duration that
// should be used before attempting to reconnect to the peer.
func (m *PersistentPeerManager) NextPeerBackoff(pubKey *btcec.PublicKey,
	startTime time.Time) time.Duration {

	m.Lock()
	defer m.Unlock()

	peer, ok := m.conns[route.NewVertex(pubKey)]
	if !ok {
		return m.cfg.MinBackoff
	}

	peer.backoff = nextPeerBackoff(
		peer.backoff, m.cfg.MinBackoff, m.cfg.MaxBackoff, startTime,
	)

	return peer.backoff
}

// nextPeerBackoff computes the next backoff duration for a peer using
// exponential backoff. If no previous backoff was known, the default is
// returned.
func nextPeerBackoff(currentBackoff, minBackoff, maxBackoff time.Duration,
	startTime time.Time) time.Duration {

	// If the peer failed to start properly, we'll just use the previous
	// backoff to compute the subsequent randomized exponential backoff
	// duration. This will roughly double on average.
	if startTime.IsZero() {
		return computeNextBackoff(currentBackoff, maxBackoff)
	}

	// The peer succeeded in starting. If the connection didn't last long
	// enough to be considered stable, we'll continue to back off retries
	// with this peer.
	connDuration := time.Since(startTime)
	if connDuration < defaultStableConnDuration {
		return computeNextBackoff(currentBackoff, maxBackoff)
	}

	// The peer succeed in starting and this was stable peer, so we'll
	// reduce the timeout duration by the length of the connection after
	// applying randomized exponential backoff. We'll only apply this in the
	// case that:
	//   reb(curBackoff) - connDuration > cfg.MinBackoff
	relaxedBackoff := computeNextBackoff(currentBackoff, maxBackoff)
	relaxedBackoff -= connDuration

	if relaxedBackoff > maxBackoff {
		return relaxedBackoff
	}

	// Lastly, if reb(currBackoff) - connDuration <= cfg.MinBackoff, meaning
	// the stable connection lasted much longer than our previous backoff.
	// To reward such good behavior, we'll reconnect after the default
	// timeout.
	return minBackoff
}

// computeNextBackoff uses a truncated exponential backoff to compute the next
// backoff using the value of the exiting backoff. The returned duration is
// randomized in either direction by 1/20 to prevent tight loops from
// stabilizing.
func computeNextBackoff(currBackoff, maxBackoff time.Duration) time.Duration {
	// Double the current backoff, truncating if it exceeds our maximum.
	nextBackoff := 2 * currBackoff
	if nextBackoff > maxBackoff {
		nextBackoff = maxBackoff
	}

	// Using 1/10 of our duration as a margin, compute a random offset to
	// avoid the nodes entering connection cycles.
	margin := nextBackoff / 10

	var wiggle big.Int
	wiggle.SetUint64(uint64(margin))
	if _, err := rand.Int(rand.Reader, &wiggle); err != nil {
		// Randomizing is not mission critical, so we'll just return the
		// current backoff.
		return nextBackoff
	}

	// Otherwise add in our wiggle, but subtract out half of the margin so
	// that the backoff can tweaked by 1/20 in either direction.
	return nextBackoff + (time.Duration(wiggle.Uint64()) - margin/2)
}
