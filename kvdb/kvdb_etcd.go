//go:build kvdb_etcd
// +build kvdb_etcd

package kvdb

import (
	"github.com/lightningnetwork/lnd/kvdb/etcd"
)

const EtcdBackend = true

// GetEtcdTestBackend creates an embedded etcd backend for testing
// storig the database at the passed path.
func StartEtcdTestBackend(path string, clientPort, peerPort uint16,
	logFile string) (*etcd.Config, func(), error) {

	return etcd.NewEmbeddedEtcdInstance(
		path, clientPort, peerPort, logFile,
	)
}
