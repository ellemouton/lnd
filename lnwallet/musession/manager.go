package musession

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/keychain"
)

// MuSig2State is a struct that holds on to the internal signing session state
// of a MuSig2 session.
type MuSig2State struct {
	// MuSig2SessionInfo is the associated meta information of the signing
	// session.
	input.MuSig2SessionInfo

	// context is the signing context responsible for keeping track of the
	// public keys involved in the signing process.
	context input.MuSig2Context

	// session is the signing session responsible for keeping track of the
	// nonces and partial signatures involved in the signing process.
	session input.MuSig2Session
}

// PrivKeyFetcher is used to fetch a private key that matches a given key desc.
type PrivKeyFetcher func(*keychain.KeyDescriptor) (*btcec.PrivateKey, error)

// Manager houses the state needed to manage concurrent musig sessions. Each
// session is identified by a unique session ID which is used by callers to
// interact with a given session.
type Manager struct {
	sync.Mutex

	keyFetcher PrivKeyFetcher

	musig2Sessions map[input.MuSig2SessionID]*MuSig2State
}

// NewManager creates a new musig manager given an abstract key fetcher.
func NewManager(keyFetcher PrivKeyFetcher) *Manager {
	return &Manager{
		keyFetcher:     keyFetcher,
		musig2Sessions: make(map[input.MuSig2SessionID]*MuSig2State),
	}
}

// MuSig2CreateSession creates a new MuSig2 signing session using the local
// key identified by the key locator. The complete list of all public keys of
// all signing parties must be provided, including the public key of the local
// signing key. If nonces of other parties are already known, they can be
// submitted as well to reduce the number of method calls necessary later on.
//
// The set of sessionOpts are _optional_ and allow a caller to modify the
// generated sessions. As an example the local nonce might already be generated
// ahead of time.
func (m *Manager) MuSig2CreateSession(bipVersion input.MuSig2Version,
	keyLoc keychain.KeyLocator, allSignerPubKeys []*btcec.PublicKey,
	tweaks *input.MuSig2Tweaks, otherSignerNonces [][musig2.PubNonceSize]byte,
	sessionOpts ...musig2.SessionOption) (*input.MuSig2SessionInfo, error) {

	// We need to derive the private key for signing. In the remote signing
	// setup, this whole RPC call will be forwarded to the signing
	// instance, which requires it to be stateful.
	privKey, err := m.keyFetcher(&keychain.KeyDescriptor{
		KeyLocator: keyLoc,
	})
	if err != nil {
		return nil, fmt.Errorf("error deriving private key: %v", err)
	}

	// Create a signing context and session with the given private key and
	// list of all known signer public keys.
	musigContext, musigSession, err := input.MuSig2CreateContext(
		bipVersion, privKey, allSignerPubKeys, tweaks,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating signing context: %w",
			err)
	}

	// Add all nonces we might've learned so far.
	haveAllNonces := false
	for _, otherSignerNonce := range otherSignerNonces {
		haveAllNonces, err = musigSession.RegisterPubNonce(
			otherSignerNonce,
		)
		if err != nil {
			return nil, fmt.Errorf("error registering other "+
				"signer public nonce: %v", err)
		}
	}

	// Register the new session.
	combinedKey, err := musigContext.CombinedKey()
	if err != nil {
		return nil, fmt.Errorf("error getting combined key: %v", err)
	}
	session := &MuSig2State{
		MuSig2SessionInfo: input.MuSig2SessionInfo{
			SessionID: input.NewMuSig2SessionID(
				combinedKey, musigSession.PublicNonce(),
			),
			Version:       bipVersion,
			PublicNonce:   musigSession.PublicNonce(),
			CombinedKey:   combinedKey,
			TaprootTweak:  tweaks.HasTaprootTweak(),
			HaveAllNonces: haveAllNonces,
		},
		context: musigContext,
		session: musigSession,
	}

	// The internal key is only calculated if we are using a taproot tweak
	// and need to know it for a potential script spend.
	if tweaks.HasTaprootTweak() {
		internalKey, err := musigContext.TaprootInternalKey()
		if err != nil {
			return nil, fmt.Errorf("error getting internal key: %v",
				err)
		}
		session.TaprootInternalKey = internalKey
	}

	// Since we generate new nonces for every session, there is no way that
	// a session with the same ID already exists. So even if we call the API
	// twice with the same signers, we still get a new ID.
	m.Lock()
	m.musig2Sessions[session.SessionID] = session
	m.Unlock()

	return &session.MuSig2SessionInfo, nil
}

// MuSig2Sign creates a partial signature using the local signing key
// that was specified when the session was created. This can only be
// called when all public nonces of all participants are known and have
// been registered with the session. If this node isn't responsible for
// combining all the partial signatures, then the cleanup parameter
// should be set, indicating that the session can be removed from memory
// once the signature was produced.
func (m *Manager) MuSig2Sign(sessionID input.MuSig2SessionID,
	msg [sha256.Size]byte, cleanUp bool) (*musig2.PartialSignature, error) {

	// We hold the lock during the whole operation, we don't want any
	// interference with calls that might come through in parallel for the
	// same session.
	m.Lock()
	defer m.Unlock()

	session, ok := m.musig2Sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session with ID %x not found",
			sessionID[:])
	}

	// We can only sign once we have all other signer's nonces.
	if !session.HaveAllNonces {
		return nil, fmt.Errorf("only have %d of %d required nonces",
			session.session.NumRegisteredNonces(),
			len(session.context.SigningKeys()))
	}

	// Create our own partial signature with the local signing key.
	partialSig, err := input.MuSig2Sign(session.session, msg, true)
	if err != nil {
		return nil, fmt.Errorf("error signing with local key: %w", err)
	}

	// Clean up our local state if requested.
	if cleanUp {
		delete(m.musig2Sessions, sessionID)
	}

	return partialSig, nil
}

// MuSig2CombineSig combines the given partial signature(s) with the
// local one, if it already exists. Once a partial signature of all
// participants is registered, the final signature will be combined and
// returned.
func (m *Manager) MuSig2CombineSig(sessionID input.MuSig2SessionID,
	partialSigs []*musig2.PartialSignature) (*schnorr.Signature, bool,
	error) {

	// We hold the lock during the whole operation, we don't want any
	// interference with calls that might come through in parallel for the
	// same session.
	m.Lock()
	defer m.Unlock()

	session, ok := m.musig2Sessions[sessionID]
	if !ok {
		return nil, false, fmt.Errorf("session with ID %x not found",
			sessionID[:])
	}

	// Make sure we don't exceed the number of expected partial signatures
	// as that would indicate something is wrong with the signing setup.
	if session.HaveAllSigs {
		return nil, true, fmt.Errorf("already have all partial" +
			"signatures")
	}

	// Add all sigs we got so far.
	var (
		finalSig *schnorr.Signature
		err      error
	)
	for _, otherPartialSig := range partialSigs {
		session.HaveAllSigs, err = input.MuSig2CombineSig(
			session.session, otherPartialSig,
		)
		if err != nil {
			return nil, false, fmt.Errorf("error combining "+
				"partial signature: %w", err)
		}
	}

	// If we have all partial signatures, we should be able to get the
	// complete signature now. We also remove this session from memory since
	// there is nothing more left to do.
	if session.HaveAllSigs {
		finalSig = session.session.FinalSig()
		delete(m.musig2Sessions, sessionID)
	}

	return finalSig, session.HaveAllSigs, nil
}

// MuSig2Cleanup removes a session from memory to free up resources.
func (m *Manager) MuSig2Cleanup(sessionID input.MuSig2SessionID) error {
	// We hold the lock during the whole operation, we don't want any
	// interference with calls that might come through in parallel for the
	// same session.
	m.Lock()
	defer m.Unlock()

	_, ok := m.musig2Sessions[sessionID]
	if !ok {
		return fmt.Errorf("session with ID %x not found", sessionID[:])
	}

	delete(m.musig2Sessions, sessionID)

	return nil
}

// MuSig2RegisterNonces registers one or more public nonces of other signing
// participants for a session identified by its ID. This method returns true
// once we have all nonces for all other signing participants.
func (m *Manager) MuSig2RegisterNonces(sessionID input.MuSig2SessionID,
	otherSignerNonces [][musig2.PubNonceSize]byte) (bool, error) {

	// We hold the lock during the whole operation, we don't want any
	// interference with calls that might come through in parallel for the
	// same session.
	m.Lock()
	defer m.Unlock()

	session, ok := m.musig2Sessions[sessionID]
	if !ok {
		return false, fmt.Errorf("session with ID %x not found",
			sessionID[:])
	}

	// Make sure we don't exceed the number of expected nonces as that would
	// indicate something is wrong with the signing setup.
	if session.HaveAllNonces {
		return true, fmt.Errorf("already have all nonces")
	}

	numSigners := len(session.context.SigningKeys())
	remainingNonces := numSigners - session.session.NumRegisteredNonces()
	if len(otherSignerNonces) > remainingNonces {
		return false, fmt.Errorf("only %d other nonces remaining but "+
			"trying to register %d more", remainingNonces,
			len(otherSignerNonces))
	}

	// Add all nonces we've learned so far.
	var err error
	for _, otherSignerNonce := range otherSignerNonces {
		session.HaveAllNonces, err = session.session.RegisterPubNonce(
			otherSignerNonce,
		)
		if err != nil {
			return false, fmt.Errorf("error registering other "+
				"signer public nonce: %v", err)
		}
	}

	return session.HaveAllNonces, nil
}
