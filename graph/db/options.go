package graphdb

import "time"

const (
	// DefaultRejectCacheSize is the default number of rejectCacheEntries to
	// cache for use in the rejection cache of incoming gossip traffic. This
	// produces a cache size of around 1MB.
	DefaultRejectCacheSize = 50000

	// DefaultChannelCacheSize is the default number of ChannelEdges cached
	// in order to reply to gossip queries. This produces a cache size of
	// around 40MB.
	DefaultChannelCacheSize = 20000
)

// StoreOptions holds parameters for tuning and customizing a graph DB.
type StoreOptions struct {
	// RejectCacheSize is the maximum number of rejectCacheEntries to hold
	// in the rejection cache.
	RejectCacheSize int

	// ChannelCacheSize is the maximum number of ChannelEdges to hold in the
	// channel cache.
	ChannelCacheSize int

	// BatchCommitInterval is the maximum duration the batch schedulers will
	// wait before attempting to commit a pending set of updates.
	BatchCommitInterval time.Duration

	// NoMigration specifies that underlying backend was opened in read-only
	// mode and migrations shouldn't be performed. This can be useful for
	// applications that use the channeldb package as a library.
	NoMigration bool
}

// DefaultOptions returns a StoreOptions populated with default values.
func DefaultOptions() *StoreOptions {
	return &StoreOptions{
		RejectCacheSize:  DefaultRejectCacheSize,
		ChannelCacheSize: DefaultChannelCacheSize,
		NoMigration:      false,
	}
}

// StoreOptionModifier is a function signature for modifying the default
// StoreOptions.
type StoreOptionModifier func(*StoreOptions)

// WithRejectCacheSize sets the RejectCacheSize to n.
func WithRejectCacheSize(n int) StoreOptionModifier {
	return func(o *StoreOptions) {
		o.RejectCacheSize = n
	}
}

// WithChannelCacheSize sets the ChannelCacheSize to n.
func WithChannelCacheSize(n int) StoreOptionModifier {
	return func(o *StoreOptions) {
		o.ChannelCacheSize = n
	}
}

// WithBatchCommitInterval sets the batch commit interval for the interval batch
// schedulers.
func WithBatchCommitInterval(interval time.Duration) StoreOptionModifier {
	return func(o *StoreOptions) {
		o.BatchCommitInterval = interval
	}
}
