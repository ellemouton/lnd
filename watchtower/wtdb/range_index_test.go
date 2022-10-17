package wtdb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRangeIndex tests that the RangeIndex works as expected.
func TestRangeIndex(t *testing.T) {
	// Construct a new range index.
	index := NewRangeIndex()

	// Since zero values are tricky, assert that the index does not include
	// zero.
	require.False(t, index.IsInIndex(0))

	// Add various numbers and assert that the index is updated as
	// expected.
	index.Add(1)
	index.Add(1)

	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 1, index.getRangeMax(1))
	require.EqualValues(t, 1, index.MaxHeight())
	require.True(t, index.IsInIndex(1))
	require.False(t, index.IsInIndex(3))
	require.False(t, index.IsInIndex(0))

	// Test that the existing range is just extended with these new numbers.
	index.Add(2)
	index.Add(3)
	index.Add(4)

	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 4, index.getRangeMax(1))
	require.EqualValues(t, 4, index.MaxHeight())
	require.True(t, index.IsInIndex(3))

	// Since the existing range ends at 4, test that adding the following
	// numbers creates a new range.
	index.Add(6)
	index.Add(7)
	index.Add(8)

	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 4, index.getRangeMax(1))
	require.EqualValues(t, 8, index.getRangeMax(6))
	require.EqualValues(t, 8, index.MaxHeight())

	index.Add(12)
	index.Add(13)

	require.EqualValues(t, 3, index.numRanges())
	require.EqualValues(t, 4, index.getRangeMax(1))
	require.EqualValues(t, 8, index.getRangeMax(6))
	require.EqualValues(t, 13, index.getRangeMax(12))
	require.EqualValues(t, 13, index.MaxHeight())

	// Adding 5 should merge two of the ranges.
	index.Add(5)

	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 8, index.getRangeMax(1))
	require.EqualValues(t, 13, index.getRangeMax(12))

	index.Add(10)

	require.EqualValues(t, 3, index.numRanges())
	require.EqualValues(t, 8, index.getRangeMax(1))
	require.EqualValues(t, 10, index.getRangeMax(10))
	require.EqualValues(t, 13, index.getRangeMax(12))
	require.EqualValues(t, 11, index.NumInSet())

	index.Add(11)

	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 8, index.getRangeMax(1))
	require.EqualValues(t, 13, index.getRangeMax(10))

	index.Add(9)

	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 13, index.MaxHeight())

	// Make sure that things already in the range dont affect the index.
	index.Add(9)
	index.Add(7)
	index.Add(2)
	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 13, index.MaxHeight())

	// Test the transaction commit & revert.
	tx := index.StartTx()
	changes := tx.GetChanges(20)
	require.NotNil(t, changes)
	require.EqualValues(t, 20, *changes.NewIndexKey)
	require.EqualValues(t, 20, *changes.NewIndexValue)
	require.Nil(t, changes.DeleteIndex)

	tx.Revert()
	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 13, index.MaxHeight())
	require.True(t, index.IsInIndex(13))
	require.False(t, index.IsInIndex(18))

	tx = index.StartTx()
	changes = tx.GetChanges(20)
	require.NotNil(t, changes)
	require.EqualValues(t, 20, *changes.NewIndexKey)
	require.EqualValues(t, 20, *changes.NewIndexValue)
	require.Nil(t, changes.DeleteIndex)
	tx.Commit(changes)

	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 20, index.getRangeMax(20))
	require.EqualValues(t, 20, index.MaxHeight())
	require.EqualValues(t, 14, index.NumInSet())
	require.True(t, index.IsInIndex(13))
	require.False(t, index.IsInIndex(18))
	require.False(t, index.IsInIndex(0))

	// Test adding ranges to the set.

	// Adding an existing range should have no effect.
	index.AddRange(1, 13)
	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 20, index.getRangeMax(20))
	require.EqualValues(t, 20, index.MaxHeight())

	index.AddRange(21, 25)
	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 25, index.getRangeMax(20))
	require.EqualValues(t, 25, index.MaxHeight())

	index.AddRange(15, 20)
	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 25, index.getRangeMax(15))
	require.EqualValues(t, 25, index.MaxHeight())
	require.True(t, index.IsInIndex(13))
	require.True(t, index.IsInIndex(18))

	index.AddRange(16, 26)
	require.EqualValues(t, 2, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 26, index.getRangeMax(15))
	require.EqualValues(t, 26, index.MaxHeight())

	index.AddRange(28, 30)
	require.EqualValues(t, 3, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 26, index.getRangeMax(15))
	require.EqualValues(t, 30, index.getRangeMax(28))
	require.EqualValues(t, 30, index.MaxHeight())

	index.AddRange(32, 34)
	require.EqualValues(t, 4, index.numRanges())
	require.EqualValues(t, 13, index.getRangeMax(1))
	require.EqualValues(t, 26, index.getRangeMax(15))
	require.EqualValues(t, 30, index.getRangeMax(28))
	require.EqualValues(t, 34, index.getRangeMax(32))
	require.EqualValues(t, 34, index.MaxHeight())

	index.AddRange(14, 35)
	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 35, index.getRangeMax(1))
	require.EqualValues(t, 35, index.MaxHeight())
	require.EqualValues(t, 35, index.NumInSet())
	require.True(t, index.IsInIndex(30))
	require.False(t, index.IsInIndex(36))

	index.Add(0)
	require.EqualValues(t, 1, index.numRanges())
	require.EqualValues(t, 35, index.getRangeMax(0))
	require.True(t, index.IsInIndex(0))
}
