package channeldb

import (
	"errors"
	"testing"

	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

// TestWaitingProofStore tests add/get/remove functions of the waiting proof
// storage.
func TestWaitingProofStore(t *testing.T) {
	t.Parallel()

	db, err := MakeTestDB(t)
	require.NoError(t, err, "failed to make test database")

	proof1 := NewWaitingProof(true, &lnwire.AnnounceSignatures1{
		NodeSignature:    wireSig,
		BitcoinSignature: wireSig,
		ExtraOpaqueData:  make([]byte, 0),
	})

	store, err := NewWaitingProofStore(db)
	require.NoError(t, err, "unable to create waiting proofs storage")

	require.NoError(t, store.Add(proof1), "unable to add proof to storage")

	proof2, err := store.Get(proof1.Key())
	require.NoError(t, err, "unable retrieve proof from storage")
	require.Equal(t, proof1, proof2)

	_, err = store.Get(proof1.OppositeKey())
	require.ErrorIs(t, err, ErrWaitingProofNotFound)

	require.NoError(t, store.Remove(proof1.Key()))

	if err := store.ForAll(func(proof *WaitingProof) error {
		return errors.New("storage should be empty")
	}, func() {}); err != nil && err != ErrWaitingProofNotFound {
		require.NoError(t, err)
	}
}
