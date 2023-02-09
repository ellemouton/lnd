package wtclient

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"github.com/btcsuite/btclog"
	"github.com/lightningnetwork/lnd/watchtower/wtdb"
)

// mode represents the mode that the DiskOverflowQueue is in.
type mode uint8

const (
	// modeMem means that the in-memory queue has not yet reached its
	// maximum capacity and so any new items can just be put straight into
	// the in-memory channel. If the in-memory queue becomes saturated, then
	// the mode should be changed to modeDisk.
	modeMem mode = iota

	// modeDisk means that the queue should be fed new items from the main
	// disk queue and any new items should be persisted to the main disk
	// queue. Only once all items have been read from disk _and_ there is
	// space in the in-memory channel should the mode be switched to
	// modeMem.
	modeDisk

	// dbErrorBackoff is the length of time we will back off before retrying
	// any DB action that failed.
	dbErrorBackoff = time.Second * 5
)

// DiskOverflowQueue is a queue that must be initialised with a certain maximum
// buffer size which represents the maximum number of elements that the queue
// should hold in memory. If the queue is full, then any new elements added to
// the queue will be persisted to disk instead. Once a consumer starts reading
// from the front of the queue again then items on disk will be moved into the
// queue again. The queue is also re-start safe. When it is stopped, any items
// in the memory queue, will be persisted to disk. On start up, the queue will
// be re-initialised with the items on disk.
type DiskOverflowQueue[T any] struct {
	startOnce sync.Once
	stopOnce  sync.Once

	log btclog.Logger

	// db is the database that will be used to persist queue items to disk.
	db wtdb.Queue[T]

	// mode represents the current mode of operation of the queue.
	mode   mode
	modeMu sync.Mutex

	// We used an unbound list for the input of the queue so that producers
	// putting items into the queue are never blocked.
	inputListMu   sync.Mutex
	inputListCond *sync.Cond
	inputList     *list.List

	// inputChan is an unbuffered channel used to pass items from
	// manageQueueTail to manageMemQueueInput.
	inputChan chan *internalTask[T]

	// memQueue is a buffered channel used to pass items from
	// manageMemQueueInput to manageQueueHead.
	memQueue chan T

	// outputChan is an unbuffered channel from which items at the head of
	// the queue can be read.
	outputChan chan T

	// newDiskItemSignal is used to signal that there is a new item in the
	// main disk queue. There should only be one reader and one writer for
	// this channel.
	newDiskItemSignal chan struct{}

	// leftOverItem1 will be a non-nil task on shutdown if the
	// manageQueueHead method was holding an unhandled tasks at shutdown
	// time. Since manageQueueHead handles the very head of the queue, this
	// item should be the first to be reloaded on restart.
	leftOverItem1 *T

	// leftOverItem2 will be non-nil on shutdown if the manageMemQueueInput
	// method was holding an unhandled task at shutdown time. Since
	// manageMemQueueInput manages the input to the queue, this task should
	// be pushed to the head of the disk queue.
	leftOverItem2 *T

	// leftOverItem3 will be non-nil on shutdown if manageQueueTail was
	// holding an unhandled task at shutdown time. This task should be put
	// at the tail of the disk queue but should come before any input list
	// task.
	leftOverItem3 *T

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewDiskOverflowQueue constructs a new DiskOverflowQueue.
func NewDiskOverflowQueue[T any](db wtdb.Queue[T], maxQueueSize uint64,
	logger btclog.Logger) (*DiskOverflowQueue[T], error) {

	if maxQueueSize < 2 {
		return nil, errors.New("the in-memory queue buffer size " +
			"must be larger than 2")
	}

	q := &DiskOverflowQueue[T]{
		log:               logger,
		mode:              modeMem,
		db:                db,
		inputList:         list.New(),
		newDiskItemSignal: make(chan struct{}, 1),
		inputChan:         make(chan *internalTask[T]),
		memQueue:          make(chan T, maxQueueSize-2),
		outputChan:        make(chan T),
		quit:              make(chan struct{}),
	}
	q.inputListCond = sync.NewCond(&q.inputListMu)

	return q, nil
}

// Start kicks off all the goroutines that are required to manage the queue.
func (q *DiskOverflowQueue[T]) Start() error {
	var err error
	q.startOnce.Do(func() {
		err = q.start()
	})

	return err
}

// start kicks off all the goroutines that are required to manage the queue.
func (q *DiskOverflowQueue[T]) start() error {
	numDisk, err := q.db.Len()
	if err != nil {
		return err
	}
	if numDisk != 0 {
		q.setMode(modeDisk)
	}

	// Kick off the three goroutines which will handle the input list, the
	// in-memory queue and the output channel.
	q.wg.Add(3)
	go q.manageQueueTail()
	go q.manageMemQueueInput()
	go q.manageQueueHead()

	return nil
}

// Stop stops the queue and persists any items in the memory queue to disk.
func (q *DiskOverflowQueue[T]) Stop() error {
	var err error
	q.stopOnce.Do(func() {
		err = q.stop()
	})

	return err
}

// stop the queue and persists any items in the memory queue to disk.
func (q *DiskOverflowQueue[T]) stop() error {
	close(q.quit)

	// Signal on the inputListCond until all the goroutines have returned.
	shutdown := make(chan struct{})
	go func() {
		for {
			select {
			case <-time.After(time.Millisecond):
				q.inputListCond.Signal()
			case <-shutdown:
				return
			}
		}
	}()

	q.wg.Wait()
	close(shutdown)

	// queueHead will be the items that we will be pushed to the head of
	// the queue.
	var queueHead []T

	// First, we append leftOverItem1 since this task is the current head
	// of the queue.
	if q.leftOverItem1 != nil {
		queueHead = append(queueHead, *q.leftOverItem1)
	}

	// Next, drain the buffered queue.
	for {
		task, ok := <-q.memQueue
		if !ok {
			break
		}

		queueHead = append(queueHead, task)
	}

	// Then, any item held in leftOverItem2 would have been next to join the
	// memQueue. So that gets added next.
	if q.leftOverItem2 != nil {
		queueHead = append(queueHead, *q.leftOverItem2)
	}

	// Now, push these items to the head of the queue.
	err := q.db.PushHead(queueHead...)
	if err != nil {
		q.log.Errorf("could not add tasks to queue head: %v", err)
	}

	// Next we handle any items that need to be added to the main disk
	// queue.
	var diskQueue []T

	// Any item in leftOverItem3 is the first item that should join the
	// disk queue.
	if q.leftOverItem3 != nil {
		diskQueue = append(diskQueue, *q.leftOverItem3)
	}

	// Lastly, drain any items in the unbuffered input list.
	for q.inputList.Front() != nil {
		e := q.inputList.Front()

		//nolint:forcetypeassert
		task := q.inputList.Remove(e).(T)

		diskQueue = append(diskQueue, task)
	}

	// Now persist these items to the main disk queue.
	err = q.db.Push(diskQueue...)
	if err != nil {
		q.log.Errorf("could not add tasks to queue tail: %v", err)
	}

	return nil
}

// setMode sets the mode of the queue.
func (q *DiskOverflowQueue[T]) setMode(m mode) {
	q.modeMu.Lock()
	defer q.modeMu.Unlock()

	q.mode = m
}

// getMode returns the current mode of the queue.
func (q *DiskOverflowQueue[T]) getMode() mode {
	q.modeMu.Lock()
	defer q.modeMu.Unlock()

	return q.mode
}

// QueueBackupID adds a wtdb.BackupID to the queue. It will only return an error
// if the queue has been stopped. It is non-blocking.
func (q *DiskOverflowQueue[T]) QueueBackupID(item *wtdb.BackupID) error {
	// Return an error if the queue has been stopped
	select {
	case <-q.quit:
		return ErrClientExiting
	default:
	}

	// Add the new item to the unbound input list.
	q.inputListCond.L.Lock()
	q.inputList.PushBack(item)
	q.inputListCond.L.Unlock()

	// Signal that there is a new item in the input list.
	q.inputListCond.Signal()

	return nil
}

// NextBackupID can be used to read from the head of the DiskOverflowQueue.
func (q *DiskOverflowQueue[T]) NextBackupID() <-chan T {
	return q.outputChan
}

// internalTask wraps a BackupID task with a success channel.
type internalTask[T any] struct {
	task    T
	success chan bool
}

// manageQueueTail handles the input to the DiskOverflowQueue. It takes from the
// un-bounded input list and then, depending on what mode the queue is in,
// either puts the new item straight onto the persisted disk queue or attempts
// to feed it into the memQueue. On exit, any unhandled task will be assigned to
// leftOverItem3.
func (q *DiskOverflowQueue[T]) manageQueueTail() {
	defer q.wg.Done()

	for {
		// Wait for the input list to not be empty.
		q.inputListCond.L.Lock()
		for q.inputList.Front() == nil {
			q.inputListCond.Wait()

			select {
			case <-q.quit:
				return
			default:
			}
		}

		// Pop the first element from the queue.
		e := q.inputList.Front()

		//nolint:forcetypeassert
		task := q.inputList.Remove(e).(T)
		q.inputListCond.L.Unlock()

		// What we do with this new item depends on what the mode of the
		// queue currently is.
	checkMode:
		switch q.getMode() {
		// If the queue is in modeDisk then any new items should be put
		// straight into the disk queue.
		case modeDisk:
			err := q.db.Push(task)
			if err != nil {
				// Log and back off for a few seconds and then
				// try again with the same task.
				q.log.Errorf("could not persist %s to disk. "+
					"Retrying after backoff", task)

				select {
				// Backoff for a bit and then re-check the mode
				// and try again to handle the task.
				case <-time.After(dbErrorBackoff):
					goto checkMode

				// If the queue is quit at this moment, then the
				// unhandled task is assigned to leftOverItem3
				// so that it can be handled by the stop method.
				case <-q.quit:
					q.leftOverItem3 = &task
					return
				}
			}

			// Send a signal that there is a new item in the main
			// disk queue.
			select {
			case q.newDiskItemSignal <- struct{}{}:
			case <-q.quit:
				return
			default:
			}

		// If the mode is modeMem, then try feed it to the
		// manageMemQueueInput handler via the un-buffered inputChan
		// channel. We wrap it in an internal task so that we can find
		// out if manageMemQueueInput successfully handled the item. If
		// it did, we continue in modeMem and if not, then we switch to
		// modeDisk so that we can persist the item to the disk queue
		// instead.
		case modeMem:
			success := make(chan bool)
			it := &internalTask[T]{
				task:    task,
				success: success,
			}

			select {
			// Try feed the task to the manageMemQueueInput handler.
			// The handler, if it does take the task, is guaranteed
			// to respond via the success channel of the task to
			// indicate if the task was successfully added to the
			// memQueue.
			case q.inputChan <- it:

			// If the queue is quit at this moment, then the
			// unhandled task is assigned to leftOverItem3 so that
			// it can be handled by the stop method.
			case <-q.quit:
				q.leftOverItem3 = &task

				return

			default:
				// Was not accepted. So maybe the mode changed.
				goto checkMode
			}

			// If we get here, it means that the manageMemQueueInput
			// handler took the task. It is guaranteed to respond
			// via the success channel, so we wait for that response
			// here.
			s := <-success
			if s {
				continue
			}

			// If the task was not successfully handled by
			// manageMemQueueInput, then we switch to modeDisk so
			// that the task can be persisted in the disk queue
			// instead.
			q.setMode(modeDisk)

			goto checkMode
		}
	}
}

// manageMemQueueInput manages which items should be fed onto the buffered
// memQueue. If the queue is then in modeDisk, then the handler will read new
// tasks from the disk queue until it is empty. After that, it will switch
// between reading from the input channel or the disk queue depending on the
// queue mode.
func (q *DiskOverflowQueue[T]) manageMemQueueInput() {
	defer func() {
		close(q.memQueue)
		q.wg.Done()
	}()

	// If the queue is in modeDisk, then the memQueue is fed with tasks
	// from the disk queue until it is empty.
	if q.getMode() == modeDisk {
		for {
			task, err := q.db.Pop()
			if errors.Is(err, wtdb.ErrEmptyQueue) {
				q.setMode(modeMem)
				break
			} else if err != nil {
				q.log.Errorf("could not load next task from " +
					"disk. Retrying.")

				select {
				case <-time.After(dbErrorBackoff):
					continue
				case <-q.quit:
					return
				}
			}

			select {
			case q.memQueue <- task:

			// If the queue is quit at this moment, then the
			// unhandled task is assigned to leftOverItem2 so that
			// it can be handled by the stop method.
			case <-q.quit:
				q.leftOverItem2 = &task
				return
			}
		}
	}

	// Now the queue enters its normal operation which can only ever switch
	// between modeMem and modeDisk.
	for {
		select {
		case <-q.quit:
			return

		// If there is a signal that a new item has been added to disk
		// then we use the disk queue as the source of the next task
		// to feed into memQueue.
		case <-q.newDiskItemSignal:
			for {
				task, err := q.db.Pop()
				// If the disk-queue is empty, then we switch
				// back to modeMem.
				if errors.Is(err, wtdb.ErrEmptyQueue) {
					q.setMode(modeMem)

					break
				} else if err != nil {
					q.log.Errorf("could not load next " +
						"task from disk. Retrying.")

					select {
					case <-time.After(dbErrorBackoff):
						continue
					case <-q.quit:
						return
					}
				}

				select {
				case q.memQueue <- task:

				// If the queue is quit at this moment, then the
				// unhandled task is assigned to leftOverItem2
				// so that it can be handled by the stop method.
				case <-q.quit:
					q.leftOverItem2 = &task
					return
				}
			}

		// If any items come through on the inputChan, then we try feed
		// these directly into the memQueue. If there is space in the
		// memeQueue then we respond with success to the producer,
		// otherwise we respond with failure so that the producer can
		// instead persist the task to disk.
		case task := <-q.inputChan:
			select {
			case q.memQueue <- task.task:
				task.success <- true
				continue
			default:
				task.success <- false
			}
		}
	}
}

// manageQueueHead will pop an item from the buffered memQueue and block until
// the item is taken from the un-buffered outputChan. This is done repeatedly
// for the lifetime of the DiskOverflowQueue. On shutdown of the queue, any
// item not consumed by the outputChan but held by this method is assigned to
// the leftOverItem1 member so that the Stop method can persist the item to
// disk so that it is reloaded on restart.
//
// NOTE: This must be run as a goroutine.
func (q *DiskOverflowQueue[T]) manageQueueHead() {
	defer func() {
		close(q.outputChan)
		q.wg.Done()
	}()

	for {
		select {
		case nextTask, ok := <-q.memQueue:
			// If the memQueue is closed, then the queue is
			// stopping.
			if !ok {
				return
			}

			select {
			case q.outputChan <- nextTask:
			case <-q.quit:
				q.leftOverItem1 = &nextTask
				return
			}

		case <-q.quit:
			return
		}
	}
}
