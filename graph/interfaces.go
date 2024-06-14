package graph

import (
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/lnwire"
)

type Graph interface {
	// IsStaleEdgePolicy returns true if the graph source has a channel
	// edge for the passed channel ID (and flags) that have a more recent
	// timestamp.
	IsStaleEdgePolicy(chanID lnwire.ShortChannelID, timestamp time.Time,
		flags lnwire.ChanUpdateChanFlags) bool
}

type GraphDB interface {
	PruneTip() (*chainhash.Hash, uint32, error)

	PruneGraph(spentOutputs []*wire.OutPoint, blockHash *chainhash.Hash,
		blockHeight uint32) ([]*models.ChannelEdgeInfo, error)

	ChannelView() ([]channeldb.EdgePoint, error)

	PruneGraphNodes() error

	SourceNode() (*channeldb.LightningNode, error)

	DisabledChannelIDs() ([]uint64, error)

	FetchChanInfos(chanIDs []uint64) ([]channeldb.ChannelEdge, error)

	ChanUpdatesInHorizon(startTime, endTime time.Time) (
		[]channeldb.ChannelEdge, error)

	DeleteChannelEdges(strictZombiePruning, markZombie bool,
		chanIDs ...uint64) error

	DisconnectBlockAtHeight(height uint32) ([]*models.ChannelEdgeInfo,
		error)

	HasChannelEdge(chanID uint64) (time.Time, time.Time, bool, bool, error)
}
