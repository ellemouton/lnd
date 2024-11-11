package sources

import graphdb "github.com/lightningnetwork/lnd/graph/db"

// ChanGraphSource is an implementation of the lnd.GraphSource interface which
// can be used to make read-only graph queries.
type ChanGraphSource struct {
	db *graphdb.ChannelGraph
}

// NewChanGraphSource returns a new instance of the ChanGraphSource backed by
// a graphdb.ChannelGraph instance.
func NewChanGraphSource(db *graphdb.ChannelGraph) *ChanGraphSource {
	return &ChanGraphSource{
		db: db,
	}
}
