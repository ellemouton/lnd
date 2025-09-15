package graph

const (
	// DefaultPreAllocCacheNumNodes is the default number of channels we
	// assume for mainnet for pre-allocating the Graph cache. As of
	// September 2021, there currently are 14k nodes in a strictly pruned
	// Graph, so we choose a number that is slightly higher.
	DefaultPreAllocCacheNumNodes = 15000
)

// builderOptions holds parameters for tuning and customizing the Builder.
type builderOptions struct {
	// useGraphCache denotes whether the in-memory Graph cache should be
	// used or a fallback version that uses the underlying database for
	// path finding.
	useGraphCache bool

	// preAllocCacheNumNodes is the number of nodes we expect to be in the
	// Graph cache, so we can pre-allocate the map accordingly.
	preAllocCacheNumNodes int
}

// defaultBuilderOptions returns a new builderOptions instance populated with
// default values.
func defaultBuilderOptions() *builderOptions {
	return &builderOptions{
		useGraphCache:         true,
		preAllocCacheNumNodes: DefaultPreAllocCacheNumNodes,
	}
}

// BuilderOption describes the signature of a functional option that can be
// used to customize a ChannelGraph instance.
type BuilderOption func(*builderOptions)

// WithUseGraphCache sets whether the in-memory Graph cache should be used.
func WithUseGraphCache(use bool) BuilderOption {
	return func(o *builderOptions) {
		o.useGraphCache = use
	}
}

// WithPreAllocCacheNumNodes sets the number of nodes we expect to be in the
// Graph cache, so we can pre-allocate the map accordingly.
func WithPreAllocCacheNumNodes(n int) BuilderOption {
	return func(o *builderOptions) {
		o.preAllocCacheNumNodes = n
	}
}
