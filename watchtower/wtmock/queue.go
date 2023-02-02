package wtmock

import (
	"container/list"
	"sync"

	"github.com/lightningnetwork/lnd/watchtower/wtdb"
)

type DiskOverflowDB struct {
	disk map[string]*list.List
	mu   sync.RWMutex
}

func NewQueueDB() *DiskOverflowDB {
	d := &DiskOverflowDB{
		disk: make(map[string]*list.List),
	}

	return d
}

func (d *DiskOverflowDB) NumTasks(namespace string) (uint64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	l, ok := d.disk[namespace]
	if !ok {
		return 0, nil
	}

	return uint64(l.Len()), nil
}

func (d *DiskOverflowDB) AddTask(namespace string, t *wtdb.BackupID) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	l, ok := d.disk[namespace]
	if !ok {
		l = list.New()
		d.disk[namespace] = l
	}

	l.PushBack(t)

	return nil
}

func (d *DiskOverflowDB) NextTask(namespace string) (*wtdb.BackupID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	l, ok := d.disk[namespace]
	if !ok {
		return nil, nil
	}

	if l.Len() == 0 {
		return nil, nil
	}

	e := l.Front()
	task := l.Remove(e).(*wtdb.BackupID)

	return task, nil
}
