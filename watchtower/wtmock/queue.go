package wtmock

import (
	"container/list"
	"fmt"
	"sync"

	"github.com/lightningnetwork/lnd/watchtower/wtdb"
)

// DiskQueueDB is an in-memory implementation of the wtclient.Queue interface.
type DiskQueueDB[T any] struct {
	disk *list.List
	mu   sync.RWMutex
}

// NewQueueDB constructs a new DiskQueueDB.
func NewQueueDB[T any]() wtdb.Queue[T] {
	return &DiskQueueDB[T]{
		disk: list.New(),
	}
}

// Len returns the number of tasks in the queue.
//
// NOTE: This is part of the wtclient.Queue interface.
func (d *DiskQueueDB[T]) Len() (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return uint64(d.disk.Len()), nil
}

// Push adds new T items to the tail of the queue.
//
// NOTE: This is part of the wtclient.Queue interface.
func (d *DiskQueueDB[T]) Push(items ...T) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, item := range items {
		d.disk.PushBack(item)
	}

	return nil
}

// Pop pops the next T from the head of the queue. If no more items
// are in the queue then wtdb.ErrEmptyQueue is returned.
//
// NOTE: This is part of the wtclient.Queue interface.
func (d *DiskQueueDB[T]) Pop() (T, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var task T

	if d.disk.Len() == 0 {
		return task, wtdb.ErrEmptyQueue
	}

	e := d.disk.Front()
	task, ok := d.disk.Remove(e).(T)
	if !ok {
		return task, fmt.Errorf("queue item not of type %T", task)
	}

	return task, nil
}

// PushHead pushes new T items to the head of the queue in reverse order.
//
// NOTE: This is part of the wtclient.Queue interface.
func (d *DiskQueueDB[T]) PushHead(items ...T) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := len(items) - 1; i >= 0; i-- {
		d.disk.PushFront(items[i])
	}

	return nil
}
