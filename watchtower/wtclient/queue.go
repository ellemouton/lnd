package wtclient

import (
	"container/list"
	"sync"
	"time"

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

	// modeDiskPrevMem means that there are items on disk that represent
	// items that were in the queue before one of the previous restarts.
	// Note that the maxQueueSize value could have changed between restarts
	// meaning that it could be the case that not all items that previously
	// fit into the queue can still do so now. In this mode, the in-memory
	// queue should be fed new items from this prev-mem queue. Any new items
	// should be persisted in the main disk queue. Once all the items from
	// the prev-mem disk queue have been fed into the in-memory channel,
	// the mode should be switched to modeDisk.
	modeDiskPrevMem

	// modeDisk means that the queue should be fed new items from the main
	// disk queue and any new items should be persisted to the main disk
	// queue. Only once all items have been read from disk _and_ there is
	// space in the in-memory channel should the mode be switched to
	// modeMem.
	modeDisk

	// prevMemNamespace is the name of the on-disk queue used to store any
	// items that are in the in-memory queue on shutdown so that they can
	// be reloaded on restart.
	prevMemNamespace = "prev-mem-queue"

	// diskNamespace is the name of the on-disk queue used to store any
	// items added to the queue when the in-memory queue is saturated.
	diskNamespace = "disk-queue"

	// dbErrorBackoff is the length of time we will back off before retrying
	// any DB action that failed.
	dbErrorBackoff = time.Second * 5
)

// DiskOverflowQueueDB is an interface that must be satisfied by a database
// backing the DiskOverflowQueue. It should mimic the behaviour of a FIFO queue.
// The DB must also allow the caller to specify a namespace so that it can store
// different wtdb.BackupID in different queues determined by the namespace if
// required.
type DiskOverflowQueueDB interface {
	// NumTasks returns the number of tasks in the queue of the given
	// namespace.
	NumTasks(namespace string) (uint64, error)

	// AddTask adds a wtdb.BackupID to the queue of the given namespace.
	AddTask(namespace string, t *wtdb.BackupID) error

	// NextTask pops the next wtdb.BackupID from the queue of the given
	// namespace. If no more items are in the queue then nil is returned.
	NextTask(namespace string) (*wtdb.BackupID, error)
}

// DiskOverflowQueue is a queue that must be initialised with a certain maximum
// buffer size which represents the maximum number of elements that the queue
// should hold in memory. If the queue is full, then any new elements added to
// the queue will be persisted to disk instead. Once a consumer starts reading
// from the front of the queue again then items on disk will be moved into the
// queue again. The queue is also re-start safe. When it is stopped, any items
// in the memory queue, will be persisted to disk. On start up, the queue will
// be re-initialised with the items on disk.
type DiskOverflowQueue struct {
	startOnce sync.Once
	stopOnce  sync.Once

	// db is the database that will be used to persist queue items to disk.
	db DiskOverflowQueueDB

	// mode represents the current mode of operation of the queue.
	mode   mode
	modeMu sync.Mutex

	// We used an unbound list for the input of the queue so that producers
	// putting items into the queue are never blocked.
	inputListMu   sync.Mutex
	inputListCond *sync.Cond
	inputList     *list.List

	inputChan  chan *internalTask
	memQueue   chan *wtdb.BackupID
	outputChan chan *wtdb.BackupID

	// newDiskItemSignal is used to signal that there is a new item in the
	// main disk queue. There should only be one reader and one writer for
	// this channel.
	newDiskItemSignal chan struct{}

	quit chan struct{}
	wg   sync.WaitGroup

	// leftOverItem1 will be a non-nil task on shutdown if the
	// manageQueueHead method was holding any unhandled tasks on shutdown.
	// Since manageQueueHead handles the very head of the queue, this item
	// should be the first to be reloaded on restart.
	leftOverItem1 *wtdb.BackupID

	//
	leftOverItem2 *wtdb.BackupID

	// this one should really go in disk-queue. not mem-queue. same for
	// any list items.
	leftOverItem3 *wtdb.BackupID
}

// NewDiskOverflowQueue constructs a new DiskOverflowQueue.
func NewDiskOverflowQueue(db DiskOverflowQueueDB,
	maxQueueSize int) *DiskOverflowQueue {

	q := &DiskOverflowQueue{
		mode:              modeMem,
		db:                db,
		inputList:         list.New(),
		newDiskItemSignal: make(chan struct{}, 1),
		inputChan:         make(chan *internalTask),
		memQueue:          make(chan *wtdb.BackupID, maxQueueSize-1),
		outputChan:        make(chan *wtdb.BackupID),
		quit:              make(chan struct{}),
	}
	q.inputListCond = sync.NewCond(&q.inputListMu)

	return q
}

// Start starts the queue and re-initialises the queue with any items that are
// on in the previous-memory part of the disk. It also kicks off all the
// goroutines that are required to manage the queue.
func (q *DiskOverflowQueue) Start() error {
	var err error
	q.startOnce.Do(func() {
		err = q.start()
	})
	return err
}

// Start the queue and re-initialises the queue with any items that are on in
// the previous-memory part of the disk. It also kicks off all the goroutines
// that are required to manage the queue.
func (q *DiskOverflowQueue) start() error {
	// Before starting with normal operations, we fill the memQueue with
	// any items that we may have persisted to the prev-mem disk queue
	// previously.
	var leftOverTask *wtdb.BackupID
	for {
		memTask, err := q.db.NextTask(prevMemNamespace)
		if err != nil {
			return err
		}

		// There are no more items in the prev-mem disk queue, so we
		// can start normal operation.
		if memTask == nil {
			break
		}

		// We found an item in the prev-mem disk queue. We attempt to
		// feed it into the buffered memQueue. If there is space, then
		// we continue attempting to read more items from the prev-mem
		// disk queue.
		select {
		case q.memQueue <- memTask:
			continue
		default:
		}

		// If there is no more space in the buffered memQueue, then
		// we can break out of this loop. We will pass this task onto
		// the manageMemQueueInput goroutine though so that it is
		// properly handled once there is space in the memQueue.
		leftOverTask = memTask
		break
	}

	// The default start mode of the queue is memMode. We switch it to
	// diskMode if there are currently items in the disk queue that need to
	// be handled.
	numDisk, err := q.db.NumTasks(diskNamespace)
	if err != nil {
		return err
	}
	if numDisk != 0 {
		q.setMode(modeDisk)
	}

	// However, if there are still items in the prev-mem disk queue, then
	// these take priority, and so we switch the mode to modeDiskPrevMem.
	numMem, err := q.db.NumTasks(prevMemNamespace)
	if err != nil {
		return err
	}
	if numMem != 0 {
		q.setMode(modeDiskPrevMem)
	}

	// Kick off the three goroutines which will handle the input list, the
	// in-memory queue and the output channel.
	q.wg.Add(3)
	go q.manageQueueTail()
	go q.manageMemQueueInput(leftOverTask)
	go q.manageQueueHead()

	return nil
}

// Stop stops the queue and persists any items in the memory queue to disk.
func (q *DiskOverflowQueue) Stop() error {
	var err error
	q.stopOnce.Do(func() {
		err = q.stop()
	})
	return err
}

// stop the queue and persists any items in the memory queue to disk.
func (q *DiskOverflowQueue) stop() error {
	close(q.quit)

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

	var prevMemQueue []*wtdb.BackupID

	if q.leftOverItem1 != nil {
		prevMemQueue = append(prevMemQueue, q.leftOverItem1)
	}

	// Drain the buffered queue.
	for len(q.memQueue) > 0 {
		prevMemQueue = append(prevMemQueue, <-q.memQueue)
	}

	if q.leftOverItem2 != nil {
		prevMemQueue = append(prevMemQueue, q.leftOverItem2)
	}

	// Drain any existing persisted items to the back of this queue.
	for {
		memTask, err := q.db.NextTask(prevMemNamespace)
		if err != nil {
			log.Error("could not fetch next memory task from disk")
			continue
		}

		if memTask == nil {
			break
		}

		prevMemQueue = append(prevMemQueue, memTask)
	}

	// Now persist those so that they can be reloaded on startup.
	for _, task := range prevMemQueue {
		err := q.db.AddTask(prevMemNamespace, task)
		if err != nil {
			log.Errorf("could not persist %s to the prev-mem "+
				"disk queue", task)
		}
	}

	var diskQueue []*wtdb.BackupID
	if q.leftOverItem3 != nil {
		diskQueue = append(diskQueue, q.leftOverItem3)
	}

	// Drain anything left in the in-list.
	for q.inputList.Front() != nil {
		e := q.inputList.Front()
		task := q.inputList.Remove(e).(*wtdb.BackupID)

		diskQueue = append(diskQueue, task)
	}

	// Now persist those so that they can be reloaded on startup.
	for _, task := range diskQueue {
		err := q.db.AddTask(diskNamespace, task)
		if err != nil {
			log.Errorf("could not persist %s to the disk queue",
				task)
		}
	}

	return nil
}

// setMode sets the mode of the queue.
func (q *DiskOverflowQueue) setMode(m mode) {
	q.modeMu.Lock()
	defer q.modeMu.Unlock()

	q.mode = m
}

// getMode returns the current mode of the queue.
func (q *DiskOverflowQueue) getMode() mode {
	q.modeMu.Lock()
	defer q.modeMu.Unlock()

	return q.mode
}

// QueueBackupID adds a wtdb.BackupID to the queue. It will only return an error
// if the queue has been stopped. It is non-blocking.
func (q *DiskOverflowQueue) QueueBackupID(item *wtdb.BackupID) error {
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
func (q *DiskOverflowQueue) NextBackupID() <-chan *wtdb.BackupID {
	return q.outputChan
}

// internalTask wraps a BackupID task with a success channel.
type internalTask struct {
	task    *wtdb.BackupID
	success chan bool
}

// manageQueueTail handles the input to the DiskOverflowQueue. It takes from the
// un-bounded input list and then, depending on what mode the queue is in,
// either puts the new item straight onto the persisted disk queue or attempts
// to feed it into the memQueue. On exit, any unhandled task will be assigned to
// leftOverItem3.
func (q *DiskOverflowQueue) manageQueueTail() {
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
		task := q.inputList.Remove(e).(*wtdb.BackupID)
		q.inputListCond.L.Unlock()

		// What we do with this new item depends on what the mode of the
		// queue currently is.
	checkMode:
		switch q.getMode() {
		// If the queue is in either modeDiskPrevMem or modeDisk then
		// any new items should be put straight into the disk queue.
		case modeDiskPrevMem, modeDisk:
			err := q.db.AddTask(diskNamespace, task)
			if err != nil {
				// Log and back off for a few seconds and then
				// try again with the same task.
				log.Errorf("could not persist %s to disk. "+
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
					q.leftOverItem3 = task
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
			it := &internalTask{
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
				q.leftOverItem3 = task

				return

			default:
				// Was not accepted. So maybe the mode changed.
				goto checkMode
			}

			// If we get here, it means that the manageMemQueueInput
			// handler took the task. It is guaranteed to respond
			// via the success channel, so we wait for that response
			// here.
			select {
			case s := <-success:
				if s {
					continue
				}
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
// memQueue. If it is provided with a non-nil memTask left over from startup,
// then it will line this task up as the first one to be fed into the memQueue.
// Then, if the queue is still in modeDiskPrevMem, then it will read new tasks
// from the prev-mem disk queue until there are no more tasks left to read from
// that queue. If the queue is then in modeDisk, then the handler will read new
// tasks from the disk queue until it is empty.
func (q *DiskOverflowQueue) manageMemQueueInput(memTask *wtdb.BackupID) {
	defer q.wg.Done()

	if memTask != nil {
		select {
		case q.memQueue <- memTask:
		case <-q.quit:
			q.leftOverItem2 = memTask
			return
		}
	}

	// If the mode is initially in pre-mem-disk mode then it means that
	// there are some items on in the prev-mem disk queue that we should
	// read into the in-mem queue before any other items.
	if q.getMode() == modeDiskPrevMem {
		for {
			task, err := q.db.NextTask(prevMemNamespace)
			if err != nil {
				log.Error("could not fetch next task from " +
					"disk. Retrying")

				select {
				case <-time.After(dbErrorBackoff):
					continue
				case <-q.quit:
					return
				}
			}

			// If the returned item is nil, then there are no more
			// prev-mem tasks to read from disk. So we can now
			// change the mode to modeDisk so that the disk is next
			// checked for any items to add to the queue.
			if task == nil {
				q.setMode(modeDisk)
				break
			}

			// Otherwise, we got an item from the prev-mem disk
			// queue, and so we try and feed it into the buffered
			// memory queue now.
			select {
			case q.memQueue <- task:

			// If the queue is quit at this moment, then the
			// unhandled task is assigned to leftOverItem2 so that
			// it can be handled by the stop method.
			case <-q.quit:
				q.leftOverItem2 = task
				return
			}
		}
	}

	// If the queue is in modeDisk, then the memQueue is fed with tasks
	// from the disk queue until it is empty.
	if q.getMode() == modeDisk {
		for {
			task, err := q.db.NextTask(diskNamespace)
			if err != nil {
				log.Errorf("could not load next task from " +
					"disk. Retrying.")

				select {
				case <-time.After(dbErrorBackoff):
					continue
				case <-q.quit:
					return
				}
			}

			if task == nil {
				q.setMode(modeMem)
				break
			}

			select {
			case q.memQueue <- task:

			// If the queue is quit at this moment, then the
			// unhandled task is assigned to leftOverItem2 so that
			// it can be handled by the stop method.
			case <-q.quit:
				q.leftOverItem2 = task
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
				task, err := q.db.NextTask(diskNamespace)
				if err != nil {
					log.Errorf("could not load next task " +
						"from disk. Retrying.")

					select {
					case <-time.After(dbErrorBackoff):
						continue
					case <-q.quit:
						return
					}
				}

				// If the disk-queue is empty, then we switch
				// back to modeMem.
				if task == nil {
					q.setMode(modeMem)
					break
				}

				select {
				case q.memQueue <- task:

				// If the queue is quit at this moment, then the
				// unhandled task is assigned to leftOverItem2
				// so that it can be handled by the stop method.
				case <-q.quit:
					q.leftOverItem2 = task
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
func (q *DiskOverflowQueue) manageQueueHead() {
	defer func() {
		close(q.outputChan)
		q.wg.Done()
	}()

	var nextTask *wtdb.BackupID
	for {
		select {
		case nextTask = <-q.memQueue:
		case <-q.quit:
			return
		}

		select {
		case q.outputChan <- nextTask:
		case <-q.quit:
			q.leftOverItem1 = nextTask
			return
		}
	}
}
