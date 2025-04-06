//go:build !test_db_sqlite && !test_db_postgres

package graphdb

import (
	"testing"

	"github.com/lightningnetwork/lnd/kvdb"
)

func TestMain(m *testing.M) {
	kvdb.RunTests(m)
}
