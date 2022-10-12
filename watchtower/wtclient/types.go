package wtclient

import (
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/keychain"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
)

type Tower struct {
	// ID is the unique, db-assigned, identifier for this tower.
	ID wtdb.TowerID

	// IdentityKey is the public key of the remote node, used to
	// authenticate the brontide transport.
	IdentityKey *btcec.PublicKey

	Addresses *AddressIterator
}

func NewTowerFromDBTower(t *wtdb.Tower) (*Tower, error) {
	addrs, err := newAddressIterator(t.Addresses...)
	if err != nil {
		return nil, err
	}

	return &Tower{
		ID:          t.ID,
		IdentityKey: t.IdentityKey,
		Addresses:   addrs,
	}, nil
}

type ClientSession struct {
	// ID is the client's public key used when authenticating with the
	// tower.
	ID wtdb.SessionID

	wtdb.ClientSessionBody

	// Tower holds the pubkey and address of the watchtower.
	Tower *Tower

	// SessionKeyECDH is the ECDH capable wrapper of the ephemeral secret
	// key used to connect to the watchtower.
	SessionKeyECDH keychain.SingleKeyECDH
}
