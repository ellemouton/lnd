package blob

import (
	"io"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwire"
)

// JusticeKit is an interface that describes lé Blob of Justice. An
// implementation of the JusticeKit contains information required to construct
// a justice transaction, that sweeps a remote party's revoked commitment
// transaction. It supports encryption and decryption using chacha20poly1305,
// allowing the client to encrypt the contents of the blob, and for a
// watchtower to later decrypt if action must be taken.
type JusticeKit interface {
	// ToLocalOutputSpendInfo returns the info required to send the to-local
	// output. It returns the output pub key script and the witness required
	// to spend the output.
	ToLocalOutputSpendInfo() (*txscript.PkScript, wire.TxWitness, error)

	// ToRemoteOutputSpendInfo returns the info required to send the
	// to-remote output. It returns the output pub key script, the witness
	// required to spend the output and the sequence to apply.
	ToRemoteOutputSpendInfo() (*txscript.PkScript, wire.TxWitness, uint32,
		error)

	// HasCommitToRemoteOutput returns true if the kit does include the
	// information required to sweep the to-remote output.
	HasCommitToRemoteOutput() bool

	// AddToLocalSig adds the to-local signature to the kit.
	AddToLocalSig(sig lnwire.Sig)

	// AddToRemoteSig adds the to-remote signature to the kit.
	AddToRemoteSig(sig lnwire.Sig)

	// SweepAddress returns the sweep address to be used on the justice tx
	// output.
	SweepAddress() []byte

	// PlainTextSize is the size of the encoded-but-unencrypted blob in
	// bytes.
	PlainTextSize() int

	encode(w io.Writer) error
	decode(r io.Reader) error
}

// legacyJusticeKit is an implementation of the JusticeKit interface which can
// be used for backing up commitments of legacy (pre-anchor) channels.
type legacyJusticeKit struct {
	justiceKitPacketV0
}

// A compile-time check to ensure that legacyJusticeKit implements the
// JusticeKit interface.
var _ JusticeKit = (*legacyJusticeKit)(nil)

// newLegacyJusticeKit constructs a new legacyJusticeKit.
func newLegacyJusticeKit(sweepScript []byte,
	breachInfo *lnwallet.BreachRetribution,
	withToRemote bool) *legacyJusticeKit {

	keyRing := breachInfo.KeyRing

	packet := justiceKitPacketV0{
		sweepAddress:         sweepScript,
		revocationPubKey:     toBlobPubKey(keyRing.RevocationKey),
		localDelayPubKey:     toBlobPubKey(keyRing.ToLocalKey),
		csvDelay:             breachInfo.RemoteDelay,
		commitToRemotePubKey: pubKey{},
	}

	if withToRemote {
		packet.commitToRemotePubKey = toBlobPubKey(
			keyRing.ToRemoteKey,
		)
	}

	return &legacyJusticeKit{packet}
}

// ToLocalOutputSpendInfo returns the info required to send the to-local output.
// It returns the output pub key script and the witness required to spend the
// output.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) ToLocalOutputSpendInfo() (*txscript.PkScript,
	wire.TxWitness, error) {

	revocationPubKey, err := btcec.ParsePubKey(l.revocationPubKey[:])
	if err != nil {
		return nil, nil, err
	}

	localDelayedPubKey, err := btcec.ParsePubKey(l.localDelayPubKey[:])
	if err != nil {
		return nil, nil, err
	}

	script, err := input.CommitScriptToSelf(
		l.csvDelay, localDelayedPubKey, revocationPubKey,
	)
	if err != nil {
		return nil, nil, err
	}

	scriptPubKey, err := input.WitnessScriptHash(script)
	if err != nil {
		return nil, nil, err
	}

	toLocalSig, err := l.commitToLocalSig.ToSignature()
	if err != nil {
		return nil, nil, err
	}

	witness := make(wire.TxWitness, 3)
	witness[0] = append(toLocalSig.Serialize(), byte(txscript.SigHashAll))
	witness[1] = []byte{1}
	witness[2] = script

	pkScript, err := txscript.ParsePkScript(scriptPubKey)
	if err != nil {
		return nil, nil, err
	}

	return &pkScript, witness, nil
}

// ToRemoteOutputSpendInfo returns the info required to send the to-remote
// output. It returns the output pub key script, the witness required to spend
// the output and the sequence to apply.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) ToRemoteOutputSpendInfo() (*txscript.PkScript,
	wire.TxWitness, uint32, error) {

	if !btcec.IsCompressedPubKey(l.commitToRemotePubKey[:]) {
		return nil, nil, 0, ErrNoCommitToRemoteOutput
	}

	toRemoteScript := l.commitToRemotePubKey[:]

	// Since the to-remote witness script should just be a regular p2wkh
	// output, we'll parse it to retrieve the public key.
	toRemotePubKey, err := btcec.ParsePubKey(toRemoteScript)
	if err != nil {
		return nil, nil, 0, err
	}

	// Compute the witness script hash from the to-remote pubkey, which will
	// be used to locate the input on the breach commitment transaction.
	toRemoteScriptHash, err := input.CommitScriptUnencumbered(
		toRemotePubKey,
	)
	if err != nil {
		return nil, nil, 0, err
	}

	toRemoteSig, err := l.commitToRemoteSig.ToSignature()
	if err != nil {
		return nil, nil, 0, err
	}

	witness := make(wire.TxWitness, 2)
	witness[0] = append(toRemoteSig.Serialize(), byte(txscript.SigHashAll))
	witness[1] = toRemoteScript

	pkScript, err := txscript.ParsePkScript(toRemoteScriptHash)
	if err != nil {
		return nil, nil, 0, err
	}

	return &pkScript, witness, 0, nil
}

// HasCommitToRemoteOutput returns true if the blob contains a to-remote p2wkh
// pubkey.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) HasCommitToRemoteOutput() bool {
	return btcec.IsCompressedPubKey(l.commitToRemotePubKey[:])
}

// SweepAddress returns the sweep address to be used on the justice tx
// output.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) SweepAddress() []byte {
	return l.sweepAddress
}

// AddToLocalSig adds the to-local signature to the kit.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) AddToLocalSig(sig lnwire.Sig) {
	l.commitToLocalSig = sig
}

// AddToRemoteSig adds the to-remote signature to the kit.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) AddToRemoteSig(sig lnwire.Sig) {
	l.commitToRemoteSig = sig
}

// PlainTextSize is the size of the encoded-but-unencrypted blob in
// bytes.
//
// NOTE: This is part of the JusticeKit interface.
func (l *legacyJusticeKit) PlainTextSize() int {
	return V0PlaintextSize
}

// anchorJusticeKit is an implementation of the JusticeKit interface which can
// be used for backing up commitments of anchor channels. It inherits most of
// the methods from the legacyJusticeKit and overrides the
// ToRemoteOutputSpendInfo method since the to-remote output of an anchor
// output is a P2WSH instead of the P2WPKH used by the legacy channels.
type anchorJusticeKit struct {
	legacyJusticeKit
}

// A compile-time check to ensure that legacyJusticeKit implements the
// JusticeKit interface.
var _ JusticeKit = (*anchorJusticeKit)(nil)

// newAnchorJusticeKit constructs a new anchorJusticeKit.
func newAnchorJusticeKit(sweepScript []byte,
	breachInfo *lnwallet.BreachRetribution,
	withToRemote bool) *anchorJusticeKit {

	legacyKit := newLegacyJusticeKit(sweepScript, breachInfo, withToRemote)

	return &anchorJusticeKit{
		legacyJusticeKit: *legacyKit,
	}
}

// ToRemoteOutputSpendInfo returns the info required to send the to-remote
// output. It returns the output pub key script, the witness required to spend
// the output and the sequence to apply.
//
// NOTE: This is part of the JusticeKit interface.
func (a *anchorJusticeKit) ToRemoteOutputSpendInfo() (*txscript.PkScript,
	wire.TxWitness, uint32, error) {

	if !btcec.IsCompressedPubKey(a.commitToRemotePubKey[:]) {
		return nil, nil, 0, ErrNoCommitToRemoteOutput
	}

	pk, err := btcec.ParsePubKey(a.commitToRemotePubKey[:])
	if err != nil {
		return nil, nil, 0, err
	}

	toRemoteScript, err := input.CommitScriptToRemoteConfirmed(pk)
	if err != nil {
		return nil, nil, 0, err
	}

	toRemoteScriptHash, err := input.WitnessScriptHash(toRemoteScript)
	if err != nil {
		return nil, nil, 0, err
	}

	toRemoteSig, err := a.commitToRemoteSig.ToSignature()
	if err != nil {
		return nil, nil, 0, err
	}

	witness := make([][]byte, 2)
	witness[0] = append(toRemoteSig.Serialize(), byte(txscript.SigHashAll))
	witness[1] = toRemoteScript

	pkScript, err := txscript.ParsePkScript(toRemoteScriptHash)
	if err != nil {
		return nil, nil, 0, err
	}

	return &pkScript, witness, 1, nil
}
