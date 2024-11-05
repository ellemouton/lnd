package main

import (
	"context"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnrpc/graphrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

type remoteWrapper struct {
	conn graphrpc.GraphClient

	local *graphdb.ChannelGraph
}

func (r *remoteWrapper) NewReadTx(ctx context.Context) (graphdb.RTx, error) {
	return r.local.NewReadTx(ctx)
}

func (r *remoteWrapper) ForEachNodeDirectedChannel(tx graphdb.RTx, node route.Vertex, cb func(channel *graphdb.DirectedChannel) error) error {
	return r.local.ForEachNodeDirectedChannel(tx, node, cb)
}

func (r *remoteWrapper) FetchNodeFeatures(tx graphdb.RTx, node route.Vertex) (*lnwire.FeatureVector, error) {
	return r.local.FetchNodeFeatures(tx, node)
}

func (r *remoteWrapper) ForEachNode(tx graphdb.RTx, cb func(graphdb.RTx, *models.LightningNode) error) error {
	return r.local.ForEachNode(tx, cb)
}

func (r *remoteWrapper) FetchLightningNode(tx graphdb.RTx, nodePub route.Vertex) (*models.LightningNode, error) {
	return r.local.FetchLightningNode(tx, nodePub)
}

func (r *remoteWrapper) ForEachNodeChannel(tx graphdb.RTx, nodePub route.Vertex, cb func(graphdb.RTx, *models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	return r.local.ForEachNodeChannel(tx, nodePub, cb)
}

func (r *remoteWrapper) ForEachNodeCached(cb func(node route.Vertex, chans map[uint64]*graphdb.DirectedChannel) error) error {
	return r.local.ForEachNodeCached(cb)
}

func (r *remoteWrapper) FetchChannelEdgesByID(chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	return r.local.FetchChannelEdgesByID(chanID)
}

func (r *remoteWrapper) IsPublicNode(pubKey [33]byte) (bool, error) {
	return r.local.IsPublicNode(pubKey)
}

func (r *remoteWrapper) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	return r.local.FetchChannelEdgesByOutpoint(point)
}

func (r *remoteWrapper) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr, error) {
	return r.local.AddrsForNode(nodePub)
}

func (r *remoteWrapper) ForEachChannel(cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	return r.local.ForEachChannel(cb)
}

func (r *remoteWrapper) HasLightningNode(ctx context.Context, nodePub [33]byte) (time.Time, bool, error) {
	return r.local.HasLightningNode(ctx, nodePub)
}

func (r *remoteWrapper) NumZombies(ctx context.Context) (uint64, error) {
	stats, err := r.conn.Stats(ctx, &graphrpc.StatsReq{})
	if err != nil {
		return 0, err
	}

	return stats.NumZombies, nil
}

func (r *remoteWrapper) LookupAlias(pub *btcec.PublicKey) (string, error) {
	return r.local.LookupAlias(pub)
}

var _ lnd.GraphSource = (*remoteWrapper)(nil)
