package wtclient_test

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/tor"
	"github.com/lightningnetwork/lnd/watchtower/blob"
	"github.com/lightningnetwork/lnd/watchtower/wtclient"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/lightningnetwork/lnd/watchtower/wtmock"
	"github.com/lightningnetwork/lnd/watchtower/wtpolicy"
	"github.com/lightningnetwork/lnd/watchtower/wtserver"
	"github.com/stretchr/testify/require"
)

const (
	csvDelay uint32 = 144

	towerAddrStr  = "18.28.243.2:9911"
	tower2AddrStr = "18.28.243.3:9911"
	timeout       = 200 * time.Millisecond
)

var (
	revPrivBytes = []byte{
		0x8f, 0x4b, 0x51, 0x83, 0xa9, 0x34, 0xbd, 0x5f,
		0x74, 0x6c, 0x9d, 0x5c, 0xae, 0x88, 0x2d, 0x31,
		0x06, 0x90, 0xdd, 0x8c, 0x9b, 0x31, 0xbc, 0xd1,
		0x78, 0x91, 0x88, 0x2a, 0xf9, 0x74, 0xa0, 0xef,
	}

	toLocalPrivBytes = []byte{
		0xde, 0x17, 0xc1, 0x2f, 0xdc, 0x1b, 0xc0, 0xc6,
		0x59, 0x5d, 0xf9, 0xc1, 0x3e, 0x89, 0xbc, 0x6f,
		0x01, 0x85, 0x45, 0x76, 0x26, 0xce, 0x9c, 0x55,
		0x3b, 0xc9, 0xec, 0x3d, 0xd8, 0x8b, 0xac, 0xa8,
	}

	toRemotePrivBytes = []byte{
		0x28, 0x59, 0x6f, 0x36, 0xb8, 0x9f, 0x19, 0x5d,
		0xcb, 0x07, 0x48, 0x8a, 0xe5, 0x89, 0x71, 0x74,
		0x70, 0x4c, 0xff, 0x1e, 0x9c, 0x00, 0x93, 0xbe,
		0xe2, 0x2e, 0x68, 0x08, 0x4c, 0xb4, 0x0f, 0x4f,
	}

	// addr is the server's reward address given to watchtower clients.
	addr, _ = btcutil.DecodeAddress(
		"tb1pw8gzj8clt3v5lxykpgacpju5n8xteskt7gxhmudu6pa70nwfhe6s3unsyk",
		&chaincfg.TestNet3Params,
	)

	addrScript, _ = txscript.PayToAddrScript(addr)
)

// randPrivKey generates a new secp keypair, and returns the public key.
func randPrivKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()

	sk, err := btcec.NewPrivateKey()
	require.NoError(t, err, "unable to generate pubkey")

	return sk
}

type mockNet struct {
	mu            sync.RWMutex
	connCallbacks map[string]func(wtserver.Peer)
}

func newMockNet() *mockNet {
	return &mockNet{
		connCallbacks: make(map[string]func(peer wtserver.Peer)),
	}
}

func (m *mockNet) Dial(_, _ string, _ time.Duration) (net.Conn, error) {
	return nil, nil
}

func (m *mockNet) LookupHost(_ string) ([]string, error) {
	panic("not implemented")
}

func (m *mockNet) LookupSRV(_, _, _ string) (string, []*net.SRV, error) {
	panic("not implemented")
}

func (m *mockNet) ResolveTCPAddr(_, _ string) (*net.TCPAddr, error) {
	panic("not implemented")
}

func (m *mockNet) AuthDial(local keychain.SingleKeyECDH,
	netAddr *lnwire.NetAddress, _ tor.DialFunc) (wtserver.Peer, error) {

	localPk := local.PubKey()
	localAddr := &net.TCPAddr{
		IP:   net.IP{0x32, 0x31, 0x30, 0x29},
		Port: 36723,
	}

	localPeer, remotePeer := wtmock.NewMockConn(
		localPk, netAddr.IdentityKey, localAddr, netAddr.Address, 0,
	)

	m.mu.RLock()
	defer m.mu.RUnlock()
	cb, ok := m.connCallbacks[netAddr.String()]
	if !ok {
		return nil, fmt.Errorf("no callback registered for this peer")
	}

	cb(remotePeer)

	return localPeer, nil
}

func (m *mockNet) registerConnCallback(netAddr *lnwire.NetAddress,
	cb func(wtserver.Peer)) {

	m.mu.Lock()
	defer m.mu.Unlock()

	m.connCallbacks[netAddr.String()] = cb
}

func (m *mockNet) removeConnCallback(netAddr *lnwire.NetAddress) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.connCallbacks, netAddr.String())
}

type mockChannel struct {
	mu            sync.Mutex
	commitHeight  uint64
	retributions  map[uint64]*lnwallet.BreachRetribution
	localBalance  lnwire.MilliSatoshi
	remoteBalance lnwire.MilliSatoshi

	revSK     *btcec.PrivateKey
	revPK     *btcec.PublicKey
	revKeyLoc keychain.KeyLocator

	toRemoteSK     *btcec.PrivateKey
	toRemotePK     *btcec.PublicKey
	toRemoteKeyLoc keychain.KeyLocator

	toLocalPK *btcec.PublicKey // only need to generate to-local script

	dustLimit lnwire.MilliSatoshi
	csvDelay  uint32
}

func newMockChannel(t *testing.T, signer *wtmock.MockSigner,
	localAmt, remoteAmt lnwire.MilliSatoshi) *mockChannel {

	// Generate the revocation, to-local, and to-remote keypairs.
	revSK := randPrivKey(t)
	revPK := revSK.PubKey()

	toLocalSK := randPrivKey(t)
	toLocalPK := toLocalSK.PubKey()

	toRemoteSK := randPrivKey(t)
	toRemotePK := toRemoteSK.PubKey()

	// Register the revocation secret key and the to-remote secret key with
	// the signer. We will not need to sign with the to-local key, as this
	// is to be known only by the counterparty.
	revKeyLoc := signer.AddPrivKey(revSK)
	toRemoteKeyLoc := signer.AddPrivKey(toRemoteSK)

	c := &mockChannel{
		retributions:   make(map[uint64]*lnwallet.BreachRetribution),
		localBalance:   localAmt,
		remoteBalance:  remoteAmt,
		revSK:          revSK,
		revPK:          revPK,
		revKeyLoc:      revKeyLoc,
		toLocalPK:      toLocalPK,
		toRemoteSK:     toRemoteSK,
		toRemotePK:     toRemotePK,
		toRemoteKeyLoc: toRemoteKeyLoc,
		dustLimit:      546000,
		csvDelay:       144,
	}

	// Create the initial remote commitment with the initial balances.
	c.createRemoteCommitTx(t)

	return c
}

func (c *mockChannel) createRemoteCommitTx(t *testing.T) {
	t.Helper()

	// Construct the to-local witness script.
	toLocalScript, err := input.CommitScriptToSelf(
		c.csvDelay, c.toLocalPK, c.revPK,
	)
	require.NoError(t, err, "unable to create to-local script")

	// Compute the to-local witness script hash.
	toLocalScriptHash, err := input.WitnessScriptHash(toLocalScript)
	require.NoError(t, err, "unable to create to-local witness script hash")

	// Compute the to-remote witness script hash.
	toRemoteScriptHash, err := input.CommitScriptUnencumbered(c.toRemotePK)
	require.NoError(t, err, "unable to create to-remote script")

	// Construct the remote commitment txn, containing the to-local and
	// to-remote outputs. The balances are flipped since the transaction is
	// from the PoV of the remote party. We don't need any inputs for this
	// test. We increment the version with the commit height to ensure that
	// all commitment transactions are unique even if the same distribution
	// of funds is used more than once.
	commitTxn := &wire.MsgTx{
		Version: int32(c.commitHeight + 1),
	}

	var (
		toLocalSignDesc  *input.SignDescriptor
		toRemoteSignDesc *input.SignDescriptor
	)

	var outputIndex int
	if c.remoteBalance >= c.dustLimit {
		commitTxn.TxOut = append(commitTxn.TxOut, &wire.TxOut{
			Value:    int64(c.remoteBalance.ToSatoshis()),
			PkScript: toLocalScriptHash,
		})

		// Create the sign descriptor used to sign for the to-local
		// input.
		toLocalSignDesc = &input.SignDescriptor{
			KeyDesc: keychain.KeyDescriptor{
				KeyLocator: c.revKeyLoc,
				PubKey:     c.revPK,
			},
			WitnessScript: toLocalScript,
			Output:        commitTxn.TxOut[outputIndex],
			HashType:      txscript.SigHashAll,
		}
		outputIndex++
	}
	if c.localBalance >= c.dustLimit {
		commitTxn.TxOut = append(commitTxn.TxOut, &wire.TxOut{
			Value:    int64(c.localBalance.ToSatoshis()),
			PkScript: toRemoteScriptHash,
		})

		// Create the sign descriptor used to sign for the to-remote
		// input.
		toRemoteSignDesc = &input.SignDescriptor{
			KeyDesc: keychain.KeyDescriptor{
				KeyLocator: c.toRemoteKeyLoc,
				PubKey:     c.toRemotePK,
			},
			WitnessScript: toRemoteScriptHash,
			Output:        commitTxn.TxOut[outputIndex],
			HashType:      txscript.SigHashAll,
		}
		outputIndex++
	}

	txid := commitTxn.TxHash()

	var (
		toLocalOutPoint  wire.OutPoint
		toRemoteOutPoint wire.OutPoint
	)

	outputIndex = 0
	if toLocalSignDesc != nil {
		toLocalOutPoint = wire.OutPoint{
			Hash:  txid,
			Index: uint32(outputIndex),
		}
		outputIndex++
	}
	if toRemoteSignDesc != nil {
		toRemoteOutPoint = wire.OutPoint{
			Hash:  txid,
			Index: uint32(outputIndex),
		}
		outputIndex++
	}

	commitKeyRing := &lnwallet.CommitmentKeyRing{
		RevocationKey: c.revPK,
		ToRemoteKey:   c.toLocalPK,
		ToLocalKey:    c.toRemotePK,
	}

	retribution := &lnwallet.BreachRetribution{
		BreachTxHash:         commitTxn.TxHash(),
		RevokedStateNum:      c.commitHeight,
		KeyRing:              commitKeyRing,
		RemoteDelay:          c.csvDelay,
		LocalOutpoint:        toRemoteOutPoint,
		LocalOutputSignDesc:  toRemoteSignDesc,
		RemoteOutpoint:       toLocalOutPoint,
		RemoteOutputSignDesc: toLocalSignDesc,
	}

	c.retributions[c.commitHeight] = retribution
	c.commitHeight++
}

// advanceState creates the next channel state and retribution without altering
// channel balances.
func (c *mockChannel) advanceState(t *testing.T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.createRemoteCommitTx(t)
}

// sendPayment creates the next channel state and retribution after transferring
// amt to the remote party.
func (c *mockChannel) sendPayment(t *testing.T, amt lnwire.MilliSatoshi) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.localBalance < amt {
		t.Fatalf("insufficient funds to send, need: %v, have: %v",
			amt, c.localBalance)
	}

	c.localBalance -= amt
	c.remoteBalance += amt
	c.createRemoteCommitTx(t)
}

// receivePayment creates the next channel state and retribution after
// transferring amt to the local party.
func (c *mockChannel) receivePayment(t *testing.T, amt lnwire.MilliSatoshi) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.remoteBalance < amt {
		t.Fatalf("insufficient funds to recv, need: %v, have: %v",
			amt, c.remoteBalance)
	}

	c.localBalance += amt
	c.remoteBalance -= amt
	c.createRemoteCommitTx(t)
}

// getState retrieves the channel's commitment and retribution at state i.
func (c *mockChannel) getState(
	i uint64) (chainhash.Hash, *lnwallet.BreachRetribution) {

	c.mu.Lock()
	defer c.mu.Unlock()

	retribution := c.retributions[i]

	return retribution.BreachTxHash, retribution
}

type testHarness struct {
	t         *testing.T
	cfg       harnessCfg
	signer    *wtmock.MockSigner
	capacity  lnwire.MilliSatoshi
	clientDB  *wtmock.ClientDB
	clientCfg *wtclient.Config
	client    wtclient.Client
	server    *serverHarness
	net       *mockNet

	mu       sync.Mutex
	channels map[lnwire.ChannelID]*mockChannel
}

type harnessCfg struct {
	localBalance       lnwire.MilliSatoshi
	remoteBalance      lnwire.MilliSatoshi
	policy             wtpolicy.Policy
	noRegisterChan0    bool
	noAckCreateSession bool
}

func newHarness(t *testing.T, cfg harnessCfg) *testHarness {
	signer := wtmock.NewMockSigner()
	mockNet := newMockNet()
	clientDB := wtmock.NewClientDB()

	clientCfg := &wtclient.Config{
		Signer:        signer,
		Dial:          mockNet.Dial,
		DB:            clientDB,
		AuthDial:      mockNet.AuthDial,
		SecretKeyRing: wtmock.NewSecretKeyRing(),
		Policy:        cfg.policy,
		NewAddress: func() ([]byte, error) {
			return addrScript, nil
		},
		ReadTimeout:    timeout,
		WriteTimeout:   timeout,
		MinBackoff:     time.Millisecond,
		MaxBackoff:     time.Second,
		ForceQuitDelay: 10 * time.Second,
	}

	h := &testHarness{
		t:         t,
		cfg:       cfg,
		signer:    signer,
		capacity:  cfg.localBalance + cfg.remoteBalance,
		clientDB:  clientDB,
		clientCfg: clientCfg,
		net:       mockNet,
		channels:  make(map[lnwire.ChannelID]*mockChannel),
	}

	server := h.newServerHarness(towerAddrStr)
	h.server = server
	h.server.start()
	h.startClient()

	h.makeChannel(0, h.cfg.localBalance, h.cfg.remoteBalance)
	if !cfg.noRegisterChan0 {
		h.registerChannel(0)
	}

	return h
}

// startClient creates a new server using the harness's current clientCf and
// starts it.
func (h *testHarness) startClient() {
	h.t.Helper()

	var err error
	h.client, err = wtclient.New(h.clientCfg)
	require.NoError(h.t, err)
	require.NoError(h.t, h.client.Start())
	require.NoError(h.t, h.client.AddTower(h.server.addr))
}

// chanIDFromInt creates a unique channel id given a unique integral id.
func chanIDFromInt(id uint64) lnwire.ChannelID {
	var chanID lnwire.ChannelID
	binary.BigEndian.PutUint64(chanID[:8], id)
	return chanID
}

// makeChannel creates new channel with id, using the localAmt and remoteAmt as
// the starting balances. The channel will be available by using h.channel(id).
//
// NOTE: The method fails if channel for id already exists.
func (h *testHarness) makeChannel(id uint64,
	localAmt, remoteAmt lnwire.MilliSatoshi) {

	h.t.Helper()

	chanID := chanIDFromInt(id)
	c := newMockChannel(h.t, h.signer, localAmt, remoteAmt)

	c.mu.Lock()
	_, ok := h.channels[chanID]
	if !ok {
		h.channels[chanID] = c
	}
	c.mu.Unlock()

	if ok {
		h.t.Fatalf("channel %d already created", id)
	}
}

// channel retrieves the channel corresponding to id.
//
// NOTE: The method fails if a channel for id does not exist.
func (h *testHarness) channel(id uint64) *mockChannel {
	h.t.Helper()

	h.mu.Lock()
	c, ok := h.channels[chanIDFromInt(id)]
	h.mu.Unlock()
	if !ok {
		h.t.Fatalf("unable to fetch channel %d", id)
	}

	return c
}

// registerChannel registers the channel identified by id with the client.
func (h *testHarness) registerChannel(id uint64) {
	h.t.Helper()

	chanID := chanIDFromInt(id)
	err := h.client.RegisterChannel(chanID)
	if err != nil {
		h.t.Fatalf("unable to register channel %d: %v", id, err)
	}
}

// advanceChannelN calls advanceState on the channel identified by id the number
// of provided times and returns the breach hints corresponding to the new
// states.
func (h *testHarness) advanceChannelN(id uint64, n int) []blob.BreachHint {
	h.t.Helper()

	channel := h.channel(id)

	var hints []blob.BreachHint
	for i := uint64(0); i < uint64(n); i++ {
		channel.advanceState(h.t)
		breachTxID, _ := h.channel(id).getState(i)
		hints = append(hints, blob.NewBreachHintFromHash(&breachTxID))
	}

	return hints
}

// backupStates instructs the channel identified by id to send backups to the
// client for states in the range [to, from).
func (h *testHarness) backupStates(id, from, to uint64, expErr error) {
	h.t.Helper()

	for i := from; i < to; i++ {
		h.backupState(id, i, expErr)
	}
}

// backupStates instructs the channel identified by id to send a backup for
// state i.
func (h *testHarness) backupState(id, i uint64, expErr error) {
	h.t.Helper()

	_, retribution := h.channel(id).getState(i)

	chanID := chanIDFromInt(id)
	err := h.client.BackupState(&chanID, retribution, channeldb.SingleFunderBit)
	if err != expErr {
		h.t.Fatalf("back error mismatch, want: %v, got: %v",
			expErr, err)
	}
}

// sendPayments instructs the channel identified by id to send amt to the remote
// party for each state in from-to times and returns the breach hints for states
// [from, to).
func (h *testHarness) sendPayments(id, from, to uint64,
	amt lnwire.MilliSatoshi) []blob.BreachHint {

	h.t.Helper()

	channel := h.channel(id)

	var hints []blob.BreachHint
	for i := from; i < to; i++ {
		h.channel(id).sendPayment(h.t, amt)
		breachTxID, _ := channel.getState(i)
		hints = append(hints, blob.NewBreachHintFromHash(&breachTxID))
	}

	return hints
}

// receivePayment instructs the channel identified by id to recv amt from the
// remote party for each state in from-to times and returns the breach hints for
// states [from, to).
func (h *testHarness) recvPayments(id, from, to uint64,
	amt lnwire.MilliSatoshi) []blob.BreachHint {

	h.t.Helper()

	channel := h.channel(id)

	var hints []blob.BreachHint
	for i := from; i < to; i++ {
		channel.receivePayment(h.t, amt)
		breachTxID, _ := channel.getState(i)
		hints = append(hints, blob.NewBreachHintFromHash(&breachTxID))
	}

	return hints
}

// waitServerUpdates blocks until the breach hints provided all appear in the
// watchtower's database or the timeout expires. This is used to test that the
// client in fact sends the updates to the server, even if it is offline.
func (s *serverHarness) waitServerUpdates(hints []blob.BreachHint,
	timeout time.Duration) {

	t := s.h.t
	t.Helper()

	// If no breach hints are provided, we will wait out the full timeout to
	// assert that no updates appear.
	wantUpdates := len(hints) > 0

	hintSet := make(map[blob.BreachHint]struct{})
	for _, hint := range hints {
		hintSet[hint] = struct{}{}
	}
	require.Equal(t, len(hints), len(hintSet))

	// Closure to assert the server's matches are consistent with the hint
	// set.
	serverHasHints := func(matches []wtdb.Match) bool {
		if len(hintSet) != len(matches) {
			return false
		}

		for _, match := range matches {
			_, ok := hintSet[match.Hint]
			require.Truef(t, ok, "match %v in db is not in hint "+
				"set", match.Hint)
		}

		return true
	}

	failTimeout := time.After(timeout)
	for {
		select {
		case <-time.After(time.Second):
			matches, err := s.db.QueryMatches(hints)
			switch {
			case err != nil:
				t.Fatalf("unable to query for hints: %v", err)

			case wantUpdates && serverHasHints(matches):
				return

			case wantUpdates:
				t.Logf("Received %d/%d\n", len(matches),
					len(hints))
			}

		case <-failTimeout:
			matches, err := s.db.QueryMatches(hints)
			switch {
			case err != nil:
				t.Fatalf("unable to query for hints: %v", err)

			case serverHasHints(matches):
				return

			default:
				t.Fatalf("breach hints not received, only "+
					"got %d/%d", len(matches), len(hints))
			}
		}
	}
}

// assertUpdatesForPolicy queries the server db for matches using the provided
// breach hints, then asserts that each match has a session with the expected
// policy.
func (s *serverHarness) assertUpdatesForPolicy(hints []blob.BreachHint,
	expPolicy wtpolicy.Policy) {

	// Query for matches on the provided hints.
	matches, err := s.db.QueryMatches(hints)
	require.NoError(s.h.t, err)

	// Assert that the number of matches is exactly the number of provided
	// hints.
	require.Equal(s.h.t, len(matches), len(hints))

	// Assert that all of the matches correspond to a session with the
	// expected policy.
	for _, match := range matches {
		matchPolicy := match.SessionInfo.Policy
		require.Equal(s.h.t, expPolicy, matchPolicy)
	}
}

// addTower adds a tower found at `addr` to the client.
func (h *testHarness) addTower(addr *lnwire.NetAddress) {
	h.t.Helper()

	if err := h.client.AddTower(addr); err != nil {
		h.t.Fatalf("unable to add tower: %v", err)
	}
}

// removeTower removes a tower from the client. If `addr` is specified, then the
// only said address is removed from the tower.
func (h *testHarness) removeTower(pubKey *btcec.PublicKey, addr net.Addr) {
	h.t.Helper()

	if err := h.client.RemoveTower(pubKey, addr); err != nil {
		h.t.Fatalf("unable to remove tower: %v", err)
	}
}

// newServerHarness constructs a new mock watchtower server.
func (h *testHarness) newServerHarness(netAddr string) *serverHarness {
	towerTCPAddr, err := net.ResolveTCPAddr("tcp", netAddr)
	require.NoError(h.t, err, "Unable to resolve tower TCP addr")

	privKey, err := btcec.NewPrivateKey()
	require.NoError(h.t, err, "Unable to generate tower private key")
	privKeyECDH := &keychain.PrivKeyECDH{PrivKey: privKey}

	towerPubKey := privKey.PubKey()
	towerAddr := &lnwire.NetAddress{
		IdentityKey: towerPubKey,
		Address:     towerTCPAddr,
	}

	db := wtmock.NewTowerDB()
	cfg := &wtserver.Config{
		DB:           db,
		ReadTimeout:  timeout,
		WriteTimeout: timeout,
		NodeKeyECDH:  privKeyECDH,
		NewAddress: func() (btcutil.Address, error) {
			return addr, nil
		},
		NoAckCreateSession: h.cfg.noAckCreateSession,
	}

	server, err := wtserver.New(cfg)
	require.NoError(h.t, err, "unable to create wtserver")

	return &serverHarness{
		h:      h,
		cfg:    cfg,
		db:     db,
		addr:   towerAddr,
		server: server,
	}
}

// serverHarness represents a mock watchtower server.
type serverHarness struct {
	h *testHarness

	cfg    *wtserver.Config
	addr   *lnwire.NetAddress
	db     *wtmock.TowerDB
	server *wtserver.Server
}

// start creates a new server using the harness's current server cfg and starts
// it after registering its Dial callback with the mockNet.
func (s *serverHarness) start() {
	s.h.t.Helper()

	var err error
	s.server, err = wtserver.New(s.cfg)
	require.NoError(s.h.t, err)

	s.h.net.registerConnCallback(s.addr, s.server.InboundPeerConnected)
	require.NoError(s.h.t, s.server.Start())
}

// stop halts the server and removes its Dial callback from the mockNet.
func (s *serverHarness) stop() {
	require.NoError(s.h.t, s.server.Stop())
	s.h.net.removeConnCallback(s.addr)
}

// restart stops the server, applies any given config tweaks and then starts the
// server again.
func (s *serverHarness) restart(op func(cfg *wtserver.Config)) {
	s.stop()
	op(s.cfg)
	s.start()
}

const (
	localBalance  = lnwire.MilliSatoshi(100000000)
	remoteBalance = lnwire.MilliSatoshi(200000000)
)

var defaultTxPolicy = wtpolicy.TxPolicy{
	BlobType:     blob.TypeAltruistCommit,
	SweepFeeRate: wtpolicy.DefaultSweepFeeRate,
}

type clientTest struct {
	name string
	cfg  harnessCfg
	fn   func(*testHarness)
}

var clientTests = []clientTest{
	{
		// Asserts that client will return the ErrUnregisteredChannel
		// error when trying to backup states for a channel that has not
		// been registered (and received it's pkscript).
		name: "backup unregistered channel",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 20000,
			},
			noRegisterChan0: true,
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Advance the channel and backup the retributions. We
			// expect ErrUnregisteredChannel to be returned since
			// the channel was not registered during harness
			// creation.
			h.advanceChannelN(chanID, numUpdates)
			h.backupStates(
				chanID, 0, numUpdates,
				wtclient.ErrUnregisteredChannel,
			)
		},
	},
	{
		// Asserts that the client returns an ErrClientExiting when
		// trying to backup channels after the Stop method has been
		// called.
		name: "backup after stop",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 20000,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Stop the client, subsequent backups should fail.
			h.client.Stop()

			// Advance the channel and try to back up the states. We
			// expect ErrClientExiting to be returned from
			// BackupState.
			h.advanceChannelN(chanID, numUpdates)
			h.backupStates(
				chanID, 0, numUpdates,
				wtclient.ErrClientExiting,
			)
		},
	},
	{
		// Asserts that the client will continue to back up all states
		// that have previously been enqueued before it finishes
		// exiting.
		name: "backup reliable flush",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Generate numUpdates retributions and back them up to
			// the tower.
			hints := h.advanceChannelN(chanID, numUpdates)
			h.backupStates(chanID, 0, numUpdates, nil)

			// Stop the client in the background, to assert the
			// pipeline is always flushed before it exits.
			go h.client.Stop()

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, time.Second)
		},
	},
	{
		// Assert that the client will not send out backups for states
		// whose justice transactions are ineligible for backup, e.g.
		// creating dust outputs.
		name: "backup dust ineligible",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy: wtpolicy.TxPolicy{
					BlobType: blob.TypeAltruistCommit,
					// high sweep fee creates dust
					SweepFeeRate: 1000000,
				},
				MaxUpdates: 20000,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Create the retributions and queue them for backup.
			h.advanceChannelN(chanID, numUpdates)
			h.backupStates(chanID, 0, numUpdates, nil)

			// Ensure that no updates are received by the server,
			// since they should all be marked as ineligible.
			h.server.waitServerUpdates(nil, time.Second)
		},
	},
	{
		// Verifies that the client will properly retransmit a committed
		// state update to the watchtower after a restart if the update
		// was not acked while the client was active last.
		name: "committed update restart",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 20000,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			hints := h.advanceChannelN(0, numUpdates)

			var numSent uint64

			// Add the first two states to the client's pipeline.
			h.backupStates(chanID, 0, 2, nil)
			numSent = 2

			// Wait for both to be reflected in the server's
			// database.
			h.server.waitServerUpdates(hints[:numSent], time.Second)

			// Now, restart the server and prevent it from acking
			// state updates.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckUpdates = true
			})

			// Send the next state update to the tower. Since the
			// tower isn't acking state updates, we expect this
			// update to be committed and sent by the session queue,
			// but it will never receive an ack.
			h.backupState(chanID, numSent, nil)
			numSent++

			// Force quit the client to abort the state updates it
			// has queued. The sleep ensures that the session queues
			// have enough time to commit the state updates before
			// the client is killed.
			time.Sleep(time.Second)
			h.client.ForceQuit()

			// Restart the server and allow it to ack the updates
			// after the client retransmits the unacked update.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckUpdates = false
			})

			// Restart the client and allow it to process the
			// committed update.
			h.startClient()
			defer h.client.ForceQuit()

			// Wait for the committed update to be accepted by the
			// tower.
			h.server.waitServerUpdates(hints[:numSent], time.Second)

			// Finally, send the rest of the updates and wait for
			// the tower to receive the remaining states.
			h.backupStates(chanID, numSent, numUpdates, nil)

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, time.Second)

		},
	},
	{
		// Asserts that the client will continue to retry sending state
		// updates if it doesn't receive an ack from the server. The
		// client is expected to flush everything in its in-memory
		// pipeline once the server begins sending acks again.
		name: "no ack from server",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 100
				chanID     = 0
			)

			// Generate the retributions that will be backed up.
			hints := h.advanceChannelN(chanID, numUpdates)

			// Restart the server and prevent it from acking state
			// updates.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckUpdates = true
			})

			// Now, queue the retributions for backup.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Stop the client in the background, to assert the
			// pipeline is always flushed before it exits.
			go h.client.Stop()

			// Give the client time to saturate a large number of
			// session queues for which the server has not acked the
			// state updates that it has received.
			time.Sleep(time.Second)

			// Restart the server and allow it to ack the updates
			// after the client retransmits the unacked updates.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckUpdates = false
			})

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 5*time.Second)
		},
	},
	{
		// Asserts that the client is able to send state updates to the
		// tower for a full range of channel values, assuming the sweep
		// fee rates permit it. We expect all of these to be successful
		// since a sweep transactions spending only from one output is
		// less expensive than one that sweeps both.
		name: "send and recv",
		cfg: harnessCfg{
			localBalance:  100000001, // ensure (% amt != 0)
			remoteBalance: 200000001, // ensure (% amt != 0)
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 1000,
			},
		},
		fn: func(h *testHarness) {
			var (
				capacity = h.cfg.localBalance +
					h.cfg.remoteBalance
				paymentAmt = lnwire.MilliSatoshi(2000000)
				numSends   = uint64(
					h.cfg.localBalance / paymentAmt,
				)
				numRecvs = uint64(
					capacity / paymentAmt,
				)
				numUpdates = numSends + numRecvs // 200 updates
				chanID     = uint64(0)
			)

			// Send money to the remote party until all funds are
			// depleted.
			sendHints := h.sendPayments(
				chanID, 0, numSends, paymentAmt,
			)

			// Now, sequentially receive the entire channel balance
			// from the remote party.
			recvHints := h.recvPayments(
				chanID, numSends, numUpdates, paymentAmt,
			)

			// Collect the hints generated by both sending and
			// receiving.
			hints := append(sendHints, recvHints...)

			// Backup the channel's states the client.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 3*time.Second)
		},
	},
	{
		// Asserts that the client is able to support multiple links.
		name: "multiple link backup",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const numUpdates = 5

			// Initialize and register an additional 9 channels.
			for id := uint64(1); id < 10; id++ {
				h.makeChannel(
					id, h.cfg.localBalance,
					h.cfg.remoteBalance,
				)
				h.registerChannel(id)
			}

			// Generate the retributions for all 10 channels and
			// collect the breach hints.
			var hints []blob.BreachHint
			for id := uint64(0); id < 10; id++ {
				chanHints := h.advanceChannelN(id, numUpdates)
				hints = append(hints, chanHints...)
			}

			// Provided all retributions to the client from all
			// channels.
			for id := uint64(0); id < 10; id++ {
				h.backupStates(id, 0, numUpdates, nil)
			}

			// Test reliable flush under multi-client scenario.
			go h.client.Stop()

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 10*time.Second)
		},
	},
	{
		name: "create session no ack",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
			noAckCreateSession: true,
		},
		fn: func(h *testHarness) {
			const (
				chanID     = 0
				numUpdates = 3
			)

			// Generate the retributions that will be backed up.
			hints := h.advanceChannelN(chanID, numUpdates)

			// Now, queue the retributions for backup.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Since the client is unable to create a session, the
			// server should have no updates.
			h.server.waitServerUpdates(nil, time.Second)

			// Force quit the client since it has queued backups.
			h.client.ForceQuit()

			// Restart the server and allow it to ack session
			// creation.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckCreateSession = false
			})

			// Restart the client with the same policy, which will
			// immediately try to overwrite the old session with an
			// identical one.
			h.startClient()
			defer h.client.ForceQuit()

			// Now, queue the retributions for backup.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 5*time.Second)

			// Assert that the server has updates for the clients
			// most recent policy.
			h.server.assertUpdatesForPolicy(
				hints, h.clientCfg.Policy,
			)
		},
	},
	{
		name: "create session no ack change policy",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
			noAckCreateSession: true,
		},
		fn: func(h *testHarness) {
			const (
				chanID     = 0
				numUpdates = 3
			)

			// Generate the retributions that will be backed up.
			hints := h.advanceChannelN(chanID, numUpdates)

			// Now, queue the retributions for backup.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Since the client is unable to create a session, the
			// server should have no updates.
			h.server.waitServerUpdates(nil, time.Second)

			// Force quit the client since it has queued backups.
			h.client.ForceQuit()

			// Restart the server and allow it to ack session
			// creation.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckCreateSession = false
			})

			// Restart the client with a new policy, which will
			// immediately try to overwrite the prior session with
			// the old policy.
			h.clientCfg.Policy.SweepFeeRate *= 2
			h.startClient()
			defer h.client.ForceQuit()

			// Now, queue the retributions for backup.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 5*time.Second)

			// Assert that the server has updates for the clients
			// most recent policy.
			h.server.assertUpdatesForPolicy(
				hints, h.clientCfg.Policy,
			)
		},
	},
	{
		// Asserts that the client will not request a new session if
		// already has an existing session with the same TxPolicy. This
		// permits the client to continue using policies that differ in
		// operational parameters, but don't manifest in different
		// justice transactions.
		name: "create session change policy same txpolicy",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 10,
			},
		},
		fn: func(h *testHarness) {
			const (
				chanID     = 0
				numUpdates = 6
			)

			// Generate the retributions that will be backed up.
			hints := h.advanceChannelN(chanID, numUpdates)

			// Now, queue the first half of the retributions.
			h.backupStates(chanID, 0, numUpdates/2, nil)

			// Wait for the server to collect the first half.
			h.server.waitServerUpdates(
				hints[:numUpdates/2], time.Second,
			)

			// Stop the client, which should have no more backups.
			h.client.Stop()

			// Record the policy that the first half was stored
			// under. We'll expect the second half to also be stored
			// under the original policy, since we are only
			// adjusting the MaxUpdates. The client should detect
			// that the two policies have equivalent TxPolicies and
			// continue using the first.
			expPolicy := h.clientCfg.Policy

			// Restart the client with a new policy.
			h.clientCfg.Policy.MaxUpdates = 20
			h.startClient()
			defer h.client.ForceQuit()

			// Now, queue the second half of the retributions.
			h.backupStates(chanID, numUpdates/2, numUpdates, nil)

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 5*time.Second)

			// Assert that the server has updates for the client's
			// original policy.
			h.server.assertUpdatesForPolicy(hints, expPolicy)
		},
	},
	{
		// Asserts that the client will deduplicate backups presented by
		// a channel both in memory and after a restart. The client
		// should only accept backups with a commit height greater than
		// any processed already processed for a given policy.
		name: "dedup backups",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 10
				chanID     = 0
			)

			// Generate the retributions that will be backed up.
			hints := h.advanceChannelN(chanID, numUpdates)

			// Queue the first half of the retributions twice, the
			// second batch should be entirely deduped by the
			// client's in-memory tracking.
			h.backupStates(chanID, 0, numUpdates/2, nil)
			h.backupStates(chanID, 0, numUpdates/2, nil)

			// Wait for the first half of the updates to be
			// populated in the server's database.
			h.server.waitServerUpdates(
				hints[:len(hints)/2], 5*time.Second,
			)

			// Restart the client, so we can ensure the deduping is
			// maintained across restarts.
			h.client.Stop()
			h.startClient()
			defer h.client.ForceQuit()

			// Try to back up the full range of retributions. Only
			// the second half should actually be sent.
			h.backupStates(chanID, 0, numUpdates, nil)

			// Wait for all of the updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(hints, 5*time.Second)
		},
	},
	{
		// Asserts that the client can continue making backups to a
		// tower that's been re-added after it's been removed.
		name: "re-add removed tower",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				chanID     = 0
				numUpdates = 4
			)

			// Create four channel updates and only back up the
			// first two.
			hints := h.advanceChannelN(chanID, numUpdates)
			h.backupStates(chanID, 0, numUpdates/2, nil)
			h.server.waitServerUpdates(
				hints[:numUpdates/2], 5*time.Second,
			)

			// Fully remove the tower, causing its existing sessions
			// to be marked inactive.
			h.removeTower(h.server.addr.IdentityKey, nil)

			// Back up the remaining states. Since the tower has
			// been removed, it shouldn't receive any updates.
			h.backupStates(chanID, numUpdates/2, numUpdates, nil)
			h.server.waitServerUpdates(nil, time.Second)

			// Re-add the tower. We prevent the tower from acking
			// session creation to ensure the inactive sessions are
			// not used.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckCreateSession = true
			})
			h.addTower(h.server.addr)
			h.server.waitServerUpdates(nil, time.Second)

			// Finally, allow the tower to ack session creation,
			// allowing the state updates to be sent through the new
			// session.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckCreateSession = false
			})
			h.server.waitServerUpdates(
				hints[numUpdates/2:], 5*time.Second,
			)
		},
	},
	{
		// Asserts that the client's force quite delay will properly
		// shutdown the client if it is unable to completely drain the
		// task pipeline.
		name: "force unclean shutdown",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				chanID     = 0
				numUpdates = 6
				maxUpdates = 5
			)

			// Advance the channel to create all states.
			hints := h.advanceChannelN(chanID, numUpdates)

			// Back up 4 of the 5 states for the negotiated session.
			h.backupStates(chanID, 0, maxUpdates-1, nil)
			h.server.waitServerUpdates(
				hints[:maxUpdates-1], 5*time.Second,
			)

			// Now, restart the tower and prevent it from acking any
			// new sessions. We do this here as once the last slot
			// is exhausted the client will attempt to renegotiate.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckCreateSession = true
			})

			// Back up the remaining two states. Once the first is
			// processed, the session will be exhausted but the
			// client won't be able to regnegotiate a session for
			// the final state. We'll only wait for the first five
			// states to arrive at the tower.
			h.backupStates(chanID, maxUpdates-1, numUpdates, nil)
			h.server.waitServerUpdates(
				hints[:maxUpdates], 5*time.Second,
			)

			// Finally, stop the client which will continue to
			// attempt session negotiation since it has one more
			// state to process. After the force quite delay
			// expires, the client should force quite itself and
			// allow the test to complete.
			err := h.client.Stop()
			require.Nil(h.t, err)
		},
	},
	{
		// Assert that the client is able to switch to a new tower if
		// the primary one goes down.
		name: "switch to new tower",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Generate numUpdates retributions and back a few of
			// them up to the main tower.
			hints := h.advanceChannelN(chanID, numUpdates)
			h.backupStates(chanID, 0, numUpdates-2, nil)

			// Wait for all the backed up updates to be populated in
			// the server's database.
			h.server.waitServerUpdates(
				hints[:numUpdates-2], 5*time.Second,
			)

			// Now we add a new tower.
			server2 := h.newServerHarness(tower2AddrStr)
			server2.start()
			h.addTower(server2.addr)

			// Stop the old tower and remove it from the client.
			h.server.stop()
			h.removeTower(h.server.addr.IdentityKey, nil)

			// Back up the remaining states.
			h.backupStates(chanID, numUpdates-2, numUpdates, nil)

			// Assert that the new tower has the remaining states.
			server2.waitServerUpdates(
				hints[numUpdates-2:], 5*time.Second,
			)
		},
	},
	{
		// Show that if a client switches to a new tower _after_ backup
		// tasks have been bound to the session with the first old tower
		// then these updates are replayed onto the new tower.
		name: "switch to new tower after tasks are bound",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Generate numUpdates retributions and back a few of
			// them up to the main tower.
			hints := h.advanceChannelN(chanID, numUpdates)
			h.backupStates(chanID, 0, numUpdates-2, nil)

			// Wait for all these updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(
				hints[:numUpdates-2], 5*time.Second,
			)

			// Now stop the server.
			h.server.stop()

			// Back up the remaining tasks. This will bind the
			// backup tasks to the session with the old server.
			h.backupStates(chanID, numUpdates-2, numUpdates, nil)

			// Now we add a new tower.
			server2 := h.newServerHarness(tower2AddrStr)
			server2.start()
			h.addTower(server2.addr)

			// Now we can remove the old one.
			err := wait.Predicate(func() bool {
				err := h.client.RemoveTower(
					h.server.addr.IdentityKey, nil,
				)
				return err == nil
			}, time.Second*5)
			require.NoError(h.t, err)

			// Now we assert that the backups are backed up to the
			// new tower.
			server2.waitServerUpdates(
				hints[numUpdates-2:], 5*time.Second,
			)
		},
	},
	{
		// Assert that a client is able to remove a tower if there
		// are persisted un-acked updates as long as the client is not
		// restarted.
		name: "can remove tower with un-acked updates before restart",
		cfg: harnessCfg{
			localBalance:  localBalance,
			remoteBalance: remoteBalance,
			policy: wtpolicy.Policy{
				TxPolicy:   defaultTxPolicy,
				MaxUpdates: 5,
			},
		},
		fn: func(h *testHarness) {
			const (
				numUpdates = 5
				chanID     = 0
			)

			// Generate numUpdates retributions and back a few of
			// them up to the main tower.
			hints := h.advanceChannelN(chanID, numUpdates)
			h.backupStates(chanID, 0, numUpdates-2, nil)

			// Wait for all these updates to be populated in the
			// server's database.
			h.server.waitServerUpdates(
				hints[:numUpdates-2], 5*time.Second,
			)

			// Now stop the server and restart it with the
			// NoAckUpdates set to true.
			h.server.restart(func(cfg *wtserver.Config) {
				cfg.NoAckUpdates = true
			})

			// Back up the remaining tasks. This will bind the
			// backup tasks to the session with the server. The
			// client will also persist the updates.
			h.backupStates(chanID, numUpdates-2, numUpdates, nil)

			tower, err := h.clientDB.LoadTower(
				h.server.addr.IdentityKey,
			)
			require.NoError(h.t, err)

			// Wait till the updates have been persisted.
			err = wait.Predicate(func() bool {
				sm, err := h.clientDB.ListClientSessions(
					&tower.ID,
				)
				require.NoError(h.t, err)

				for _, s := range sm {
					return len(s.CommittedUpdates) != 0
				}

				return false

			}, time.Second*5)
			require.NoError(h.t, err)

			// Now remove the tower.
			err = h.client.RemoveTower(
				h.server.addr.IdentityKey, nil,
			)
			require.NoError(h.t, err)

			// Add a new tower.
			server2 := h.newServerHarness(tower2AddrStr)
			server2.start()
			h.addTower(server2.addr)

			// Now we assert that the backups are backed up to the
			// new tower.
			server2.waitServerUpdates(
				hints[numUpdates-2:], 5*time.Second,
			)
		},
	},
}

// TestClient executes the client test suite, asserting the ability to backup
// states in a number of failure cases and it's reliability during shutdown.
func TestClient(t *testing.T) {
	for _, test := range clientTests {
		tc := test
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, tc.cfg)
			defer h.server.stop()
			defer h.client.ForceQuit()

			tc.fn(h)
		})
	}
}
