package wtdb_test

import (
	"testing"

	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
	"github.com/stretchr/testify/require"
)

// TestDiskQueue ensures that the ClientDBs disk queue methods behave as is
// expected of a queue.
func TestDiskQueue(t *testing.T) {
	t.Parallel()

	// Set up a temporary bolt backend.
	dbCfg := &kvdb.BoltConfig{DBTimeout: kvdb.DefaultDBTimeout}
	bdb, err := wtdb.NewBoltBackendCreator(
		true, t.TempDir(), "wtclient.db",
	)(dbCfg)
	require.NoError(t, err)

	// Construct the ClientDB.
	db, err := wtdb.OpenClientDB(bdb)
	require.NoError(t, err)
	t.Cleanup(func() {
		err = db.Close()
		require.NoError(t, err)
	})

	namespace := []byte("test-namespace")
	queue := db.GetDBQueue(namespace)

	addTasksToTail := func(tasks ...*wtdb.BackupID) {
		err := queue.Push(tasks...)
		require.NoError(t, err)
	}

	addTasksToHead := func(tasks ...*wtdb.BackupID) {
		err := queue.PushHead(tasks...)
		require.NoError(t, err)
	}

	assertNumTasks := func(expNum int) {
		num, err := queue.Len()
		require.NoError(t, err)
		require.EqualValues(t, expNum, num)
	}

	popAndAssert := func(expTask *wtdb.BackupID) {
		task, err := queue.Pop()
		require.NoError(t, err)
		require.EqualValues(t, expTask, task)
	}

	// Create a few tasks that we use throughout the test.
	task1 := &wtdb.BackupID{CommitHeight: 1}
	task2 := &wtdb.BackupID{CommitHeight: 2}
	task3 := &wtdb.BackupID{CommitHeight: 3}
	task4 := &wtdb.BackupID{CommitHeight: 4}
	task5 := &wtdb.BackupID{CommitHeight: 5}
	task6 := &wtdb.BackupID{CommitHeight: 6}

	// Namespace 1 should initially have no items.
	assertNumTasks(0)

	// Now add a few items to the tail of the queue.
	addTasksToTail(task1, task2)

	// Check that the number of tasks is now two.
	assertNumTasks(2)

	// Pop a task, check that it is task 1 and assert that the number of
	// items left is now 1.
	popAndAssert(task1)
	assertNumTasks(1)

	// Pop a task, check that it is task 2 and assert that the number of
	// items left is now 0.
	popAndAssert(task2)
	assertNumTasks(0)

	// Once again add a few tasks.
	addTasksToTail(task3, task4)

	// Now push some tasks to the head of the queue.
	addTasksToHead(task6, task5)

	// Ensure that both the disk queue lengths are added together when
	// querying the length of the queue.
	assertNumTasks(4)

	// Ensure that the order that the tasks are popped is correct.
	popAndAssert(task6)
	popAndAssert(task5)
	popAndAssert(task3)
	popAndAssert(task4)
}
