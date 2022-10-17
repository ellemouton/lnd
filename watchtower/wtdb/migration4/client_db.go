package migration4

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/lightningnetwork/lnd/kvdb"
)

var (
	// cChanDetailsBkt is a top-level bucket storing:
	//   channel-id => cChannelSummary -> encoded ClientChanSummary.
	// 		=> cChanDBID -> db-assigned-id
	cChanDetailsBkt = []byte("client-channel-detail-bucket")

	cChanDBID = []byte("client-channel-db-id")

	// cSessionBkt is a top-level bucket storing:
	//   session-id => cSessionBody -> encoded ClientSessionBody
	//              => cSessionCommits => seqnum -> encoded CommittedUpdate
	//              => cSessionAcks => seqnum -> encoded BackupID
	cSessionBkt = []byte("client-session-bucket")

	// cSessionAcks is a sub-bucket of cSessionBkt storing:
	//    seqnum -> encoded BackupID.
	cSessionAcks = []byte("client-session-acks")

	// cSessionAckRangeIndex is a sub-bucket of cSessionBkt storing:
	//    chan-id => start -> end
	cSessionAckRangeIndex = []byte("client-session-ack-range-index")

	// ErrUninitializedDB signals that top-level buckets for the database
	// have not been initialized.
	ErrUninitializedDB = errors.New("db not initialized")

	// ErrClientSessionNotFound signals that the requested client session
	// was not found in the database.
	ErrClientSessionNotFound = errors.New("client session not found")

	// ErrCorruptChanDetails signals that the clients channel detail's
	// on-disk structure deviates from what is expected.
	ErrCorruptChanDetails = errors.New("channel details corrupted")

	// byteOrder is the default endianness used when serializing integers.
	byteOrder = binary.BigEndian
)

// MigrateAckedUpdates migrates the tower client DB. It takes the individual
// Acked Updates that are stored for each session and re-stores them using the
// RangeIndex representation.
func MigrateAckedUpdates(tx kvdb.RwTx) error {
	log.Infof("Migrating the tower client DB to move all Acked Updates " +
		"to the new Range Index representation.")

	// Get sessions bucket.
	mainSessionsBkt := tx.ReadWriteBucket(cSessionBkt)
	if mainSessionsBkt == nil {
		return ErrUninitializedDB
	}

	// Iterate over each session ID in the bucket.
	return mainSessionsBkt.ForEach(func(sessID, _ []byte) error {
		// Get the bucket for this particular session.
		sessionBkt := mainSessionsBkt.NestedReadWriteBucket(sessID)
		if sessionBkt == nil {
			return ErrClientSessionNotFound
		}

		// Get the existing cSessionAcks bucket. If there is no such
		// bucket, then there are no Acked Updates to migrate for this
		// session.
		sessionAcks := sessionBkt.NestedReadBucket(cSessionAcks)
		if sessionAcks == nil {
			return nil
		}

		// Otherwise, we will iterate over each of the acked updates
		// and we will construct a new RangeIndex for each channel.
		m := make(map[ChannelID]*RangeIndex)
		err := sessionAcks.ForEach(func(_, v []byte) error {
			var backupID BackupID
			err := backupID.Decode(bytes.NewReader(v))
			if err != nil {
				return err
			}

			if _, ok := m[backupID.ChanID]; !ok {
				index, err := NewRangeIndex(nil)
				if err != nil {
					return err
				}

				m[backupID.ChanID] = index
			}

			return m[backupID.ChanID].Add(
				backupID.CommitHeight, nil,
			)
		})
		if err != nil {
			return err
		}

		// Now that we have read everything that we need to from the
		// cSessionAcks sub-bucket, we can delete it.
		err = sessionBkt.DeleteNestedBucket(cSessionAcks)
		if err != nil {
			return err
		}

		// Create a new sub-bucket that will be used to store the new
		// RangeIndex representation of the acked updates.
		ackRangeBkt, err := sessionBkt.CreateBucket(
			cSessionAckRangeIndex,
		)
		if err != nil {
			return err
		}

		// Iterate over each of the new range indexes that we will add
		// for this session.
		for chanID, rangeIndex := range m {
			// Get db chanID.
			chanDetailsBkt := tx.ReadWriteBucket(cChanDetailsBkt)
			if chanDetailsBkt == nil {
				return ErrUninitializedDB
			}

			chanDetails := chanDetailsBkt.NestedReadWriteBucket(
				chanID[:],
			)
			if chanDetails == nil {
				return ErrCorruptChanDetails
			}

			// Create a sub-bucket for this channel using the
			// db-assigned ID for the channel.
			dbChanID := chanDetails.Get(cChanDBID)
			chanAcksBkt, err := ackRangeBkt.CreateBucket(dbChanID)
			if err != nil {
				return err
			}

			// Iterate over the range pairs that we need to add to
			// the DB.
			for k, v := range rangeIndex.GetAllRanges() {
				var (
					start [8]byte
					end   [8]byte
				)
				byteOrder.PutUint64(start[:], k)
				byteOrder.PutUint64(end[:], v)

				err = chanAcksBkt.Put(start[:], end[:])
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}
