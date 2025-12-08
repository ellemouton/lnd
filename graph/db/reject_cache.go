package graphdb

import "github.com/lightningnetwork/lnd/lnwire"

// cacheKey is a composite key used by the reject and channel caches to
// uniquely identify a channel by both its channel ID and gossip version.
// This ensures that V1 and V2 channels with the same ID don't collide in
// the cache.
type cacheKey struct {
	chanID  uint64
	version lnwire.GossipVersion
}

// rejectFlags is a compact representation of various metadata stored by the
// reject cache about a particular channel.
type rejectFlags uint8

const (
	// rejectFlagExists is a flag indicating whether the channel exists,
	// i.e. the channel is open and has a recent channel update. If this
	// flag is not set, the channel is either a zombie or unknown.
	rejectFlagExists rejectFlags = 1 << iota

	// rejectFlagZombie is a flag indicating whether the channel is a
	// zombie, i.e. the channel is open but has no recent channel updates.
	rejectFlagZombie
)

// packRejectFlags computes the rejectFlags corresponding to the passed boolean
// values indicating whether the edge exists or is a zombie.
func packRejectFlags(exists, isZombie bool) rejectFlags {
	var flags rejectFlags
	if exists {
		flags |= rejectFlagExists
	}
	if isZombie {
		flags |= rejectFlagZombie
	}

	return flags
}

// unpack returns the booleans packed into the rejectFlags. The first indicates
// if the edge exists in our graph, the second indicates if the edge is a
// zombie.
func (f rejectFlags) unpack() (bool, bool) {
	return f&rejectFlagExists == rejectFlagExists,
		f&rejectFlagZombie == rejectFlagZombie
}

// rejectCacheEntry caches frequently accessed information about a channel,
// including the timestamps of its latest edge policies and whether or not the
// channel exists in the graph.
type rejectCacheEntry struct {
	upd1Time int64
	upd2Time int64
	flags    rejectFlags
}

// rejectCache is an in-memory cache used to improve the performance of
// HasChannelEdge. It caches information about the whether or channel exists, as
// well as the most recent timestamps for each policy (if they exists).
type rejectCache struct {
	n     int
	edges map[cacheKey]rejectCacheEntry
}

// newRejectCache creates a new rejectCache with maximum capacity of n entries.
func newRejectCache(n int) *rejectCache {
	return &rejectCache{
		n:     n,
		edges: make(map[cacheKey]rejectCacheEntry, n),
	}
}

// get returns the entry from the cache for the given chanid and version, if it
// exists.
func (c *rejectCache) get(chanid uint64, version lnwire.GossipVersion) (
	rejectCacheEntry, bool) {

	key := cacheKey{chanID: chanid, version: version}
	entry, ok := c.edges[key]

	return entry, ok
}

// insert adds the entry to the reject cache. If an entry for the given chanid
// and version already exists, it will be replaced with the new entry. If the
// entry doesn't exist, it will be inserted to the cache, performing a random
// eviction if the cache is at capacity.
func (c *rejectCache) insert(chanid uint64, version lnwire.GossipVersion,
	entry rejectCacheEntry) {

	key := cacheKey{chanID: chanid, version: version}

	// If entry exists, replace it.
	if _, ok := c.edges[key]; ok {
		c.edges[key] = entry
		return
	}

	// Otherwise, evict an entry at random and insert.
	if len(c.edges) == c.n {
		for id := range c.edges {
			delete(c.edges, id)
			break
		}
	}
	c.edges[key] = entry
}

// remove deletes an entry for the given chanid and version from the cache, if
// it exists.
func (c *rejectCache) remove(chanid uint64, version lnwire.GossipVersion) {
	key := cacheKey{chanID: chanid, version: version}
	delete(c.edges, key)
}
