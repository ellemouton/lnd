package wtclient

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/build"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/subscribe"
	"github.com/lightningnetwork/lnd/tor"
	"github.com/lightningnetwork/lnd/watchtower/blob"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/lightningnetwork/lnd/watchtower/wtpolicy"
)

// Client is the primary interface used by the daemon to control a client's
// lifecycle and backup revoked states.
type Client interface {
	// AddTower adds a new watchtower reachable at the given address and
	// considers it for new sessions. If the watchtower already exists, then
	// any new addresses included will be considered when dialing it for
	// session negotiations and backups.
	AddTower(*lnwire.NetAddress) error

	// RemoveTower removes a watchtower from being considered for future
	// session negotiations and from being used for any subsequent backups
	// until it's added again. If an address is provided, then this call
	// only serves as a way of removing the address from the watchtower
	// instead.
	RemoveTower(*btcec.PublicKey, net.Addr) error

	// RegisteredTowers retrieves the list of watchtowers registered with
	// the client.
	RegisteredTowers(...wtdb.ClientSessionListOption) (
		map[blob.Type][]*RegisteredTower, error)

	// LookupTower retrieves a registered watchtower through its public key.
	LookupTower(*btcec.PublicKey, ...wtdb.ClientSessionListOption) (
		map[blob.Type]*RegisteredTower, error)

	// Stats returns the in-memory statistics of the client since startup.
	Stats() ClientStats

	// Policy returns the active client policy configuration.
	Policy(blob.Type) (wtpolicy.Policy, error)

	// RegisterChannel persistently initializes any channel-dependent
	// parameters within the client. This should be called during link
	// startup to ensure that the client is able to support the link during
	// operation.
	RegisterChannel(lnwire.ChannelID, channeldb.ChannelType) error

	// BackupState initiates a request to back up a particular revoked
	// state. If the method returns nil, the backup is guaranteed to be
	// successful unless the client is force quit, or the justice
	// transaction would create dust outputs when trying to abide by the
	// negotiated policy.
	BackupState(chanID *lnwire.ChannelID, stateNum uint64) error

	// Start initializes the watchtower client, allowing it process requests
	// to backup revoked channel states.
	Start() error

	// Stop attempts a graceful shutdown of the watchtower client. In doing
	// so, it will attempt to flush the pipeline and deliver any queued
	// states to the tower before exiting.
	Stop() error

	// ForceQuit will forcibly shutdown the watchtower client. Calling this
	// may lead to queued states being dropped.
	ForceQuit()
}

// Config provides the Manager with access to the resources it requires to
// perform its duty. All nillable fields must be non-nil for the tower to be
// initialized properly.
type Config struct {
	// Signer provides access to the wallet so that the client can sign
	// justice transactions that spend from a remote party's commitment
	// transaction.
	Signer input.Signer

	// SubscribeChannelEvents can be used to subscribe to channel event
	// notifications.
	SubscribeChannelEvents func() (subscribe.Subscription, error)

	// FetchClosedChannel can be used to fetch the info about a closed
	// channel. If the channel is not found or not yet closed then
	// channeldb.ErrClosedChannelNotFound will be returned.
	FetchClosedChannel func(cid lnwire.ChannelID) (
		*channeldb.ChannelCloseSummary, error)

	// ChainNotifier can be used to subscribe to block notifications.
	ChainNotifier chainntnfs.ChainNotifier

	// BuildBreachRetribution is a function closure that allows the client
	// fetch the breach retribution info for a certain channel at a certain
	// revoked commitment height.
	BuildBreachRetribution BreachRetributionBuilder

	// NewAddress generates a new on-chain sweep pkscript.
	NewAddress func() ([]byte, error)

	// SecretKeyRing is used to derive the session keys used to communicate
	// with the tower. The client only stores the KeyLocators internally so
	// that we never store private keys on disk.
	SecretKeyRing ECDHKeyRing

	// Dial connects to an addr using the specified net and returns the
	// connection object.
	Dial tor.DialFunc

	// AuthDialer establishes a brontide connection over an onion or clear
	// network.
	AuthDial AuthDialer

	// DB provides access to the client's stable storage medium.
	DB DB

	// ChainHash identifies the chain that the client is on and for which
	// the tower must be watching to monitor for breaches.
	ChainHash chainhash.Hash

	// ForceQuitDelay is the duration after attempting to shutdown that the
	// client will automatically abort any pending backups if an unclean
	// shutdown is detected. If the value is less than or equal to zero, a
	// call to Stop may block indefinitely. The client can always be
	// ForceQuit externally irrespective of the chosen parameter.
	ForceQuitDelay time.Duration

	// ReadTimeout is the duration we will wait during a read before
	// breaking out of a blocking read. If the value is less than or equal
	// to zero, the default will be used instead.
	ReadTimeout time.Duration

	// WriteTimeout is the duration we will wait during a write before
	// breaking out of a blocking write. If the value is less than or equal
	// to zero, the default will be used instead.
	WriteTimeout time.Duration

	// MinBackoff defines the initial backoff applied to connections with
	// watchtowers. Subsequent backoff durations will grow exponentially up
	// until MaxBackoff.
	MinBackoff time.Duration

	// MaxBackoff defines the maximum backoff applied to connections with
	// watchtowers. If the exponential backoff produces a timeout greater
	// than this value, the backoff will be clamped to MaxBackoff.
	MaxBackoff time.Duration

	// SessionCloseRange is the range over which we will generate a random
	// number of blocks to delay closing a session after its last channel
	// has been closed.
	SessionCloseRange uint32
}

type Manager struct {
	started sync.Once
	stopped sync.Once
	forced  sync.Once

	cfg *Config

	backupMu          sync.Mutex
	summaries         wtdb.ChannelSummaries
	channelTypes      map[lnwire.ChannelID]blob.Type
	chanCommitHeights map[lnwire.ChannelID]uint64

	closableSessionQueue *sessionCloseMinHeap

	clients   map[blob.Type]*towerClient
	clientsMu sync.Mutex

	quit      chan struct{}
	forceQuit chan struct{}

	wg sync.WaitGroup
}

// Compile-time constraint to ensure *towerClient implements the Client
// interface.
var _ Client = (*Manager)(nil)

func NewTowerClient(config *Config, policies ...wtpolicy.Policy) (*Manager,
	error) {

	// Copy the config to prevent side effects from modifying both the
	// internal and external version of the Config.
	cfg := new(Config)
	*cfg = *config

	// Set the read timeout to the default if none was provided.
	if cfg.ReadTimeout <= 0 {
		cfg.ReadTimeout = DefaultReadTimeout
	}

	// Set the write timeout to the default if none was provided.
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = DefaultWriteTimeout
	}

	// Load the sweep pkscripts that have been generated for all previously
	// registered channels.
	chanSummaries, err := cfg.DB.FetchChanSummaries()
	if err != nil {
		return nil, err
	}

	c := &Manager{
		cfg:                  cfg,
		clients:              make(map[blob.Type]*towerClient),
		channelTypes:         make(map[lnwire.ChannelID]blob.Type),
		summaries:            chanSummaries,
		closableSessionQueue: newSessionCloseMinHeap(),
		chanCommitHeights:    make(map[lnwire.ChannelID]uint64),
		quit:                 make(chan struct{}),
		forceQuit:            make(chan struct{}),
	}

	// perUpdate is a callback function that will be used to inspect the
	// full set of candidate client sessions loaded from disk, and to
	// determine the highest known commit height for each channel. This
	// allows the client to reject backups that it has already processed for
	// its active policy.
	perUpdate := func(policy wtpolicy.Policy, chanID lnwire.ChannelID,
		commitHeight uint64) {

		c.backupMu.Lock()
		defer c.backupMu.Unlock()

		// Take the highest commit height found in the session's acked
		// updates.
		height, ok := c.chanCommitHeights[chanID]
		if !ok || commitHeight > height {
			c.chanCommitHeights[chanID] = commitHeight
		}
	}

	for _, p := range policies {
		if err := p.Validate(); err != nil {
			return nil, err
		}

		err := c.addClient(p, perUpdate)
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (t *Manager) getSweepScript(id lnwire.ChannelID) ([]byte, bool) {
	t.backupMu.Lock()
	defer t.backupMu.Unlock()

	summary, ok := t.summaries[id]
	if !ok {
		return nil, false
	}

	return summary.SweepPkScript, true
}

func (t *Manager) AddTower(address *lnwire.NetAddress) error {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	// We'll start by updating our persisted state, followed by our
	// in-memory state, with the new tower. This might not actually be a new
	// tower, but it might include a new address at which it can be reached.
	dbTower, err := t.cfg.DB.CreateTower(address)
	if err != nil {
		return err
	}

	tower, err := NewTowerFromDBTower(dbTower)
	if err != nil {
		return err
	}

	for _, client := range t.clients {
		if err := client.AddTower(tower); err != nil {
			return err
		}
	}

	return nil
}

func (t *Manager) RemoveTower(key *btcec.PublicKey, addr net.Addr) error {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	for _, client := range t.clients {
		if err := client.RemoveTower(key, addr); err != nil {
			return err
		}
	}

	return nil
}

func (t *Manager) RegisteredTowers(opts ...wtdb.ClientSessionListOption) (
	map[blob.Type][]*RegisteredTower, error) {

	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	resp := make(map[blob.Type][]*RegisteredTower)
	for _, client := range t.clients {
		towers, err := client.RegisteredTowers(opts...)
		if err != nil {
			return nil, err
		}

		resp[client.Policy().BlobType] = towers
	}

	return resp, nil
}

func (t *Manager) LookupTower(key *btcec.PublicKey,
	opts ...wtdb.ClientSessionListOption) (map[blob.Type]*RegisteredTower,
	error) {

	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	resp := make(map[blob.Type]*RegisteredTower)
	for _, client := range t.clients {
		tower, err := client.LookupTower(key, opts...)
		if err != nil {
			return nil, err
		}

		resp[client.Policy().BlobType] = tower
	}

	return resp, nil
}

func (t *Manager) Stats() ClientStats {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	var resp ClientStats
	for _, client := range t.clients {
		stats := client.Stats()
		resp.NumTasksAccepted += stats.NumTasksAccepted
		resp.NumTasksIneligible += stats.NumTasksIneligible
		resp.NumTasksPending += stats.NumTasksPending
		resp.NumSessionsAcquired += stats.NumSessionsAcquired
		resp.NumSessionsExhausted += stats.NumSessionsExhausted
	}

	return resp
}

func (t *Manager) Policy(blobType blob.Type) (wtpolicy.Policy, error) {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	var policy wtpolicy.Policy
	client, ok := t.clients[blobType]
	if !ok {
		return policy, fmt.Errorf("no client for the given blob type")
	}

	return client.Policy(), nil
}

func (t *Manager) RegisterChannel(id lnwire.ChannelID,
	chanType channeldb.ChannelType) error {

	blobType := blob.TypeAltruistCommit
	if chanType.HasAnchors() {
		blobType = blob.TypeAltruistAnchorCommit
	}

	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	if _, ok := t.channelTypes[id]; ok {
		return nil
	}

	t.channelTypes[id] = blobType

	// If a pkscript for this channel already exists, the channel has been
	// previously registered.
	if _, ok := t.summaries[id]; ok {
		return nil
	}

	// Otherwise, generate a new sweep pkscript used to sweep funds for this
	// channel.
	pkScript, err := t.cfg.NewAddress()
	if err != nil {
		return err
	}

	// Persist the sweep pkscript so that restarts will not introduce
	// address inflation when the channel is reregistered after a restart.
	err = t.cfg.DB.RegisterChannel(id, pkScript)
	if err != nil {
		return err
	}

	// Finally, cache the pkscript in our in-memory cache to avoid db
	// lookups for the remainder of the daemon's execution.
	t.summaries[id] = wtdb.ClientChanSummary{
		SweepPkScript: pkScript,
	}

	return nil
}

func (t *Manager) BackupState(chanID *lnwire.ChannelID,
	stateNum uint64) error {

	select {
	case <-t.quit:
		return ErrClientExiting
	default:
	}

	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	blobType, ok := t.channelTypes[*chanID]
	if !ok {
		return ErrUnregisteredChannel
	}

	client, ok := t.clients[blobType]
	if !ok {
		return fmt.Errorf("no client for his blob type")
	}

	t.backupMu.Lock()

	// Ignore backups that have already been presented to the client.
	height, ok := t.chanCommitHeights[*chanID]
	if ok && stateNum <= height {
		t.backupMu.Unlock()
		log.Debugf("Ignoring duplicate backup for chanid=%v at "+
			"height=%d", chanID, stateNum)

		return nil
	}

	// This backup has a higher commit height than any known backup for this
	// channel. We'll update our tip so that we won't accept it again if the
	// link flaps.
	t.chanCommitHeights[*chanID] = stateNum
	t.backupMu.Unlock()

	return client.BackupState(chanID, stateNum)
}

func (t *Manager) Start() error {
	var returnErr error
	t.started.Do(func() {
		t.clientsMu.Lock()
		defer t.clientsMu.Unlock()

		// Iterate over the list of registered channels and check if
		// any of them can be marked as closed.
		for id := range t.summaries {
			isClosed, closedHeight, err := t.isChannelClosed(id)
			if err != nil {
				returnErr = err

				return
			}

			if !isClosed {
				continue
			}

			_, err = t.cfg.DB.MarkChannelClosed(id, closedHeight)
			if err != nil {
				log.Errorf("could not mark channel(%s) as "+
					"closed: %v", id, err)

				continue
			}

			// Since the channel has been marked as closed, we can
			// also remove it from the channel summaries map.
			delete(t.summaries, id)
		}

		chanSub, err := t.cfg.SubscribeChannelEvents()
		if err != nil {
			returnErr = err
			return
		}

		// Load all closable sessions.
		closableSessions, err := t.cfg.DB.ListClosableSessions()
		if err != nil {
			returnErr = err
			return
		}

		err = t.trackClosableSessions(closableSessions)
		if err != nil {
			returnErr = err
			return
		}

		t.wg.Add(1)
		go t.handleChannelCloses(chanSub)

		// Subscribe to new block events.
		blockEvents, err := t.cfg.ChainNotifier.RegisterBlockEpochNtfn(
			nil,
		)
		if err != nil {
			returnErr = err
			return
		}

		t.wg.Add(1)
		go t.handleClosableSessions(blockEvents)

		for _, client := range t.clients {
			if err := client.Start(); err != nil {
				returnErr = err
				return
			}
		}
	})

	return returnErr
}

func (t *Manager) Stop() error {
	var returnErr error
	t.stopped.Do(func() {
		close(t.quit)
		t.wg.Wait()

		t.clientsMu.Lock()
		defer t.clientsMu.Unlock()

		for _, client := range t.clients {
			if err := client.Stop(); err != nil {
				returnErr = err
			}
		}
	})

	return returnErr
}

func (t *Manager) ForceQuit() {
	t.forced.Do(func() {
		t.clientsMu.Lock()
		defer t.clientsMu.Unlock()

		close(t.forceQuit)

		for _, client := range t.clients {
			client.ForceQuit()
		}
	})
}

func (t *Manager) addClient(policy wtpolicy.Policy,
	perUpdate func(policy wtpolicy.Policy, chanID lnwire.ChannelID,
		commitHeight uint64)) error {

	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()

	if _, ok := t.clients[policy.BlobType]; ok {
		return fmt.Errorf("client with blog type already added")
	}

	cfg := t.cfg

	clientCfg := &policyClientCfg{
		Config:         cfg,
		Policy:         policy,
		getSweepScript: t.getSweepScript,
	}

	plog := build.NewPrefixLog(policy.BlobType.LogPrefix(), log)
	c := newTowerClient(clientCfg, plog)

	perMaxHeight := func(s *wtdb.ClientSession, chanID lnwire.ChannelID,
		height uint64) {

		perUpdate(s.Policy, chanID, height)
	}

	perCommittedUpdate := func(s *wtdb.ClientSession,
		u *wtdb.CommittedUpdate) {

		perUpdate(s.Policy, u.BackupID.ChanID, u.BackupID.CommitHeight)
	}

	candidateTowers := newTowerListIterator()
	perActiveTower := func(tower *Tower) {
		// If the tower has already been marked as active, then there is
		// no need to add it to the iterator again.
		if candidateTowers.IsActive(tower.ID) {
			return
		}

		c.log.Infof("Using private watchtower %x, offering policy %s",
			tower.IdentityKey.SerializeCompressed(), c.cfg.Policy)

		// Add the tower to the set of candidate towers.
		candidateTowers.AddCandidate(tower)
	}

	// Load all candidate sessions and towers from the database into the
	// client. We will use any of these sessions if their policies match the
	// current policy of the client, otherwise they will be ignored and new
	// sessions will be requested.
	candidateSessions, err := t.getTowerAndSessionCandidates(
		perActiveTower,
		wtdb.WithPreEvalFilterFn(c.genSessionFilter(true)),
		wtdb.WithPerMaxHeight(perMaxHeight),
		wtdb.WithPerCommittedUpdate(perCommittedUpdate),
		wtdb.WithPostEvalFilterFn(ExhaustedSessionFilter()),
	)
	if err != nil {
		return err
	}

	c.candidateTowers = candidateTowers
	c.candidateSessions = candidateSessions

	c.negotiator = newSessionNegotiator(&NegotiatorConfig{
		DB:            cfg.DB,
		SecretKeyRing: cfg.SecretKeyRing,
		Policy:        c.cfg.Policy,
		ChainHash:     cfg.ChainHash,
		SendMessage:   c.sendMessage,
		ReadMessage:   c.readMessage,
		Dial:          c.dial,
		Candidates:    c.candidateTowers,
		MinBackoff:    cfg.MinBackoff,
		MaxBackoff:    cfg.MaxBackoff,
		Log:           plog,
	})

	t.clients[c.cfg.Policy.BlobType] = c

	return nil
}

// getTowerAndSessionCandidates loads all the towers from the DB and then
// fetches the sessions for each of tower. Sessions are only collected if they
// pass the sessionFilter check. If a tower has a session that does pass the
// sessionFilter check then the perActiveTower call-back will be called on that
// tower.
func (t *Manager) getTowerAndSessionCandidates(
	perActiveTower func(tower *Tower),
	opts ...wtdb.ClientSessionListOption) (
	map[wtdb.SessionID]*ClientSession, error) {

	towers, err := t.cfg.DB.ListTowers()
	if err != nil {
		return nil, err
	}

	candidateSessions := make(map[wtdb.SessionID]*ClientSession)
	for _, dbTower := range towers {
		tower, err := NewTowerFromDBTower(dbTower)
		if err != nil {
			return nil, err
		}

		sessions, err := t.cfg.DB.ListClientSessions(&tower.ID, opts...)
		if err != nil {
			return nil, err
		}

		for _, s := range sessions {
			cs, err := NewClientSessionFromDBSession(
				s, tower, t.cfg.SecretKeyRing,
			)
			if err != nil {
				return nil, err
			}

			// Add the session to the set of candidate sessions.
			candidateSessions[s.ID] = cs

			perActiveTower(tower)
		}
	}

	return candidateSessions, nil
}
