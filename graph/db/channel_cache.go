package graphdb

import "github.com/lightningnetwork/lnd/lnwire"

// channelCache is an in-memory cache used to improve the performance of
// ChanUpdatesInHorizon. It caches the chan info and edge policies for a
// particular channel.
type channelCache struct {
	n        int
	channels map[cacheKey]ChannelEdge
}

// newChannelCache creates a new channelCache with maximum capacity of n
// channels.
func newChannelCache(n int) *channelCache {
	return &channelCache{
		n:        n,
		channels: make(map[cacheKey]ChannelEdge),
	}
}

// get returns the channel from the cache for the given chanid and version, if
// it exists.
func (c *channelCache) get(chanid uint64, version lnwire.GossipVersion) (
	ChannelEdge, bool) {

	key := cacheKey{chanID: chanid, version: version}
	channel, ok := c.channels[key]
	return channel, ok
}

// insert adds the entry to the channel cache. If an entry for the given chanid
// and version already exists, it will be replaced with the new entry. If the
// entry doesn't exist, it will be inserted to the cache, performing a random
// eviction if the cache is at capacity.
func (c *channelCache) insert(chanid uint64, version lnwire.GossipVersion,
	channel ChannelEdge) {

	key := cacheKey{chanID: chanid, version: version}

	// If entry exists, replace it.
	if _, ok := c.channels[key]; ok {
		c.channels[key] = channel
		return
	}

	// Otherwise, evict an entry at random and insert.
	if len(c.channels) == c.n {
		for id := range c.channels {
			delete(c.channels, id)
			break
		}
	}
	c.channels[key] = channel
}

// remove deletes an edge for the given chanid and version from the cache, if
// it exists.
func (c *channelCache) remove(chanid uint64, version lnwire.GossipVersion) {
	key := cacheKey{chanID: chanid, version: version}
	delete(c.channels, key)
}
