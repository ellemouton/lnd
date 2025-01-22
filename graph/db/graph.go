package graphdb

type ChannelGraph struct {
	db DB
}

func NewChannelGraph(db DB) *ChannelGraph {
	return &ChannelGraph{db}
}

func (c *ChannelGraph) NewRoutingGraphSession() (RoutingGraph, func() error,
	error) {

	return c.db.NewRoutingGraphSession()
}

func (c *ChannelGraph) NewRoutingGraph() RoutingGraph {
	return c.db.NewRoutingGraph()
}

// A compile time assertion to ensure ChannelGraph implements the GraphReads
// interface.
var _ GraphReads = (*ChannelGraph)(nil)
