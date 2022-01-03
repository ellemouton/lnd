package peer

import (
	"sync"
	"time"

	"github.com/btcsuite/btcd/wire"

	"github.com/btcsuite/btcd/connmgr"
	"github.com/lightningnetwork/lnd/lnwire"

	"github.com/lightningnetwork/lnd/routing"
)

const (
	// defaultMultiAddrConnectionStagger is the number of seconds to wait
	// between attempting to a peer with each of its advertised addresses.
	defaultMultiAddrConnectionStagger = 10 * time.Second
)

type PersistentPeerMgrConfig struct {
	// ConnMgr is used to manage the creation and removal of connection
	// requests. It handles the actual connection to a peer.
	ConnMgr connMgr

	// SubscribeTopology will be used to listen for updates to a persistent
	// peer's advertised addresses.
	SubscribeTopology func() (*routing.TopologyClient, error)

	ChainNet wire.BitcoinNet

	MultiAddrConnectionStagger time.Duration
}

// PersistentPeerManager manages persistent peers and the active connection
// requests we may have to them.
type PersistentPeerManager struct {
	cfg *PersistentPeerMgrConfig

	// conns maps a peer's public key string to a persistentPeer object.
	conns map[string]*persistentPeer

	// connsMu is used to guard the conns map.
	connsMu sync.RWMutex

	wg   sync.WaitGroup
	quit chan struct{}
}

// persistentPeer holds the addresses we stored for a persistent peer along
// with any active connection requests that we have for them. A persistentPeer
// is permanent if we want to maintain a connection to them even when we have
// no open channels with them.
type persistentPeer struct {
	// addrs is all the addresses we know about for this peer.
	addrs map[string]*lnwire.NetAddress

	// connReqs holds all the active connection requests that we have
	// for the peer.
	connReqs []*connmgr.ConnReq

	// perm indicates if we should maintain a connection with a peer even
	// if we have no channels with the peer.
	//
	// TODO(yy): the Brontide.Start doesn't know this value, which means it
	// will continue to send messages even if there are no active channels
	// and the value below is false. Once it's pruned, all its connections
	// will be closed, thus the Brontide.Start will return an error.
	perm bool

	// retryCanceller is used to cancel any retry attempts with backoffs
	// that are still maturing.
	retryCanceller *chan struct{}
}

// connMgr is what the PersistentPeerManager will use to create and remove
// connection requests. The purpose of this interface is to make testing easier.
type connMgr interface {
	Connect(c *connmgr.ConnReq)
	Remove(id uint64)
}

// NewPersistentPeerManager creates a new PersistentPeerManager instance.
func NewPersistentPeerManager(
	cfg *PersistentPeerMgrConfig) *PersistentPeerManager {

	if cfg.MultiAddrConnectionStagger == 0 {
		cfg.MultiAddrConnectionStagger = defaultMultiAddrConnectionStagger
	}

	return &PersistentPeerManager{
		cfg:   cfg,
		conns: make(map[string]*persistentPeer),
		quit:  make(chan struct{}),
	}
}

// Start begins the processes of PersistentPeerManager. It subscribes to
// graph updates and listens for any NodeAnnouncement messages that indicate
// that the addresses of one of the persistent peers has changed and then
// updates the peer's addresses and connection requests accordingly.
func (m *PersistentPeerManager) Start() error {
	graphSub, err := m.cfg.SubscribeTopology()
	if err != nil {
		return err
	}

	m.wg.Add(1)
	go func() {
		defer func() {
			graphSub.Cancel()
			m.wg.Done()
		}()

		for {
			select {
			case topChange, ok := <-graphSub.TopologyChanges:
				// If the router is shutting down, then we will
				// as well.
				if !ok {
					return
				}

				for _, update := range topChange.NodeUpdates {
					pubKeyStr := string(
						update.IdentityKey.
							SerializeCompressed(),
					)

					// We only care about updates from the
					// persistent peers that we are keeping
					// track of.
					m.connsMu.RLock()
					peer, ok := m.conns[pubKeyStr]
					m.connsMu.RUnlock()
					if !ok {
						continue
					}

					addrs := make(map[string]*lnwire.NetAddress)
					for _, addr := range update.Addresses {
						lnAddr := &lnwire.NetAddress{
							IdentityKey: update.IdentityKey,
							Address:     addr,
							ChainNet:    m.cfg.ChainNet,
						}

						addrs[lnAddr.String()] = lnAddr
					}

					m.connsMu.Lock()

					// Update the stored addresses for this
					// peer to reflect the new set.
					peer.addrs = addrs

					// If there are no outstanding
					// connection requests for this peer
					// then our work is done since we are
					// not currently trying to connect to
					// them.
					if len(peer.connReqs) == 0 {
						m.connsMu.Unlock()
						continue
					}

					m.connsMu.Unlock()

					m.ConnectPeer(pubKeyStr)
				}

			case <-m.quit:
				return
			}
		}
	}()

	return nil
}

// Stop closes the quit channel of the PersistentPeerManager and waits for all
// goroutines to exit.
func (m *PersistentPeerManager) Stop() {
	close(m.quit)
	m.wg.Wait()
}

// ConnectPeer uses all the stored addresses for a peer to attempt to connect
// to the peer. It creates connection requests if there are currently none for
// a given address and it removes old connection requests if the associated
// address is no longer in the latest address list for the peer.
func (m *PersistentPeerManager) ConnectPeer(pubKeyStr string) {
	m.connsMu.Lock()
	defer m.connsMu.Unlock()

	peer, ok := m.conns[pubKeyStr]
	if !ok {
		peerLog.Debugf("Peer %x is not a persistent peer. Ignoring "+
			"connection attempt", pubKeyStr)
		return
	}

	if len(peer.addrs) == 0 {
		peerLog.Debugf("Ignoring connection attempt to peer %x "+
			"without any stored address", pubKeyStr)
		return
	}

	// Create a quick lookup map of all the addresses we have stored for
	// the peer. We will remove entries from this duplicate map if we have
	// existing connection requests for the associated address. Any
	// addresses left over in the map will indicate which addresses we
	// should create new connection requests for.
	addrMap := make(map[string]*lnwire.NetAddress)
	for _, addr := range peer.addrs {
		addrMap[addr.String()] = addr
	}

	// Go through each of the existing connection requests and check if
	// they correspond to the latest set of addresses. If there is a
	// connection requests that does not use one of the latest advertised
	// addresses then remove that connection request.
	var updatedConnReqs []*connmgr.ConnReq
	for _, connReq := range peer.connReqs {
		lnAddr := connReq.Addr.(*lnwire.NetAddress).Address.String()

		switch _, ok := addrMap[lnAddr]; ok {
		// If the existing connection request is using one of the
		// latest advertised addresses for the peer then we add it to
		// updatedConnReqs and remove the associated address from
		// addrMap so that we don't recreate this connReq later on.
		case true:
			updatedConnReqs = append(updatedConnReqs, connReq)
			delete(addrMap, lnAddr)

		// If the existing connection request is using an address that
		// is not one of the latest advertised addresses for the peer
		// then we remove the connecting request from the connection
		// manager.
		case false:
			peerLog.Info(
				"Removing conn req:", connReq.Addr.String(),
			)
			m.cfg.ConnMgr.Remove(connReq.ID())
		}
	}

	peer.connReqs = updatedConnReqs

	// Initialize a retry canceller for this peer if one does not exist.
	var cancelChan chan struct{}
	if peer.retryCanceller != nil {
		cancelChan = *peer.retryCanceller
	} else {
		cancelChan = make(chan struct{})
		peer.retryCanceller = &cancelChan
	}

	// Any addresses left in addrMap are new ones that we have not made
	// connection requests for. So create new connection requests for those.
	// If there is more than one address in the address map, stagger the
	// creation of the connection requests for those.
	go func() {
		ticker := time.NewTicker(m.cfg.MultiAddrConnectionStagger)
		defer ticker.Stop()

		for _, addr := range addrMap {
			// Send the persistent connection request to the connection manager,
			// saving the request itself so we can cancel/restart the process as
			// needed.
			connReq := &connmgr.ConnReq{
				Addr:      addr,
				Permanent: true,
			}

			m.connsMu.Lock()
			if peer, ok = m.conns[pubKeyStr]; !ok {
				return
			}
			peer.connReqs = append(peer.connReqs, connReq)
			m.connsMu.Unlock()

			peerLog.Debugf("Attempting persistent connection to "+
				"channel peer %v", addr)

			go m.cfg.ConnMgr.Connect(connReq)

			select {
			case <-m.quit:
				return
			case <-cancelChan:
				return
			case <-ticker.C:
			}
		}
	}()
}

// PersistentPeers returns the list of pub key strings of the peers it is
// currently keeping track of.
func (m *PersistentPeerManager) PersistentPeers() []string {
	m.connsMu.Lock()
	defer m.connsMu.Unlock()

	peers := make([]string, 0, len(m.conns))
	for p := range m.conns {
		peers = append(peers, p)
	}

	return peers
}

// AddPeer adds a new persistent peer for which address updates will be kept
// track of. The peer can be initialised with an initial set of addresses.
func (m *PersistentPeerManager) AddPeer(pubKeyStr string,
	addrs []*lnwire.NetAddress, perm bool) {

	m.connsMu.Lock()
	defer m.connsMu.Unlock()

	addrMap := make(map[string]*lnwire.NetAddress)
	for _, addr := range addrs {
		addrMap[addr.String()] = addr
	}

	m.conns[pubKeyStr] = &persistentPeer{
		addrs: addrMap,
		perm:  perm,
	}
}

// DelPeer removes a peer from the list of persistent peers that the
// PersistentPeerManager will manage.
func (m *PersistentPeerManager) DelPeer(pubKeyStr string) {
	m.connsMu.Lock()
	defer m.connsMu.Unlock()

	delete(m.conns, pubKeyStr)
}

// IsPersistentPeer returns true if the given peer is a persistent peer that the
// PersistentPeerManager manages.
func (m *PersistentPeerManager) IsPersistentPeer(pubKeyStr string) bool {
	m.connsMu.RLock()
	defer m.connsMu.RUnlock()

	_, ok := m.conns[pubKeyStr]
	return ok
}

// IsPermPeer returns true if a connection should be kept with the given peer
// even in the case when we have no channels with the peer.
func (m *PersistentPeerManager) IsPermPeer(pubKeyStr string) bool {
	m.connsMu.RLock()
	defer m.connsMu.RUnlock()

	peer, ok := m.conns[pubKeyStr]
	if !ok {
		return false
	}

	return peer.perm
}

// NumConnReq the number of active connection requests for a peer.
func (m *PersistentPeerManager) NumConnReq(pubKeyStr string) int {
	m.connsMu.RLock()
	defer m.connsMu.RUnlock()

	peer, ok := m.conns[pubKeyStr]
	if !ok {
		return 0
	}

	return len(peer.connReqs)
}

// AddPeerAddresses is used to add addresses to a peers list of addresses.
func (m *PersistentPeerManager) AddPeerAddresses(pubKeyStr string,
	addrs ...*lnwire.NetAddress) {

	m.connsMu.Lock()
	defer m.connsMu.Unlock()

	peer, ok := m.conns[pubKeyStr]
	if !ok {
		return
	}

	for _, addr := range addrs {
		peer.addrs[addr.String()] = addr
	}
}

// UnassignedConnID is the default connection ID that a request can have before
// it actually is submitted to the connmgr.
// TODO(conner): move into connmgr package, or better, add connmgr method for
// generating atomic IDs.
const UnassignedConnID uint64 = 0

// RemovePeerConns removes any connection requests that are active for the
// given peer. An optional skip id can be provided to exclude removing the
// successful connection request.
func (m *PersistentPeerManager) RemovePeerConns(pubKeyStr string,
	skip *uint64) {

	m.connsMu.Lock()
	defer m.connsMu.Unlock()

	peer, ok := m.conns[pubKeyStr]
	if !ok {
		return
	}

	// Cancel any lingering persistent retry attempts, which will
	// prevent retries for any with backoffs that are still maturing.
	if peer.retryCanceller != nil {
		close(*peer.retryCanceller)
		peer.retryCanceller = nil
	}

	// Check to see if we have any outstanding persistent connection
	// requests to this peer. If so, then we'll remove all of these
	// connection requests.
	for _, cr := range peer.connReqs {
		// Atomically capture the current request identifier.
		connID := cr.ID()

		// Skip any zero IDs, this indicates the request has not
		// yet been schedule.
		if connID == UnassignedConnID {
			continue
		}

		// Skip a particular connection ID if instructed.
		if skip != nil && connID == *skip {
			continue
		}

		m.cfg.ConnMgr.Remove(cr.ID())
	}

	peer.connReqs = nil
}