package main

import (
	"context"
	"encoding/hex"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/graphrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tor"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type remoteWrapper struct {
	graphConn graphrpc.GraphClient
	lnConn    lnrpc.LightningClient

	// export from LND Server struct.
	net tor.Net

	local *graphdb.ChannelGraph
}

func (r *remoteWrapper) NewReadTx(ctx context.Context) (graphdb.RTx, error) {
	return r.local.NewReadTx(ctx)
}

func (r *remoteWrapper) ForEachNodeDirectedChannel(ctx context.Context, tx graphdb.RTx, node route.Vertex, cb func(channel *graphdb.DirectedChannel) error) error {
	return r.local.ForEachNodeDirectedChannel(ctx, tx, node, cb)
}

func (r *remoteWrapper) FetchNodeFeatures(ctx context.Context, tx graphdb.RTx, node route.Vertex) (*lnwire.FeatureVector, error) {
	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(node[:]),
		IncludeChannels: false,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return lnwire.EmptyFeatureVector(), nil
	} else if err != nil {
		return nil, err
	}

	featureBits := make([]lnwire.FeatureBit, 0, len(resp.Node.Features))
	featureNames := make(map[lnwire.FeatureBit]string)

	for featureBit, feature := range resp.Node.Features {
		featureBits = append(
			featureBits, lnwire.FeatureBit(featureBit),
		)

		featureNames[lnwire.FeatureBit(featureBit)] = feature.Name
	}

	return lnwire.NewFeatureVector(
		lnwire.NewRawFeatureVector(featureBits...), featureNames,
	), nil
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

func (r *remoteWrapper) ForEachNodeCached(ctx context.Context,
	cb func(node route.Vertex,
		chans map[uint64]*graphdb.DirectedChannel) error) error {

	return r.local.ForEachNodeCached(ctx, cb)
}

func (r *remoteWrapper) FetchChannelEdgesByID(chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	return r.local.FetchChannelEdgesByID(chanID)
}

func (r *remoteWrapper) IsPublicNode(ctx context.Context, pubKey [33]byte) (bool, error) {
	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(pubKey[:]),
		IncludeChannels: false,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return resp.IsPublic, nil
}

func (r *remoteWrapper) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	return r.local.FetchChannelEdgesByOutpoint(point)
}

func (r *remoteWrapper) AddrsForNode(ctx context.Context, nodePub *btcec.PublicKey) (bool, []net.Addr, error) {
	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub.SerializeCompressed()),
		IncludeChannels: false,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return false, nil, nil
	} else if err != nil {
		return false, nil, err
	}

	addrs := make([]net.Addr, 0, len(resp.Node.Addresses))
	for _, addr := range resp.Node.Addresses {
		a, err := r.net.ResolveTCPAddr(addr.Network, addr.Addr)
		if err != nil {
			return false, nil, err
		}
		addrs = append(addrs, a)
	}

	return true, addrs, nil
}

func (r *remoteWrapper) ForEachChannel(cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	return r.local.ForEachChannel(cb)
}

func (r *remoteWrapper) HasLightningNode(ctx context.Context, nodePub [33]byte) (time.Time, bool, error) {
	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub[:]),
		IncludeChannels: false,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return time.Time{}, false, nil
	} else if err != nil {
		return time.Time{}, false, err
	}

	return time.Unix(int64(resp.Node.LastUpdate), 0), true, nil
}

func (r *remoteWrapper) NumZombies(ctx context.Context) (uint64, error) {
	stats, err := r.graphConn.Stats(ctx, &graphrpc.StatsReq{})
	if err != nil {
		return 0, err
	}

	return stats.NumZombies, nil
}

func (r *remoteWrapper) LookupAlias(ctx context.Context, pub *btcec.PublicKey) (
	string, error) {

	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(pub.SerializeCompressed()),
		IncludeChannels: false,
	})
	if err != nil {
		return "", err
	}

	return resp.Node.Alias, nil
}

var _ lnd.GraphSource = (*remoteWrapper)(nil)
