package wtdb

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/lightningnetwork/lnd/kvdb"
)

var (
	// queueMainBkt will hold the main queue contents. It will have the
	// following structure:
	// 			=> oldestIndexKey => oldest index
	//                      => nextIndexKey => newest index
	//		        => itemsBkt => <index> -> item
	//
	// Any items added to the queue via Push, will be added to this queue.
	// Items will only be popped from this queue if the head queue is empty.
	queueMainBkt = []byte("queue-main")

	// queueHeadBkt will hold the items that have been pushed to the head
	// of the queue. It will have the following structure:
	// 			=> oldestIndexKey => oldest index
	//                      => nextIndexKey => newest index
	//		        => itemsBkt => <index> -> item
	//
	// Any items added to the queue via PushHead will be added to this queue
	// in reverse order. If this queue is not empty, then Pop will pop items
	// from this queue.
	queueHeadBkt = []byte("queue-head")

	// itemsBkt is a sub-bucket of both the main and head queue storing:
	// 		index -> encoded item
	itemsBkt = []byte("items")

	// oldestIndexKey is a key of both the main and head queue storing the
	// index of the item at the head of the queue.
	oldestIndexKey = []byte("oldest-index")

	// nextIndexKey is a key of both the main and head queue storing the
	// index of the item at the tail of the queue.
	nextIndexKey = []byte("next-index")

	// ErrEmptyQueue is returned from Pop if there are no items left in
	// the queue.
	ErrEmptyQueue = errors.New("queue is empty")
)

// Queue is an interface describing a FIFO queue for any generic type T.
type Queue[T any] interface {
	// Len returns the number of tasks in the queue.
	Len() (uint64, error)

	// Push pushes new T items to the tail of the queue.
	Push(items ...T) error

	// Pop pops the next T from the head of the queue. If no more items are
	// in the queue then ErrEmptyQueue is returned.
	Pop() (T, error)

	// PushHead pushes new T items to the head of the queue.
	PushHead(items ...T) error
}

// Serializable is an interface must be satisfied for any type that the
// DiskQueueDB should handle.
type Serializable interface {
	Encode(w io.Writer) error
	Decode(r io.Reader) error
}

// DiskQueueDB is a generic Bolt DB implementation of the Queue interface.
type DiskQueueDB[T Serializable] struct {
	db          kvdb.Backend
	topLevelBkt []byte
	constructor func() T
}

// A compile-time check to ensure that DiskQueueDB implements the Queue
// interface.
var _ Queue[Serializable] = (*DiskQueueDB[Serializable])(nil)

// NewQueueDB constructs a new DiskQueueDB. A queueBktName must be provided so
// that the DiskQueueDB can create its own namespace in the bolt db.
func NewQueueDB[T Serializable](db kvdb.Backend, queueBktName []byte,
	constructor func() T) Queue[T] {

	return &DiskQueueDB[T]{
		db:          db,
		topLevelBkt: queueBktName,
		constructor: constructor,
	}
}

// Len returns the number of tasks in the queue.
//
// NOTE: This is part of the Queue interface.
func (d *DiskQueueDB[T]) Len() (uint64, error) {
	var res uint64
	err := kvdb.View(d.db, func(tx kvdb.RTx) error {
		numMain, err := d.numTasks(tx, queueMainBkt)
		if err != nil {
			return err
		}

		numHead, err := d.numTasks(tx, queueHeadBkt)
		if err != nil {
			return err
		}

		res = numMain + numHead

		return nil
	}, func() {
		res = 0
	})
	if err != nil {
		return 0, err
	}

	return res, nil
}

// Push adds a T to the tail of the queue.
//
// NOTE: This is part of the Queue interface.
func (d *DiskQueueDB[T]) Push(items ...T) error {
	return d.db.Update(func(tx walletdb.ReadWriteTx) error {
		for _, item := range items {
			err := d.addTask(tx, queueMainBkt, item)
			if err != nil {
				return err
			}
		}

		return nil
	}, func() {})
}

// Pop pops the next T from the head of the queue. If no more items
// are in the queue then nil is returned.
//
// NOTE: This is part of the Queue interface.
func (d *DiskQueueDB[T]) Pop() (T, error) {
	item := d.constructor()
	err := d.db.Update(func(tx walletdb.ReadWriteTx) error {
		var err error
		item, err = d.nextTask(tx, queueHeadBkt)
		// No error means that an item was found.
		if err == nil {
			return nil
		} else if !errors.Is(err, ErrEmptyQueue) {
			return err
		}

		item, err = d.nextTask(tx, queueMainBkt)

		return err
	}, func() {
		item = d.constructor()
	})
	if err != nil {
		return item, err
	}

	return item, nil
}

// PushHead pushes new T items to the head of the queue.
//
// NOTE: This is part of the Queue interface.
func (d *DiskQueueDB[T]) PushHead(items ...T) error {
	return d.db.Update(func(tx walletdb.ReadWriteTx) error {
		for i := 0; i < len(items); i++ {
			err := d.addTask(tx, queueHeadBkt, items[i])
			if err != nil {
				return err
			}
		}

		return nil
	}, func() {})
}

// addTask adds the given item to the back of the given queue.
func (d *DiskQueueDB[T]) addTask(tx kvdb.RwTx, queueName []byte, item T) error {
	var (
		namespacedBkt = tx.ReadWriteBucket(d.topLevelBkt)
		err           error
	)
	if namespacedBkt == nil {
		namespacedBkt, err = tx.CreateTopLevelBucket(d.topLevelBkt)
		if err != nil {
			return err
		}
	}

	mainTasksBucket, err := namespacedBkt.CreateBucketIfNotExists(
		cTaskQueue,
	)
	if err != nil {
		return err
	}

	bucket, err := mainTasksBucket.CreateBucketIfNotExists(queueName)
	if err != nil {
		return err
	}

	// Find the index to use for placing this new item at the back of the
	// queue.
	var nextIndex uint64
	nextIndexB := bucket.Get(nextIndexKey)
	if nextIndexB != nil {
		nextIndex, err = readBigSize(nextIndexB)
		if err != nil {
			return err
		}
	} else {
		nextIndexB, err = writeBigSize(0)
		if err != nil {
			return err
		}
	}

	tasksBucket, err := bucket.CreateBucketIfNotExists(itemsBkt)
	if err != nil {
		return err
	}

	var buff bytes.Buffer
	err = item.Encode(&buff)
	if err != nil {
		return err
	}

	// Put the new task in the assigned index.
	err = tasksBucket.Put(nextIndexB, buff.Bytes())
	if err != nil {
		return err
	}

	// Increment the next-index counter.
	nextIndex++
	nextIndexB, err = writeBigSize(nextIndex)
	if err != nil {
		return err
	}

	return bucket.Put(nextIndexKey, nextIndexB)
}

// nextTask pops an item of the queue identified by the given namespace. If
// there are no items on the queue then ErrEmptyQueue is returned.
func (d *DiskQueueDB[T]) nextTask(tx kvdb.RwTx, queueName []byte) (T, error) {
	task := d.constructor()

	namespacedBkt := tx.ReadWriteBucket(d.topLevelBkt)
	if namespacedBkt == nil {
		return task, ErrEmptyQueue
	}

	mainTasksBucket := namespacedBkt.NestedReadWriteBucket(cTaskQueue)
	if mainTasksBucket == nil {
		return task, ErrEmptyQueue
	}

	bucket, err := mainTasksBucket.CreateBucketIfNotExists(queueName)
	if err != nil {
		return task, err
	}

	// Get the index of the tail of the queue.
	var nextIndex uint64
	nextIndexB := bucket.Get(nextIndexKey)
	if nextIndexB != nil {
		nextIndex, err = readBigSize(nextIndexB)
		if err != nil {
			return task, err
		}
	}

	// Get the index of the head of the queue.
	var oldestIndex uint64
	oldestIndexB := bucket.Get(oldestIndexKey)
	if oldestIndexB != nil {
		oldestIndex, err = readBigSize(oldestIndexB)
		if err != nil {
			return task, err
		}
	} else {
		oldestIndexB, err = writeBigSize(0)
		if err != nil {
			return task, err
		}
	}

	// If the head and tail are equal, then there are no items in
	// the queue.
	if oldestIndex == nextIndex {
		// Take this opportunity to reset both indexes to zero.
		zeroIndexB, err := writeBigSize(0)
		if err != nil {
			return task, err
		}

		err = bucket.Put(oldestIndexKey, zeroIndexB)
		if err != nil {
			return task, err
		}

		err = bucket.Put(nextIndexKey, zeroIndexB)
		if err != nil {
			return task, err
		}

		return task, ErrEmptyQueue
	}

	// Otherwise, pop the item at the oldest index.
	tasksBucket := bucket.NestedReadWriteBucket(itemsBkt)
	if tasksBucket == nil {
		return task, fmt.Errorf("client-tasks bucket not found")
	}

	item := tasksBucket.Get(oldestIndexB)
	if item == nil {
		return task, fmt.Errorf("no task found under index")
	}

	err = tasksBucket.Delete(oldestIndexB)
	if err != nil {
		return task, err
	}

	// Increment the oldestIndex value so that it now points to the new
	// oldest item.
	oldestIndex++
	oldestIndexB, err = writeBigSize(oldestIndex)
	if err != nil {
		return task, err
	}

	err = bucket.Put(oldestIndexKey, oldestIndexB)
	if err != nil {
		return task, err
	}

	if err = task.Decode(bytes.NewBuffer(item)); err != nil {
		return task, err
	}

	return task, nil
}

// numTasks returns the number of items in the given queue.
func (d *DiskQueueDB[T]) numTasks(tx kvdb.RTx, queueName []byte) (uint64,
	error) {

	namespacedBkt := tx.ReadBucket(d.topLevelBkt)
	if namespacedBkt == nil {
		return 0, nil
	}

	mainTasksBucket := namespacedBkt.NestedReadBucket(cTaskQueue)
	if mainTasksBucket == nil {
		return 0, nil
	}

	bucket := mainTasksBucket.NestedReadBucket(queueName)
	if bucket == nil {
		return 0, nil
	}

	var (
		nextIndex uint64
		err       error
	)

	nextIndexB := bucket.Get(nextIndexKey)
	if nextIndexB != nil {
		nextIndex, err = readBigSize(nextIndexB)
		if err != nil {
			return 0, err
		}
	}

	var oldestIndex uint64
	oldestIndexB := bucket.Get(oldestIndexKey)
	if oldestIndexB != nil {
		oldestIndex, err = readBigSize(oldestIndexB)
		if err != nil {
			return 0, err
		}
	}

	return nextIndex - oldestIndex, nil
}
