package sqldb

import (
	"context"
	"testing"

	"github.com/lightningnetwork/lnd/sqldb/sqlc"
	"github.com/stretchr/testify/require"
)

// makeMigrationTestDB is a type alias for a function that creates a new test
// database and returns the base database and a function that executes selected
// migrations.
type makeMigrationTestDB = func(*testing.T, uint) (*BaseDB,
	func(MigrationTarget) error)

// TestMigrations is a meta test runner that runs all migration tests.
func TestMigrations(t *testing.T) {
	sqliteTestDB := func(t *testing.T, version uint) (*BaseDB,
		func(MigrationTarget) error) {

		db := NewTestSqliteDBWithVersion(t, version)

		return db.BaseDB, db.ExecuteMigrations
	}

	postgresTestDB := func(t *testing.T, version uint) (*BaseDB,
		func(MigrationTarget) error) {

		pgFixture := NewTestPgFixture(t, DefaultPostgresFixtureLifetime)
		t.Cleanup(func() {
			pgFixture.TearDown(t)
		})

		db := NewTestPostgresDBWithVersion(
			t, pgFixture, version,
		)

		return db.BaseDB, db.ExecuteMigrations
	}

	tests := []struct {
		name string
		test func(*testing.T, makeMigrationTestDB)
	}{
		{
			name: "TestInvoiceExpiryMigration",
			test: testInvoiceExpiryMigration,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name+"_SQLite", func(t *testing.T) {
			test.test(t, sqliteTestDB)
		})

		t.Run(test.name+"_Postgres", func(t *testing.T) {
			test.test(t, postgresTestDB)
		})

	}
}

const insertInvoice = `-- name: InsertInvoice :one
INSERT INTO invoices (
    hash, preimage, memo, amount_msat, cltv_delta, expiry, payment_addr, 
    payment_request, payment_request_hash, state, amount_paid_msat, is_amp,
    is_hodl, is_keysend, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
) RETURNING id
`

func insertInvoiceV3(ctx context.Context, db *BaseDB,
	arg sqlc.InsertInvoiceParams) (int64, error) {

	row := db.QueryRowContext(ctx, insertInvoice,
		arg.Hash,
		arg.Preimage,
		arg.Memo,
		arg.AmountMsat,
		arg.CltvDelta,
		arg.Expiry,
		arg.PaymentAddr,
		arg.PaymentRequest,
		arg.PaymentRequestHash,
		arg.State,
		arg.AmountPaidMsat,
		arg.IsAmp,
		arg.IsHodl,
		arg.IsKeysend,
		arg.CreatedAt,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
}

// TestInvoiceExpiryMigration tests that the migration from version 3 to 4
// correctly sets the expiry value of normal invoices to 86400 seconds and
// 2592000 seconds for AMP invoices.
func testInvoiceExpiryMigration(t *testing.T, makeDB makeMigrationTestDB) {
	t.Parallel()
	ctxb := context.Background()

	// Create a new database that already has the first version of the
	// native invoice schema.
	db, migrate := makeDB(t, 3)

	// Add a few invoices. For simplicity we reuse the payment hash as the
	// payment address and payment request hash instead of setting them to
	// NULL (to not run into uniqueness constraints). Note that SQLC
	// currently doesn't support nullable blob fields porperly. A workaround
	// is in progress: https://github.com/sqlc-dev/sqlc/issues/3149

	// Add an invoice where is_amp will be set to false.
	hash1 := []byte{1, 2, 3}
	_, err := insertInvoiceV3(ctxb, db, sqlc.InsertInvoiceParams{
		Hash:               hash1,
		PaymentAddr:        hash1,
		PaymentRequestHash: hash1,
		Expiry:             -123,
		IsAmp:              false,
	})
	require.NoError(t, err)

	// Add an invoice where is_amp will be set to false.
	hash2 := []byte{4, 5, 6}
	_, err = insertInvoiceV3(ctxb, db, sqlc.InsertInvoiceParams{
		Hash:               hash2,
		PaymentAddr:        hash2,
		PaymentRequestHash: hash2,
		Expiry:             -456,
		IsAmp:              true,
	})
	require.NoError(t, err)

	// Now, we'll attempt to execute the migration that will fix the expiry
	// values by inserting 86400 seconds for non AMP and 2592000 seconds for
	// AMP invoices.
	err = migrate(TargetVersion(5))

	invoices, err := db.FilterInvoices(ctxb, sqlc.FilterInvoicesParams{
		AddIndexGet: SQLInt64(1),
		NumLimit:    100,
	})

	const (
		// 1 day in seconds.
		expiry = int32(86400)
		// 30 days in seconds.
		expiryAMP = int32(2592000)
	)

	expected := []sqlc.Invoice{
		{
			ID:                 1,
			Hash:               hash1,
			PaymentAddr:        hash1,
			PaymentRequestHash: hash1,
			Expiry:             expiry,
		},
		{
			ID:                 2,
			Hash:               hash2,
			PaymentAddr:        hash2,
			PaymentRequestHash: hash2,
			Expiry:             expiryAMP,
			IsAmp:              true,
		},
	}

	for i := range invoices {
		// Override the timestamp location as the sql driver will scan
		// the timestamp with no location and we can't create such
		// timestamps in Golang using the standard time package.
		invoices[i].CreatedAt = invoices[i].CreatedAt.UTC()

		// Override the preimage as depending on the driver it is either
		// scanned as nil or an empty byte slice.
		require.Len(t, invoices[i].Preimage, 0)
		invoices[i].Preimage = nil
	}

	require.NoError(t, err)
	require.Equal(t, expected, invoices)
}
