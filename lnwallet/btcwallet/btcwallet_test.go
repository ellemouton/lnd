package btcwallet

import (
	"errors"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightninglabs/neutrino/cache"
	"github.com/lightninglabs/neutrino/cache/lru"
	"github.com/stretchr/testify/require"
)

// TestGetBlock tests that the block cache works correctly as a LFU block
// cache for the given max capacity.
func TestGetBlock(t *testing.T) {
	// A new cache is set up with a capacity of 2 blocks
	bc := lru.NewCache(2)
	mc := newMockChain()

	bw := &BtcWallet{
		chain:      mc,
		blockCache: bc,
	}

	block1 := &wire.MsgBlock{Header: wire.BlockHeader{Nonce: 1}}
	block2 := &wire.MsgBlock{Header: wire.BlockHeader{Nonce: 2}}
	block3 := &wire.MsgBlock{Header: wire.BlockHeader{Nonce: 3}}

	blockhash1 := block1.BlockHash()
	blockhash2 := block2.BlockHash()
	blockhash3 := block3.BlockHash()

	mc.addBlock(&wire.MsgBlock{}, 1)
	mc.addBlock(&wire.MsgBlock{}, 2)
	mc.addBlock(&wire.MsgBlock{}, 3)

	// We expect the initial cache to be empty
	require.Equal(t, 0, bc.Len())

	// After calling GetBlock for block1, it is expected that the cache
	// will have a size of 1 and will contain block1. One chain backends
	// call is expected to fetch the block.
	_, err := bw.GetBlock(&blockhash1)
	require.NoError(t, err)
	require.Equal(t, 1, bc.Len())
	require.Equal(t, 1, mc.chainCallCount)
	mc.resetChainCallCount()

	_, err = bc.Get(blockhash1)
	require.NoError(t, err)

	// After calling GetBlock for block2, it is expected that the cache
	// will have a size of 2 and will contain both block1 and block2.
	// One chain backends call is expected to fetch the block.
	_, err = bw.GetBlock(&blockhash2)
	require.NoError(t, err)
	require.Equal(t, 2, bc.Len())
	require.Equal(t, 1, mc.chainCallCount)
	mc.resetChainCallCount()

	_, err = bc.Get(blockhash1)
	require.NoError(t, err)

	_, err = bc.Get(blockhash2)
	require.NoError(t, err)

	// GetBlock is called again for block1 to make block2 the LFU block.
	// No call to the chain backend is expected since block 1 is already
	// in the cache.
	_, err = bw.GetBlock(&blockhash1)
	require.NoError(t, err)
	require.Equal(t, 2, bc.Len())
	require.Equal(t, 0, mc.chainCallCount)
	mc.resetChainCallCount()

	// Since the cache is now at its max capacity, it is expected that when
	// getBlock is called for a new block then the LFU block will be
	// evicted. It is expected that block2 will be evicted. After calling
	// Getblock for block3, it is expected that the cache will have a
	// length of 2 and will contain block 1 and 3.
	_, err = bw.GetBlock(&blockhash3)
	require.NoError(t, err)
	require.Equal(t, 2, bc.Len())
	require.Equal(t, 1, mc.chainCallCount)
	mc.resetChainCallCount()

	_, err = bc.Get(blockhash1)
	require.NoError(t, err)

	_, err = bc.Get(blockhash2)
	require.True(t, errors.Is(err, cache.ErrElementNotFound))

	_, err = bc.Get(blockhash3)
	require.NoError(t, err)
}
