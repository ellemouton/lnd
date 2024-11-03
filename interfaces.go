package lnd

import (
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/autopilot"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/graph/graphsession"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"github.com/lightningnetwork/lnd/netann"
)

// GraphSource defines the read-only graph interface required by LND for graph
// related tasks.
type GraphSource interface {
	graphsession.ReadOnlyGraph
	autopilot.GraphSource
	invoicesrpc.GraphSource
	netann.ChannelGraph
	channeldb.AddrSource

	// ForEachChannel iterates through all the channel edges stored within
	// the graph and invokes the passed callback for each edge. If the
	// callback returns an error, then the transaction is aborted and the
	// iteration stops early. An edge's policy structs may be nil if the
	// ChannelUpdate in question has not yet been received for the channel.
	ForEachChannel(cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error

	// HasLightningNode determines if the graph has a vertex identified by
	// the target node identity public key. If the node exists in the
	// database, a timestamp of when the data for the node was lasted
	// updated is returned along with a true boolean. Otherwise, an empty
	// time.Time is returned with a false boolean.
	HasLightningNode(nodePub [33]byte) (time.Time, bool, error)

	// NumZombies returns the current number of zombie channels in the
	// graph.
	NumZombies() (uint64, error)

	// LookupAlias attempts to return the alias as advertised by the target
	// node. graphdb.ErrNodeAliasNotFound is returned if the alias is not
	// found.
	LookupAlias(pub *btcec.PublicKey) (string, error)
}
