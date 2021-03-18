package blockcache

import (
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/lightninglabs/neutrino/cache"
	"github.com/lightninglabs/neutrino/cache/lru"
)

// BlockCache is an lru cache for blocks.
type BlockCache struct {
	Cache     *lru.Cache
	blockMtxs *blockMutex
}

// NewBlockCache creates a new BlockCache with the given maximum capacity.
func NewBlockCache(capacity uint64) *BlockCache {
	return &BlockCache{
		Cache: lru.NewCache(capacity),
		blockMtxs: &blockMutex{
			mutexMap: make(map[chainhash.Hash]*blockEntry),
		},
	}
}

// GetBlock first checks to see if the BlockCache already contains the block
// with the given hash. If it does then the block is fetched from the cache and
// returned. Otherwise the getBlockImpl function is used in order to fetch the
// new block and then it is stored in the block cache and returned.
func (bc *BlockCache) GetBlock(hash *chainhash.Hash,
	getBlockImpl func(hash *chainhash.Hash) (*wire.MsgBlock,
		error)) (*wire.MsgBlock, error) {

	bc.LockHash(*hash)
	defer bc.UnlockHash(*hash)

	// Create an inv vector for getting the block.
	inv := wire.NewInvVect(wire.InvTypeWitnessBlock, hash)

	var block *wire.MsgBlock

	// Check if the block corresponding to the given hash is already
	// stored in the blockCache and return it if it is.
	cacheBlock, err := bc.Cache.Get(*inv)
	if err != nil && err != cache.ErrElementNotFound {
		return nil, err
	}
	if cacheBlock != nil {
		return cacheBlock.(*cache.CacheableBlock).MsgBlock(), nil
	}

	// Fetch the block from the chain backends.
	block, err = getBlockImpl(hash)
	if err != nil {
		return nil, err
	}

	// Add the new block to blockCache. If the Cache is at its maximum
	// capacity then the LFU item will be evicted in favour of this new
	// block.
	_, err = bc.Cache.Put(
		*inv, &cache.CacheableBlock{
			Block: btcutil.NewBlock(block),
		},
	)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// blockMutex holds a map of mutexes. Each block hash will have its own mutex.
type blockMutex struct {
	mu       sync.Mutex
	mutexMap map[chainhash.Hash]*blockEntry
}

// blockEntry wraps a mutex with a reference count.
type blockEntry struct {
	mu       sync.Mutex
	refCount int
}

// LockHash is used to lock the mutex for a particular block hash.
func (bc *BlockCache) LockHash(hash chainhash.Hash) {
	bc.blockMtxs.mu.Lock()
	be, ok := bc.blockMtxs.mutexMap[hash]
	if !ok {
		be = &blockEntry{}
		bc.blockMtxs.mutexMap[hash] = be
	}
	be.refCount++
	bc.blockMtxs.mu.Unlock()

	be.mu.Lock()
}

// UnlockHash is used to unlock the mutex for a particular block hash. If the
// go routine calling unlock for a particular hash is the last one doing so
// then the mutex for the hash is deleted from the mutex map. Note that
// UnlockHash must only ever be called after LockHash has been called for the
// given hash.
func (bc *BlockCache) UnlockHash(hash chainhash.Hash) {
	bc.blockMtxs.mu.Lock()
	be := bc.blockMtxs.mutexMap[hash]
	be.refCount--
	if be.refCount < 1 {
		delete(bc.blockMtxs.mutexMap, hash)
	}
	bc.blockMtxs.mu.Unlock()

	be.mu.Unlock()
}
