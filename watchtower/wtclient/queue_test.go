package wtclient

import (
	"testing"
	"time"

	"github.com/btcsuite/btclog"
	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/lightningnetwork/lnd/watchtower/wtmock"
	"github.com/stretchr/testify/require"
)

// TestDiskOverflowQueue tests that the DiskOverflowQueue behaves as expected.
func TestDiskOverflowQueue(t *testing.T) {
	t.Parallel()

	const (
		maxInMemItems = 5
		waitTime      = time.Second * 2
	)

	getNext := func(q *DiskOverflowQueue[*wtdb.BackupID],
		i int) *wtdb.BackupID {

		var item *wtdb.BackupID
		select {
		case item = <-q.NextBackupID():
		case <-time.After(waitTime):
			t.Fatalf("task %d not received in time", i)
		}

		return item
	}

	enqueue := func(q *DiskOverflowQueue[*wtdb.BackupID],
		task *wtdb.BackupID) {

		err := q.QueueBackupID(task)
		require.NoError(t, err)
	}

	waitForNumDisk := func(db wtdb.Queue[*wtdb.BackupID], num int) {
		err := wait.Predicate(func() bool {
			n, err := db.Len()
			require.NoError(t, err)

			return n == uint64(num)
		}, waitTime)
		require.NoError(t, err)
	}

	assertNumDisk := func(db wtdb.Queue[*wtdb.BackupID], num int) {
		n, err := db.Len()
		require.NoError(t, err)
		require.EqualValues(t, num, n)
	}

	t.Run("overflow to disk", func(t *testing.T) {
		t.Parallel()

		// Generate some backup IDs that we want to add to the queue.
		tasks := genBackupIDs(10)

		// Create a mock db.
		db := wtmock.NewQueueDB[*wtdb.BackupID]()

		// New mock logger.
		log := newMockLogger(t.Logf)

		// Init the queue with the mock DB.
		q, err := NewDiskOverflowQueue[*wtdb.BackupID](
			db, maxInMemItems, log,
		)
		require.NoError(t, err)

		// Start the queue.
		require.NoError(t, q.Start())

		// Initially there should be no items on disk.
		assertNumDisk(db, 0)

		// Start filling up the queue.
		enqueue(q, tasks[0])
		enqueue(q, tasks[1])
		enqueue(q, tasks[2])
		enqueue(q, tasks[3])
		enqueue(q, tasks[4])

		// The queue should now be full, so any new items should be
		// persisted to disk.
		enqueue(q, tasks[5])
		waitForNumDisk(db, 1)

		// Now pop all items from the queue to ensure that the item
		// from disk is loaded in properly once there is space.
		require.Equal(t, tasks[0], getNext(q, 0))
		require.Equal(t, tasks[1], getNext(q, 1))
		require.Equal(t, tasks[2], getNext(q, 2))
		require.Equal(t, tasks[3], getNext(q, 3))
		require.Equal(t, tasks[4], getNext(q, 4))
		require.Equal(t, tasks[5], getNext(q, 5))

		// There should no longer be any items in the disk queue.
		assertNumDisk(db, 0)

		require.NoError(t, q.Stop())
	})

	t.Run("load prev-mem items on startup", func(t *testing.T) {
		t.Parallel()

		// Generate some backup IDs that we want to add to the queue.
		tasks := genBackupIDs(4)

		// Create a mock db.
		db := wtmock.NewQueueDB[*wtdb.BackupID]()

		// New mock logger.
		log := newMockLogger(t.Logf)

		// Pre-populate the prev-mem disk queue with a few tasks.
		// These are expected to be preloaded into the queue on startup.
		require.NoError(t, db.Push(tasks[0]))
		require.NoError(t, db.Push(tasks[1]))
		require.NoError(t, db.Push(tasks[2]))

		// Assert that these items are actually in the disk queue.
		assertNumDisk(db, 3)

		// Init the queue with the mock DB.
		q, err := NewDiskOverflowQueue[*wtdb.BackupID](
			db, maxInMemItems, log,
		)
		require.NoError(t, err)

		// Start the queue.
		require.NoError(t, q.Start())

		// All the disk items should now have been loaded into memory.
		waitForNumDisk(db, 0)

		// Add one new item to the queue just to ensure that it is
		// only popped after the items loaded from prev-mem.
		enqueue(q, tasks[3])

		// Now pop the items and ensure that they are returned in the
		// correct order.
		require.Equal(t, tasks[0], getNext(q, 0))
		require.Equal(t, tasks[1], getNext(q, 1))
		require.Equal(t, tasks[2], getNext(q, 2))

		require.NoError(t, q.Stop())
	})

	t.Run("startup with smaller buffer size", func(t *testing.T) {
		t.Parallel()

		const (
			firstMaxInMemItems  = 5
			secondMaxInMemItems = 2
		)

		// Generate some backup IDs that we want to add to the queue.
		tasks := genBackupIDs(10)

		// Create a mock db.
		db := wtmock.NewQueueDB[*wtdb.BackupID]()

		// New mock logger.
		log := newMockLogger(t.Logf)

		// Init the queue with the mock DB and an initial max in-mem
		// items number.
		q, err := NewDiskOverflowQueue[*wtdb.BackupID](
			db, firstMaxInMemItems, log,
		)
		require.NoError(t, err)
		require.NoError(t, q.Start())

		// Add 7 items to the queue. The first 5 will go into the in-mem
		// queue, the other 2 will be persisted to the main disk queue.
		enqueue(q, tasks[0])
		enqueue(q, tasks[1])
		enqueue(q, tasks[2])
		enqueue(q, tasks[3])
		enqueue(q, tasks[4])
		enqueue(q, tasks[5])
		enqueue(q, tasks[6])

		waitForNumDisk(db, 2)

		// Now stop the queue and re-initialise it with a smaller
		// buffer maximum.
		require.NoError(t, q.Stop())

		// Check that there are now 7 items in the disk queue.
		waitForNumDisk(db, 7)

		// Re-init the queue with a smaller max buffer size.
		q, err = NewDiskOverflowQueue[*wtdb.BackupID](
			db, secondMaxInMemItems, log,
		)
		require.NoError(t, err)
		require.NoError(t, q.Start())

		// Now there should be a few items left in the disk queue since
		// the in-memory buffer is now smaller.
		waitForNumDisk(db, 5)

		// Make sure that items are popped off the queue in the correct
		// order.
		require.Equal(t, tasks[0], getNext(q, 0))
		require.Equal(t, tasks[1], getNext(q, 1))
		require.Equal(t, tasks[2], getNext(q, 2))
		require.Equal(t, tasks[3], getNext(q, 3))
		require.Equal(t, tasks[4], getNext(q, 4))
		require.Equal(t, tasks[5], getNext(q, 5))
		require.Equal(t, tasks[6], getNext(q, 6))

		require.NoError(t, q.Stop())
	})
}

func genBackupIDs(num int) []*wtdb.BackupID {
	ids := make([]*wtdb.BackupID, num)
	for i := 0; i < num; i++ {
		ids[i] = newBackupID(i)
	}

	return ids
}

func newBackupID(id int) *wtdb.BackupID {
	return &wtdb.BackupID{CommitHeight: uint64(id)}
}

// BenchmarkDiskOverflowQueue benchmarks the performance of adding and removing
// items from the DiskOverflowQueue using an in-memory disk db.
func BenchmarkDiskOverflowQueue(b *testing.B) {
	enqueue := func(q *DiskOverflowQueue[*wtdb.BackupID],
		task *wtdb.BackupID) {

		err := q.QueueBackupID(task)
		require.NoError(b, err)
	}

	getNext := func(q *DiskOverflowQueue[*wtdb.BackupID],
		i int) *wtdb.BackupID {

		var item *wtdb.BackupID
		select {
		case item = <-q.NextBackupID():
		case <-time.After(time.Second * 2):
			b.Fatalf("task %d not received in time", i)
		}

		return item
	}

	// Generate some backup IDs that we want to add to the queue.
	tasks := genBackupIDs(b.N)

	// Create a mock db.
	db := wtmock.NewQueueDB[*wtdb.BackupID]()

	// New mock logger.
	log := newMockLogger(b.Logf)

	// Init the queue with the mock DB.
	q, err := NewDiskOverflowQueue[*wtdb.BackupID](db, 5, log)
	require.NoError(b, err)

	// Start the queue.
	require.NoError(b, q.Start())

	// Start filling up the queue.
	for n := 0; n < b.N; n++ {
		enqueue(q, tasks[n])
	}

	// Pop all the items off the queue.
	for n := 0; n < b.N; n++ {
		require.Equal(b, tasks[n], getNext(q, n))
	}

	require.NoError(b, q.Stop())
}

type mockLogger struct {
	log func(string, ...any)

	btclog.Logger
}

func newMockLogger(logger func(string, ...any)) *mockLogger {
	return &mockLogger{log: logger}
}

// Errorf formats message according to format specifier and writes to log.
//
// NOTE: this is part of the btclog.Logger interface.
func (l *mockLogger) Errorf(format string, params ...any) {
	l.log("[ERR]: "+format, params...)
}
