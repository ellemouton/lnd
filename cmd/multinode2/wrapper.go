package main

import (
	"context"
	"encoding/hex"
	"image/color"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
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

func (r *remoteWrapper) NetworkStats(ctx context.Context, excludeNodes map[route.Vertex]struct{}, excludeChannels map[uint64]struct{}) (*models.NetworkStats, error) {
	var (
		exNodes [][]byte
		exChans []uint64
	)

	for node := range excludeNodes {
		exNodes = append(exNodes, node[:])
	}

	for chanID := range excludeChannels {
		exChans = append(exChans, chanID)
	}

	info, err := r.lnConn.GetNetworkInfo(ctx, &lnrpc.NetworkInfoRequest{
		ExcludeNodes: exNodes,
		ExcludeChans: exChans,
	})
	if err != nil {
		return nil, err
	}

	return &models.NetworkStats{
		Diameter:             info.GraphDiameter,
		MaxChanOut:           info.MaxOutDegree,
		NumNodes:             info.NumNodes,
		NumChannels:          info.NumChannels,
		TotalNetworkCapacity: btcutil.Amount(info.TotalNetworkCapacity),
		MinChanSize:          btcutil.Amount(info.MinChannelSize),
		MaxChanSize:          btcutil.Amount(info.MaxChannelSize),
		MedianChanSize:       btcutil.Amount(info.MedianChannelSizeSat),
		NumZombies:           info.NumZombieChans,
	}, nil
}

// Pathfinding.
func (r *remoteWrapper) NewReadTx(ctx context.Context) (graphdb.RTx, error) {
	return r.local.NewReadTx(ctx)
}

// Pathfinding.
func (r *remoteWrapper) ForEachNodeDirectedChannel(ctx context.Context, tx graphdb.RTx, node route.Vertex, cb func(channel *graphdb.DirectedChannel) error) error {
	return r.local.ForEachNodeDirectedChannel(ctx, tx, node, cb)
}

// DescribeGraph & autopilot.
func (r *remoteWrapper) ForEachNode(tx graphdb.RTx, cb func(graphdb.RTx, *models.LightningNode) error) error {
	return r.local.ForEachNode(tx, cb)
}

// DescribeGraph.
func (r *remoteWrapper) ForEachChannel(cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	return r.local.ForEachChannel(cb)
}

// GetNodeInfo & autopilot.
func (r *remoteWrapper) ForEachNodeChannel(tx graphdb.RTx, nodePub route.Vertex, cb func(graphdb.RTx, *models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {
	return r.local.ForEachNodeChannel(tx, nodePub, cb)
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

	return unmarshalFeatures(resp.Node.Features), nil
}

func (r *remoteWrapper) FetchLightningNode(ctx context.Context, tx graphdb.RTx,
	nodePub route.Vertex) (*models.LightningNode, error) {

	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub[:]),
		IncludeChannels: false,
	})
	if err != nil {
		return nil, err
	}
	node := resp.Node

	pubKey, err := hex.DecodeString(node.PubKey)
	if err != nil {
		return nil, err
	}
	var pubKeyBytes [33]byte
	copy(pubKeyBytes[:], pubKey)

	extra, err := lnwire.CustomRecords(node.CustomRecords).Serialize()
	if err != nil {
		return nil, err
	}

	addrs, err := r.unmarshalAddrs(resp.Node.Addresses)
	if err != nil {
		return nil, err
	}

	return &models.LightningNode{
		PubKeyBytes:          pubKeyBytes,
		HaveNodeAnnouncement: resp.IsPublic,
		LastUpdate:           time.Unix(int64(node.LastUpdate), 0),
		Addresses:            addrs,
		Color:                color.RGBA{},
		Alias:                node.Alias,
		Features:             unmarshalFeatures(node.Features),
		ExtraOpaqueData:      extra,
	}, nil
}

func unmarshalFeatures(features map[uint32]*lnrpc.Feature) *lnwire.FeatureVector {
	featureBits := make([]lnwire.FeatureBit, 0, len(features))
	featureNames := make(map[lnwire.FeatureBit]string)
	for featureBit, feature := range features {
		featureBits = append(
			featureBits, lnwire.FeatureBit(featureBit),
		)

		featureNames[lnwire.FeatureBit(featureBit)] = feature.Name
	}

	return lnwire.NewFeatureVector(
		lnwire.NewRawFeatureVector(featureBits...), featureNames,
	)
}

func (r *remoteWrapper) FetchChannelEdgesByOutpoint(ctx context.Context, point *wire.OutPoint) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	info, err := r.lnConn.GetChanInfo(ctx, &lnrpc.ChanInfoRequest{
		ChanPoint: point.String(),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return unmarshalChannelInfo(info)
}

func (r *remoteWrapper) FetchChannelEdgesByID(ctx context.Context, chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	info, err := r.lnConn.GetChanInfo(ctx, &lnrpc.ChanInfoRequest{
		ChanId: chanID,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return unmarshalChannelInfo(info)
}

func unmarshalChannelInfo(info *lnrpc.ChannelEdge) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {
	chanPoint, err := wire.NewOutPointFromString(info.ChanPoint)
	if err != nil {
		return nil, nil, nil, err
	}

	var (
		node1Bytes [33]byte
		node2Bytes [33]byte
	)
	node1, err := hex.DecodeString(info.Node1Pub)
	if err != nil {
		return nil, nil, nil, err
	}
	copy(node1Bytes[:], node1)

	node2, err := hex.DecodeString(info.Node2Pub)
	if err != nil {
		return nil, nil, nil, err
	}
	copy(node2Bytes[:], node2)

	extra, err := lnwire.CustomRecords(info.CustomRecords).Serialize()
	if err != nil {
		return nil, nil, nil, err
	}

	edge := &models.ChannelEdgeInfo{
		ChannelID:       info.ChannelId,
		ChannelPoint:    *chanPoint,
		NodeKey1Bytes:   node1Bytes,
		NodeKey2Bytes:   node2Bytes,
		Capacity:        btcutil.Amount(info.Capacity),
		ExtraOpaqueData: extra,
	}

	var (
		policy1 *models.ChannelEdgePolicy
		policy2 *models.ChannelEdgePolicy
	)
	if info.Node1Policy != nil {
		policy1, err = unmarshalPolicy(info.Node1Policy, true)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if info.Node2Policy != nil {
		policy2, err = unmarshalPolicy(info.Node2Policy, false)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return edge, policy1, policy2, nil
}

func unmarshalPolicy(rpcPolicy *lnrpc.RoutingPolicy, node1 bool) (
	*models.ChannelEdgePolicy, error) {

	var chanFlags lnwire.ChanUpdateChanFlags
	if !node1 {
		chanFlags |= lnwire.ChanUpdateDirection
	}
	if rpcPolicy.Disabled {
		chanFlags |= lnwire.ChanUpdateDisabled
	}

	extra, err := lnwire.CustomRecords(rpcPolicy.CustomRecords).Serialize()
	if err != nil {
		return nil, err
	}

	return &models.ChannelEdgePolicy{
		TimeLockDelta:             uint16(rpcPolicy.TimeLockDelta),
		MinHTLC:                   lnwire.MilliSatoshi(rpcPolicy.MinHtlc),
		MaxHTLC:                   lnwire.MilliSatoshi(rpcPolicy.MaxHtlcMsat),
		FeeBaseMSat:               lnwire.MilliSatoshi(rpcPolicy.FeeBaseMsat),
		FeeProportionalMillionths: lnwire.MilliSatoshi(rpcPolicy.FeeRateMilliMsat),
		LastUpdate:                time.Unix(int64(rpcPolicy.LastUpdate), 0),
		ChannelFlags:              chanFlags,
		ExtraOpaqueData:           extra,
	}, nil
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

	addrs, err := r.unmarshalAddrs(resp.Node.Addresses)
	if err != nil {
		return false, nil, err
	}

	return true, addrs, nil
}

func (r *remoteWrapper) unmarshalAddrs(addrs []*lnrpc.NodeAddress) ([]net.Addr, error) {
	netAddrs := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		netAddr, err := r.net.ResolveTCPAddr(addr.Network, addr.Addr)
		if err != nil {
			return nil, err
		}
		netAddrs = append(netAddrs, netAddr)
	}

	return netAddrs, nil
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
