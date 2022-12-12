//go:build !kvdb_etcd
// +build !kvdb_etcd

package kvdb

import (
	"fmt"

	"github.com/lightningnetwork/lnd/kvdb/etcd"
)

const EtcdBackend = false

var errEtcdNotAvailable = fmt.Errorf("etcd backend not available")

// StartEtcdTestBackend  is a stub returning nil, and errEtcdNotAvailable error.
func StartEtcdTestBackend(path string, clientPort, peerPort uint16,
	logFile string) (*etcd.Config, func(), error) {

	return nil, func() {}, errEtcdNotAvailable
}
