package graph

const (
	// DefaultPreAllocCacheNumNodes is the default number of channels we
	// assume for mainnet for pre-allocating the graph cache. As of
	// September 2021, there currently are 14k nodes in a strictly pruned
	// graph, so we choose a number that is slightly higher.
	DefaultPreAllocCacheNumNodes = 15000
)

// chanGraphOptions holds parameters for tuning and customizing the
// ChannelGraph.
type chanGraphOptions struct {
	// useGraphCache denotes whether the in-memory graph cache should be
	// used or a fallback version that uses the underlying database for
	// path finding.
	useGraphCache bool

	// preAllocCacheNumNodes is the number of nodes we expect to be in the
	// graph cache, so we can pre-allocate the map accordingly.
	preAllocCacheNumNodes int
}

// defaultChanGraphOptions returns a new chanGraphOptions instance populated
// with default values.
func defaultChanGraphOptions() *chanGraphOptions {
	return &chanGraphOptions{
		useGraphCache:         true,
		preAllocCacheNumNodes: DefaultPreAllocCacheNumNodes,
	}
}

// ChanGraphOption describes the signature of a functional option that can be
// used to customize a ChannelGraph instance.
type ChanGraphOption func(*chanGraphOptions)

// WithUseGraphCache sets whether the in-memory graph cache should be used.
func WithUseGraphCache(use bool) ChanGraphOption {
	return func(o *chanGraphOptions) {
		o.useGraphCache = use
	}
}

// WithPreAllocCacheNumNodes sets the number of nodes we expect to be in the
// graph cache, so we can pre-allocate the map accordingly.
func WithPreAllocCacheNumNodes(n int) ChanGraphOption {
	return func(o *chanGraphOptions) {
		o.preAllocCacheNumNodes = n
	}
}
