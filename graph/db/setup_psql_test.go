//go:build test_db_postgres && !test_db_sqlite

package graphdb

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/sqldb"
)

var (
	setupOnce    sync.Once
	teardownOnce sync.Once
)

func initTestFixture() {
	setupOnce.Do(func() {
		var err error
		testFixture, err = sqldb.NewTestPgFixture(60 * time.Minute)
		if err != nil {
			fmt.Println("Error setting up test fixture:", err)
			os.Exit(1)
		}
	})
}

func tearDownTestFixture() {
	teardownOnce.Do(func() {
		if testFixture != nil {
			err := testFixture.TearDown()
			if err != nil {
				fmt.Println("Error tearing down test fixture:",
					err)
				os.Exit(1)
			}
		}
	})
}

func TestMain(m *testing.M) {
	// Setup (e.g. start test DB, initialize global vars)
	initTestFixture() // dummy T for now; can use a wrapper

	code := m.Run() // This runs all the actual tests

	// Teardown (e.g. close DB, remove temp files)
	tearDownTestFixture() // dummy T for now; can use a wrapper

	os.Exit(code) // Don't forget to exit with the test code
}
