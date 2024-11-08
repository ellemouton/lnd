package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"image/color"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd"
	"github.com/lightningnetwork/lnd/autopilot"
	"github.com/lightningnetwork/lnd/discovery"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/graph/stats"
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

func (r *remoteWrapper) BetweenessCentrality(ctx context.Context) (map[autopilot.NodeID]*stats.BetweenessCentrality, error) {
	resp, err := r.graphConn.BetweennessCentrality(ctx, &graphrpc.BetweennessCentralityReq{})
	if err != nil {
		return nil, err
	}

	centrality := make(map[autopilot.NodeID]*stats.BetweenessCentrality)
	for _, node := range resp.NodeBetweeness {
		var id autopilot.NodeID
		copy(id[:], node.Node)
		centrality[id] = &stats.BetweenessCentrality{
			Normalized:    float64(node.Normalized),
			NonNormalized: float64(node.NonNormalized),
		}
	}

	return centrality, nil
}

func (r *remoteWrapper) GraphBootstrapper(ctx context.Context) (discovery.NetworkPeerBootstrapper, error) {
	resp, err := r.graphConn.BoostrapperName(ctx, &graphrpc.BoostrapperNameReq{})
	if err != nil {
		return nil, err
	}

	return &bootstrapper{
		name:          resp.Name,
		remoteWrapper: r,
	}, nil
}

type bootstrapper struct {
	name string
	*remoteWrapper
}

func (r *bootstrapper) SampleNodeAddrs(numAddrs uint32,
	ignore map[autopilot.NodeID]struct{}) ([]*lnwire.NetAddress, error) {

	var toIgnore [][]byte
	for id := range ignore {
		toIgnore = append(toIgnore, id[:])
	}

	resp, err := r.graphConn.BootstrapAddrs(
		context.Background(), &graphrpc.BootstrapAddrsReq{
			NumAddrs:    numAddrs,
			IgnoreNodes: toIgnore,
		},
	)
	if err != nil {
		return nil, err
	}

	netAddrs := make([]*lnwire.NetAddress, 0, len(resp.Addrs))
	for _, addr := range resp.Addrs {
		netAddr, err := r.net.ResolveTCPAddr(addr.Address.Network, addr.Address.Addr)
		if err != nil {
			return nil, err
		}
		idKey, err := btcec.ParsePubKey(addr.NodeId)
		if err != nil {
			return nil, err
		}
		netAddrs = append(netAddrs, &lnwire.NetAddress{
			IdentityKey: idKey,
			Address:     netAddr,
		})
	}

	return netAddrs, nil

}

func (r *bootstrapper) Name() string {
	return r.name
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
func (r *remoteWrapper) NewPathFindTx(ctx context.Context) (graphdb.RTx, error) {
	// TODO(elle): ??
	return graphdb.NewKVDBRTx(nil), nil
}

// Pathfinding.
func (r *remoteWrapper) ForEachNodeDirectedChannel(ctx context.Context,
	_ graphdb.RTx, node route.Vertex,
	cb func(channel *graphdb.DirectedChannel) error) error {

	info, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(node[:]),
		IncludeChannels: true,
	})
	if err != nil {
		return err
	}

	toNodeCallback := func() route.Vertex {
		return node
	}
	toNodeFeatures := unmarshalFeatures(info.Node.Features)

	for _, channel := range info.Channels {
		e, p1, p2, err := unmarshalChannelInfo(channel)
		if err != nil {
			return err
		}

		var cachedInPolicy *models.CachedEdgePolicy
		if p2 != nil {
			cachedInPolicy = models.NewCachedPolicy(p2)
			cachedInPolicy.ToNodePubKey = toNodeCallback
			cachedInPolicy.ToNodeFeatures = toNodeFeatures
		}

		var inboundFee lnwire.Fee
		if p1 != nil {
			// Extract inbound fee. If there is a decoding error,
			// skip this edge.
			_, err := p1.ExtraOpaqueData.ExtractRecords(&inboundFee)
			if err != nil {
				return nil
			}
		}

		directedChannel := &graphdb.DirectedChannel{
			ChannelID:    e.ChannelID,
			IsNode1:      node == e.NodeKey1Bytes,
			OtherNode:    e.NodeKey2Bytes,
			Capacity:     e.Capacity,
			OutPolicySet: p1 != nil,
			InPolicy:     cachedInPolicy,
			InboundFee:   inboundFee,
		}

		if node == e.NodeKey2Bytes {
			directedChannel.OtherNode = e.NodeKey1Bytes
		}

		if err := cb(directedChannel); err != nil {
			return err
		}
	}

	return nil
}

// DescribeGraph. NB: use --caches.rpc-graph-cache-duration
func (r *remoteWrapper) ForEachNode(ctx context.Context,
	cb func(*models.LightningNode) error) error {

	graph, err := r.lnConn.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
	})
	if err != nil {
		return err
	}

	selfNode, err := r.local.SourceNode()
	if err != nil {
		return err
	}

	for _, node := range graph.Nodes {
		pubKey, err := hex.DecodeString(node.PubKey)
		if err != nil {
			return err
		}
		var pubKeyBytes [33]byte
		copy(pubKeyBytes[:], pubKey)

		extra, err := lnwire.CustomRecords(node.CustomRecords).Serialize()
		if err != nil {
			return err
		}

		addrs, err := r.unmarshalAddrs(node.Addresses)
		if err != nil {
			return err
		}

		var haveNodeAnnouncement bool
		if bytes.Equal(selfNode.PubKeyBytes[:], pubKeyBytes[:]) {
			haveNodeAnnouncement = true
		} else {
			haveNodeAnnouncement = len(addrs) > 0 ||
				node.Alias != "" || len(extra) > 0 ||
				len(node.Features) > 0
		}

		n := &models.LightningNode{
			PubKeyBytes:          pubKeyBytes,
			HaveNodeAnnouncement: haveNodeAnnouncement,
			LastUpdate:           time.Unix(int64(node.LastUpdate), 0),
			Addresses:            addrs,
			Color:                color.RGBA{},
			Alias:                node.Alias,
			Features:             unmarshalFeatures(node.Features),
			ExtraOpaqueData:      extra,
		}

		err = cb(n)
		if err != nil {
			return err
		}
	}

	return nil
}

// DescribeGraph. NB: use --caches.rpc-graph-cache-duration
func (r *remoteWrapper) ForEachChannel(ctx context.Context,
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	graph, err := r.lnConn.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
	})
	if err != nil {
		return err
	}

	for _, edge := range graph.Edges {
		edgeInfo, policy1, policy2, err := unmarshalChannelInfo(edge)
		if err != nil {
			return err
		}

		// To ensure that Describe graph doesnt filter it out.
		edgeInfo.AuthProof = &models.ChannelAuthProof{}

		if err := cb(edgeInfo, policy1, policy2); err != nil {
			return err
		}
	}

	return nil
}

// GetNodeInfo.
func (r *remoteWrapper) ForEachNodeChannel(ctx context.Context,
	nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	info, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub[:]),
		IncludeChannels: true,
	})
	if err != nil {
		return err
	}

	for _, channel := range info.Channels {
		edge, policy1, policy2, err := unmarshalChannelInfo(channel)
		if err != nil {
			return err
		}

		if err := cb(edge, policy1, policy2); err != nil {
			return err
		}
	}

	return nil
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

func (r *remoteWrapper) FetchLightningNode(ctx context.Context,
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
		policy1, err = unmarshalPolicy(info.ChannelId, info.Node1Policy, true)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if info.Node2Policy != nil {
		policy2, err = unmarshalPolicy(info.ChannelId, info.Node2Policy, false)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return edge, policy1, policy2, nil
}

func unmarshalPolicy(channelID uint64, rpcPolicy *lnrpc.RoutingPolicy,
	node1 bool) (*models.ChannelEdgePolicy, error) {

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
		ChannelID:                 channelID,
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
