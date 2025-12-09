package graphdb

import (
	"time"

	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
)

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
// including the update indicators of its latest edge policies and whether or
// not the channel exists in the graph. For V1 channels, upd1 and upd2 store
// Unix timestamps. For V2 channels, they store block heights.
type rejectCacheEntry struct {
	upd1  int64
	upd2  int64
	flags rejectFlags
}

// rejectCache is an in-memory cache used to improve the performance of
// HasV1ChannelEdge. It caches information about the whether or channel exists, as
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

// policyUpdateValue extracts the update indicator from a policy. For V1
// policies, this is the Unix timestamp. For V2 policies, this is the block
// height. Returns 0 if the policy is nil.
func policyUpdateValue(policy *models.ChannelEdgePolicy) int64 {
	if policy == nil {
		return 0
	}

	if policy.Version == lnwire.GossipVersion1 {
		return policy.LastUpdate.Unix()
	}

	return int64(policy.LastBlockHeight)
}

// newRejectCacheEntry creates a rejectCacheEntry from two policies and flags.
// The policies can be nil if they don't exist. This handles both V1 and V2
// channels correctly.
func newRejectCacheEntry(policy1, policy2 *models.ChannelEdgePolicy,
	exists, isZombie bool) rejectCacheEntry {

	return rejectCacheEntry{
		upd1:  policyUpdateValue(policy1),
		upd2:  policyUpdateValue(policy2),
		flags: packRejectFlags(exists, isZombie),
	}
}

// newRejectCacheEntryFromTimes creates a rejectCacheEntry from update times
// for V1 channels or block heights for V2 channels.
func newRejectCacheEntryFromTimes(upd1, upd2 time.Time, version lnwire.GossipVersion,
	exists, isZombie bool) rejectCacheEntry {

	var upd1Val, upd2Val int64

	if version == lnwire.GossipVersion1 {
		if !upd1.IsZero() {
			upd1Val = upd1.Unix()
		}
		if !upd2.IsZero() {
			upd2Val = upd2.Unix()
		}
	} else {
		// For V2, the time.Time values are actually block heights
		// stored as Unix timestamps. Extract the Unix value directly.
		if !upd1.IsZero() {
			upd1Val = upd1.Unix()
		}
		if !upd2.IsZero() {
			upd2Val = upd2.Unix()
		}
	}

	return rejectCacheEntry{
		upd1:  upd1Val,
		upd2:  upd2Val,
		flags: packRejectFlags(exists, isZombie),
	}
}

// toTimes converts the cache entry's update values back to time.Time values
// based on the version. For V1, interprets as Unix timestamps. For V2,
// interprets as block heights encoded as Unix timestamps.
func (e rejectCacheEntry) toTimes(version lnwire.GossipVersion) (time.Time, time.Time) {
	var upd1Time, upd2Time time.Time

	if e.upd1 != 0 {
		upd1Time = time.Unix(e.upd1, 0)
	}
	if e.upd2 != 0 {
		upd2Time = time.Unix(e.upd2, 0)
	}

	return upd1Time, upd2Time
}
