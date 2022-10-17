package migration5

import (
	"sync"
)

// RangeIndex can be used go keep track of which numbers have been added to a
// set. It does so by keeping track of the start and end values of a given range
// where all values in-between have been added to the set. It works well in
// situations where it is expected numbers in the set are not sparse.
type RangeIndex struct {
	// m is a map of ranges. The keys represent the start height of the
	// range and the values represent the max height in that range.
	m map[uint64]uint64

	// maxHeight keeps track of the maximum number covered in the range set.
	maxHeight uint64

	mu sync.Mutex
}

// NewRangeIndex constructs a new RangeIndex.
func NewRangeIndex() *RangeIndex {
	return &RangeIndex{
		m: make(map[uint64]uint64),
	}
}

// GetAllRanges returns a copy of the range set.
func (a *RangeIndex) GetAllRanges() map[uint64]uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	cp := make(map[uint64]uint64, len(a.m))
	for k, v := range a.m {
		cp[k] = v
	}

	return cp
}

// AddRange can be used to add an entire new range to the set. It is the
// responsibility of the caller to ensure that the start value is less than or
// equal to the end value in the range.
func (a *RangeIndex) AddRange(start, end uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if start > end {
		return
	}

	// If the range set is empty, the new range can just be added.
	if len(a.m) == 0 {
		a.m[start] = end
		return
	}

	// The range is already covered in the index.
	if num, ok := a.m[start]; ok && end <= num {
		return
	}

	// First we assume that the given start value will be the key of the
	// range we will edit.
	rangeStart := start

	// Check if there is a range with a start key that is less than the
	// start key in question.
	rangeExists, _, rangeBelow := a.highestLowerRange(start)
	if rangeExists {
		// If such a range does exist, then we check to see if the
		// range we are trying to insert is perhaps already covered by
		// this range.
		num := a.m[rangeBelow]
		if end <= num {
			// The range is already covered in the index.
			return
		}

		// Otherwise, check if we can perhaps just extend the existing
		// range.
		if num >= start || num+1 == start {
			// Extend an existing range.
			rangeStart = rangeBelow
		}
	}

	// Write the new range.
	a.m[rangeStart] = end

	// Update our maxHeight if needed.
	if end > a.maxHeight {
		a.maxHeight = end
	}

	// Now check if we need to merge any another ranges with this one.
	for {
		// Find the start value of the lowest range that is still
		// higher than the start value that we just inserted.
		rangeExists, rangeAbove := a.lowestHigherRange(rangeStart)

		// If no such range exists, then there is no range to delete.
		if !rangeExists {
			return
		}

		// If there is such a range, but it cannot be merged with the
		// newly inserted range, then we are done.
		if rangeAbove > end+1 {
			return
		}

		// Otherwise, the two ranges can be merged.
		num := a.m[rangeAbove]
		if num >= end {
			a.m[rangeStart] = num
		}

		delete(a.m, rangeAbove)
	}
}

// IsInIndex returns true if the given number is in the range set.
func (a *RangeIndex) IsInIndex(n uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.m[n]; ok {
		return true
	}

	_, isInRange, _ := a.highestLowerRange(n)
	return isInRange
}

// NumInSet returns the number of items covered by the range set.
func (a *RangeIndex) NumInSet() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	var numAcked uint64

	for start, end := range a.m {
		numAcked += end - start + 1
	}

	return numAcked
}

// highestLowerRange finds the largest start value of a range in the set where
// the start value is still lower than the given value. The first return value
// is true if such a range exists. The second value returns true if the given
// number is included in the found range. The last return value is the start
// value of the found range.
func (a *RangeIndex) highestLowerRange(h uint64) (bool, bool, uint64) {
	var (
		rangeExists   bool
		rangeStartNum uint64
	)
	for height, maxHeight := range a.m {
		if height > h || height < rangeStartNum {
			continue
		}

		rangeExists = true
		rangeStartNum = height

		// If the new height is smaller than or equal to the max height
		// of this range, then we can exit early since the height is
		// covered by this range.
		if h <= maxHeight {
			return true, true, rangeStartNum
		}
	}

	return rangeExists, false, rangeStartNum
}

// lowestHigherRange finds the lowest start value of a range in the set where
// the start value is still higher than the given value. The first return value
// is true if such a range exists. The last return value is the start value of
// the found range.
func (a *RangeIndex) lowestHigherRange(h uint64) (bool, uint64) {
	var (
		rangeExists bool
		rangeStart  uint64
	)
	for height, maxHeight := range a.m {
		if height <= h {
			continue
		}

		if rangeExists && height > rangeStart {
			continue
		}

		rangeExists = true
		rangeStart = height

		// If the new height is smaller than or equal to the max height
		// of this range, then we can exit early since the height is
		// covered by this range.
		if h <= maxHeight {
			return true, rangeStart
		}
	}

	return rangeExists, rangeStart
}

// MaxHeight returns the highest number covered in the range.
func (a *RangeIndex) MaxHeight() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.maxHeight
}

// Add adds a single number to the range set.
func (a *RangeIndex) Add(newHeight uint64) {
	tx := a.StartTx()
	tx.Commit(tx.GetChanges(newHeight))
}

// IndexChanges represents the set of changes that must be performed on a
// range set in order to add a new number.
type IndexChanges struct {
	NewIndexKey   *uint64
	NewIndexValue *uint64
	DeleteIndex   *uint64
}

// RangeIndexTx represents a transaction on the RangeIndex.
type RangeIndexTx struct {
	*RangeIndex
}

// StartTx returns a RangeIndexTx which can be used to perform atomic operations
// on a RangeIndex. A caller can call GetChanges on the transaction to get the
// set of changes that need to be performed on the range set in order to add
// a new number. Commit can then be called to commit then changes or Revert can
// ge used to cancel the transaction.
func (a *RangeIndex) StartTx() *RangeIndexTx {
	a.mu.Lock()

	return &RangeIndexTx{RangeIndex: a}
}

// Commit can be used to commit a set of IndexChanges to a RangeIndex.
func (a *RangeIndexTx) Commit(changes *IndexChanges) {
	defer a.mu.Unlock()

	if changes == nil {
		return
	}

	if changes.NewIndexKey != nil && changes.NewIndexValue != nil {
		a.m[*changes.NewIndexKey] = *changes.NewIndexValue

		if *changes.NewIndexValue > a.maxHeight {
			a.maxHeight = *changes.NewIndexValue
		}
	}

	if changes.DeleteIndex != nil {
		delete(a.m, *changes.DeleteIndex)
	}
}

// GetChanges will calculate and return the set of changes that need to be
// applied to a range index in order to accommodate the new value.
func (a *RangeIndexTx) GetChanges(newNum uint64) *IndexChanges {
	// If the new height is the start height for any one of the ranges,
	// we can exit early.
	if _, ok := a.m[newNum]; ok {
		return nil
	}

	if len(a.m) == 0 {
		return &IndexChanges{
			NewIndexKey:   &newNum,
			NewIndexValue: &newNum,
		}
	}

	// Find the highest start height that is still smaller than the new
	// height.
	rangeExists, alreadyCovered, rangeBelow := a.highestLowerRange(
		newNum,
	)
	if alreadyCovered {
		return nil
	}

	// We now check if the new height could just extend the existing range.
	// If so, then we just increment the max height in the range and return.
	// Otherwise, it means that we need to add a new range to the index.

	maxHeight := a.m[rangeBelow]
	if !rangeExists {
		maxHeight = newNum
	}

	currentIndex := newNum
	if newNum == maxHeight+1 {
		currentIndex = rangeBelow
	}

	// If there exists another range with a starting height of newNum+1,
	// then this range can be condensed into the same range as the new
	// height. Otherwise, we are done.
	newMax, ok := a.m[newNum+1]
	if !ok {
		return &IndexChanges{
			NewIndexKey:   &currentIndex,
			NewIndexValue: &newNum,
		}
	}

	toDelete := newNum + 1

	return &IndexChanges{
		NewIndexKey:   &currentIndex,
		NewIndexValue: &newMax,
		DeleteIndex:   &toDelete,
	}
}
