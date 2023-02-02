package wtclient

import (
	"testing"
	"time"

	"github.com/lightningnetwork/lnd/lntest/wait"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/lightningnetwork/lnd/watchtower/wtmock"
	"github.com/stretchr/testify/require"
)

func TestNewOverflowQueue(t *testing.T) {
	task1 := &wtdb.BackupID{CommitHeight: 1}
	task2 := &wtdb.BackupID{CommitHeight: 2}
	task3 := &wtdb.BackupID{CommitHeight: 3}
	task4 := &wtdb.BackupID{CommitHeight: 4}
	task5 := &wtdb.BackupID{CommitHeight: 5}
	task6 := &wtdb.BackupID{CommitHeight: 6}
	task7 := &wtdb.BackupID{CommitHeight: 7}
	task8 := &wtdb.BackupID{CommitHeight: 8}
	task9 := &wtdb.BackupID{CommitHeight: 9}
	task10 := &wtdb.BackupID{CommitHeight: 10}

	db := wtmock.NewQueueDB()

	db.AddTask(prevMemNamespace, task1)
	db.AddTask(prevMemNamespace, task2)
	db.AddTask(prevMemNamespace, task3)

	q := NewDiskOverflowQueue(db, 5)
	err := q.Start()
	require.NoError(t, err)
	t.Cleanup(func() { _ = q.Stop })

	outChan := q.NextBackupID()
	getNextItem := func(i int) *wtdb.BackupID {
		var item *wtdb.BackupID
		select {
		case item = <-outChan:
		case <-time.After(time.Second * 2):
			t.Fatalf("task %d not received in time", i)
		}
		return item
	}

	require.NoError(t, q.QueueBackupID(task4))
	require.NoError(t, q.QueueBackupID(task5))

	num, err := db.NumTasks(diskNamespace)
	require.NoError(t, err)
	require.EqualValues(t, 0, num)

	require.NoError(t, q.QueueBackupID(task6))
	require.NoError(t, q.QueueBackupID(task7))
	require.NoError(t, q.QueueBackupID(task8))
	require.NoError(t, q.QueueBackupID(task9))
	require.NoError(t, q.QueueBackupID(task10))

	err = wait.Predicate(func() bool {
		num, err := db.NumTasks(diskNamespace)
		require.NoError(t, err)

		return num == 4
	}, time.Second)
	require.NoError(t, err)

	require.Equal(t, task1, getNextItem(1))
	require.Equal(t, task2, getNextItem(2))
	require.Equal(t, task3, getNextItem(3))
	require.Equal(t, task4, getNextItem(4))
	require.Equal(t, task5, getNextItem(5))
	require.Equal(t, task6, getNextItem(6))
	require.Equal(t, task7, getNextItem(7))

	require.NoError(t, q.QueueBackupID(task1))
	require.NoError(t, q.QueueBackupID(task2))
	require.NoError(t, q.QueueBackupID(task3))
	require.NoError(t, q.QueueBackupID(task4))

	err = wait.Predicate(func() bool {
		num, err := db.NumTasks(diskNamespace)
		require.NoError(t, err)

		return num == 1
	}, time.Second)
	require.NoError(t, err)

	// Lets also now shut down the queue.
	require.NoError(t, q.Stop())

	numMem, err := db.NumTasks(prevMemNamespace)
	require.NoError(t, err)
	require.EqualValues(t, 6, numMem)

	numDisk, err := db.NumTasks(diskNamespace)
	require.NoError(t, err)
	require.EqualValues(t, 1, numDisk)

	// Now restart the queue with the same db and smaller buffer size.
	q = NewDiskOverflowQueue(db, 3)

	q.Start()
	t.Cleanup(func() { _ = q.Stop })
	outChan = q.NextBackupID()

	require.Equal(t, task8, getNextItem(8))
	require.Equal(t, task9, getNextItem(9))
	require.Equal(t, task10, getNextItem(10))
	require.Equal(t, task1, getNextItem(1))
	require.Equal(t, task2, getNextItem(2))
	require.Equal(t, task3, getNextItem(3))
	require.Equal(t, task4, getNextItem(4))
}
