package migration4

import (
	"bytes"
	"testing"

	"github.com/lightningnetwork/lnd/channeldb/migtest"
	"github.com/lightningnetwork/lnd/kvdb"
)

var (
	// details is the expected data of the channel details bucket. This
	// bucket should not be changed during the migration, but it is used
	// to find the db-assigned ID for each channel.
	details = map[string]interface{}{
		channelIDString(1): map[string]interface{}{
			string(cChanDBID): uint64ToStr(10),
		},
		channelIDString(2): map[string]interface{}{
			string(cChanDBID): uint64ToStr(20),
		},
	}

	// preSessions is the expected data in the sessions bucket before the
	// migration.
	preSessions = map[string]interface{}{
		sessionIDString("1"): map[string]interface{}{
			string(cSessionAcks): map[string]interface{}{
				"1": backupIDToString(&BackupID{
					ChanID:       intToChannelID(1),
					CommitHeight: 30,
				}),
				"2": backupIDToString(&BackupID{
					ChanID:       intToChannelID(1),
					CommitHeight: 31,
				}),
				"3": backupIDToString(&BackupID{
					ChanID:       intToChannelID(1),
					CommitHeight: 32,
				}),
				"4": backupIDToString(&BackupID{
					ChanID:       intToChannelID(1),
					CommitHeight: 34,
				}),
				"5": backupIDToString(&BackupID{
					ChanID:       intToChannelID(2),
					CommitHeight: 30,
				}),
			},
		},
		sessionIDString("2"): map[string]interface{}{
			string(cSessionAcks): map[string]interface{}{
				"1": backupIDToString(&BackupID{
					ChanID:       intToChannelID(1),
					CommitHeight: 33,
				}),
			},
		},
	}

	// preFailCorruptDB should fail the migration due to no session data
	// being found for a given session ID.
	preFailCorruptDB = map[string]interface{}{
		sessionIDString("2"): "",
	}

	// postSessions is the expected data in the sessions bucket after the
	// migration.
	postSessions = map[string]interface{}{
		sessionIDString("1"): map[string]interface{}{
			string(cSessionAckRangeIndex): map[string]interface{}{
				uint64ToStr(10): map[string]interface{}{
					uint64ToStr(30): uint64ToStr(32),
					uint64ToStr(34): uint64ToStr(34),
				},
				uint64ToStr(20): map[string]interface{}{
					uint64ToStr(30): uint64ToStr(30),
				},
			},
		},
		sessionIDString("2"): map[string]interface{}{
			string(cSessionAckRangeIndex): map[string]interface{}{
				uint64ToStr(10): map[string]interface{}{
					uint64ToStr(33): uint64ToStr(33),
				},
			},
		},
	}
)

// TestMigrateAckedUpdates tests that the MigrateAckedUpdates function correctly
// migrates the existing AckedUpdates bucket for each session to the new
// RangeIndex representation.
func TestMigrateAckedUpdates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		shouldFail bool
		pre        map[string]interface{}
		post       map[string]interface{}
	}{
		{
			name:       "migration ok",
			shouldFail: false,
			pre:        preSessions,
			post:       postSessions,
		},
		{
			name:       "fail due to corrupt db",
			shouldFail: true,
			pre:        preFailCorruptDB,
		},
		{
			name:       "no sessions details",
			shouldFail: false,
			pre:        nil,
			post:       nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			// Before the migration we have a details bucket.
			before := func(tx kvdb.RwTx) error {
				err := migtest.RestoreDB(
					tx, cChanDetailsBkt, details,
				)
				if err != nil {
					return err
				}

				return migtest.RestoreDB(
					tx, cSessionBkt, test.pre,
				)
			}

			// After the migration, we should have an untouched
			// summary bucket and a new index bucket.
			after := func(tx kvdb.RwTx) error {
				// The channel details bucket should remain
				// untouched.
				err := migtest.VerifyDB(
					tx, cChanDetailsBkt, details,
				)
				if err != nil {
					return err
				}

				// If the migration fails, the sessions bucket
				// should be untouched.
				if test.shouldFail {
					if err := migtest.VerifyDB(
						tx, cSessionBkt, test.pre,
					); err != nil {
						return err
					}

					return nil
				}

				return migtest.VerifyDB(
					tx, cSessionBkt, test.post,
				)
			}

			migtest.ApplyMigration(
				t, before, after, MigrateAckedUpdates,
				test.shouldFail,
			)
		})
	}
}

func sessionIDString(id string) string {
	var sessID SessionID
	copy(sessID[:], id)
	return sessID.String()
}

func intToChannelID(id uint64) ChannelID {
	var chanID ChannelID
	byteOrder.PutUint64(chanID[:], id)
	return chanID
}

func channelIDString(id uint64) string {
	var chanID ChannelID
	byteOrder.PutUint64(chanID[:], id)
	return string(chanID[:])
}

func uint64ToStr(id uint64) string {
	var b [8]byte
	byteOrder.PutUint64(b[:], id)
	return string(b[:])
}

func backupIDToString(backup *BackupID) string {
	var b bytes.Buffer
	_ = backup.Encode(&b)
	return b.String()
}
