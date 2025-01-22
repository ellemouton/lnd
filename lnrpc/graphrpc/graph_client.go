package graphrpc

import (
	"context"
	"encoding/hex"
	"image/color"
	"net"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	graphdb "github.com/lightningnetwork/lnd/graph/db"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lncfg"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client is a wrapper that implements the sources.GraphSource interface using a
// grpc connection which it uses to communicate with a grpc GraphClient client
// along with an lnrpc.LightningClient.
type Client struct {
	// lnConn is a grpc client that implements the LightningClient service.
	lnConn lnrpc.LightningClient

	resolveTCPAddr func(network, address string) (*net.TCPAddr, error)
}

// NewRemoteClient constructs a new Client that uses the given grpc connections
// to implement the sources.GraphSource interface.
func NewRemoteClient(conn *grpc.ClientConn,
	resolveTCPAddr func(network, address string) (*net.TCPAddr,
		error)) *Client {

	return &Client{
		lnConn: lnrpc.NewLightningClient(conn),
	}
}

// AddrsForNode queries the remote node for all the addresses it knows about for
// the node in question. The returned boolean indicates if the given node is
// unknown to the remote node.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) AddrsForNode(nodePub *btcec.PublicKey) (bool, []net.Addr, error) {

	resp, err := r.lnConn.GetNodeInfo(context.TODO(), &lnrpc.NodeInfoRequest{
		PubKey: hex.EncodeToString(
			nodePub.SerializeCompressed(),
		),
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

func (r *Client) unmarshalAddrs(addrs []*lnrpc.NodeAddress) ([]net.Addr,
	error) {

	netAddrs := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		netAddr, err := lncfg.ParseAddressString(
			addr.Addr, strconv.Itoa(9735), // TODO: move DefaultPeerPort to lncfg and import
			r.resolveTCPAddr,
		)
		if err != nil {
			return nil, err
		}
		netAddrs = append(netAddrs, netAddr)
	}

	return netAddrs, nil
}

// ForEachChannel calls DescribeGraph on the remote node to fetch all the
// channels known to it. It then invokes the passed callback for each edge.
// An edge's policy structs may be nil if the ChannelUpdate in question has not
// yet been received for the channel. No error is returned if no channels are
// found.
//
// NOTE: this is an expensive call as it fetches all nodes from the network and
// so it is recommended that caching be used by both the client and the server
// where appropriate to reduce the number of calls to this method.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) ForEachChannel(
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	graph, err := r.lnConn.DescribeGraph(context.TODO(), &lnrpc.ChannelGraphRequest{
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

		if err := cb(edgeInfo, policy1, policy2); err != nil {
			return err
		}
	}

	return nil
}

// FetchChannelEdgesByOutpoint queries the remote node for the channel edge info
// and most recent channel edge policies for a given outpoint. If the channel
// can't be found, then graphdb.ErrEdgeNotFound is returned.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

	info, err := r.lnConn.GetChanInfo(context.TODO(), &lnrpc.ChanInfoRequest{
		ChanPoint: point.String(),
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return nil, nil, nil, graphdb.ErrEdgeNotFound
	} else if err != nil {
		return nil, nil, nil, err
	}

	return unmarshalChannelInfo(info)
}

// HasLightningNode determines if the graph has a vertex identified by the
// target node identity public key. If the node exists in the database, a
// timestamp of when the data for the node was lasted updated is returned along
// with a true boolean. Otherwise, an empty time.Time is returned with a false
// boolean and a nil error.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) HasLightningNode(nodePub [33]byte) (
	time.Time, bool, error) {

	resp, err := r.lnConn.GetNodeInfo(context.TODO(), &lnrpc.NodeInfoRequest{
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

// FetchLightningNode queries the remote node for information regarding the node
// in question. If the node isn't found in the database, then
// graphdb.ErrGraphNodeNotFound is returned.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) FetchLightningNode(
	nodePub route.Vertex) (*models.LightningNode, error) {

	resp, err := r.lnConn.GetNodeInfo(context.TODO(), &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub[:]),
		IncludeChannels: false,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return nil, graphdb.ErrGraphNodeNotFound
	} else if err != nil {
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

// ForEachNode calls DescribeGraph on the remote node and applies the call back
// to each node in the response. No error is returned if no nodes are found.
//
// NOTE: this is an expensive call as it fetches all nodes from the network and
// so it is recommended that caching be used by both the client and the server
// where appropriate to reduce the number of calls to this method.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) ForEachNode(
	cb func(*models.LightningNode) error) error {

	graph, err := r.lnConn.DescribeGraph(context.TODO(), &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
	})
	if err != nil {
		return err
	}

	// Iterate through all the nodes, convert the types to that expected
	// by the call-back and then apply the call-back.
	for _, node := range graph.Nodes {
		pubKey, err := hex.DecodeString(node.PubKey)
		if err != nil {
			return err
		}
		var pubKeyBytes [33]byte
		copy(pubKeyBytes[:], pubKey)

		extra, err := lnwire.CustomRecords(
			node.CustomRecords,
		).Serialize()
		if err != nil {
			return err
		}

		addrs, err := r.unmarshalAddrs(node.Addresses)
		if err != nil {
			return err
		}

		// If we have addresses for this node, or the node has an alias,
		// or the node has extra data or a non-empty feature vector,
		// then we assume the remote node has a node announcement for
		// this node.
		haveNodeAnnouncement := len(addrs) > 0 || node.Alias != "" ||
			len(extra) > 0 || len(node.Features) > 0

		n := &models.LightningNode{
			PubKeyBytes:          pubKeyBytes,
			HaveNodeAnnouncement: haveNodeAnnouncement,
			LastUpdate: time.Unix(
				int64(node.LastUpdate), 0,
			),
			Addresses:       addrs,
			Color:           color.RGBA{},
			Alias:           node.Alias,
			Features:        unmarshalFeatures(node.Features),
			ExtraOpaqueData: extra,
		}

		err = cb(n)
		if err != nil {
			return err
		}
	}

	return nil
}

// ForEachNodeChannel fetches all the channels for the given node from the
// remote node and applies the call back to each channel and its policies.
// The first edge policy is the outgoing edge *to* the connecting node, while
// the second is the incoming edge *from* the connecting node. Unknown policies
// are passed into the callback as nil values. No error is returned if the node
// is not found.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) ForEachNodeChannel(ctx context.Context,
	nodePub route.Vertex, cb func(*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	info, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub[:]),
		IncludeChannels: true,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return nil
	} else if err != nil {
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

// FetchNodeFeatures queries the remote node for the feature vector of the node
// in question. If no features are known for the node or the node itself is
// unknown, an empty feature vector is returned.
//
// NOTE: The read transaction passed in is ignored.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) FetchNodeFeatures(ctx context.Context, node route.Vertex) (
	*lnwire.FeatureVector, error) {

	// Query the remote node for information about the node in question.
	// There is no need to include channel information since we only care
	// about features.
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

// IsPublicNode queries the remote node for information regarding the node in
// question and returns whether the node is seen as public by the remote node.
// If this node is unknown, then graphdb.ErrGraphNodeNotFound is returned.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) IsPublicNode(ctx context.Context, pubKey [33]byte) (bool,
	error) {

	resp, err := r.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(pubKey[:]),
		IncludeChannels: false,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return false, graphdb.ErrGraphNodeNotFound
	} else if err != nil {
		return false, err
	}

	return resp.IsPublic, nil
}

// FetchChannelEdgesByID queries the remote node for the channel edge info and
// most recent channel edge policies for a given channel ID. If the channel
// can't be found, then graphdb.ErrEdgeNotFound is returned.
//
// NOTE: this is part of the sources.GraphSource interface.
func (r *Client) FetchChannelEdgesByID(ctx context.Context,
	chanID uint64) (*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	info, err := r.lnConn.GetChanInfo(ctx, &lnrpc.ChanInfoRequest{
		ChanId: chanID,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return unmarshalChannelInfo(info)
}

func (r Client) NumZombies() (uint64, error) {
	info, err := r.lnConn.GetNetworkInfo(context.TODO(), &lnrpc.NetworkInfoRequest{})
	if err != nil {
		return 0, err
	}

	return info.NumZombieChans, nil
}

func (r *Client) ForEachNodeCached(cb func(node route.Vertex, chans map[uint64]*models.DirectedChannel) error) error {
	return r.ForEachNode(func(node *models.LightningNode) error {
		channels := make(map[uint64]*models.DirectedChannel)
		err := r.ForEachNodeChannel(
			context.TODO(), node.PubKeyBytes, func(e *models.ChannelEdgeInfo, p1 *models.ChannelEdgePolicy, p2 *models.ChannelEdgePolicy) error {
				toNodeCallback := func() route.Vertex {
					return node.PubKeyBytes
				}
				toNodeFeatures := node.Features

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

				directedChannel := &models.DirectedChannel{
					ChannelID:    e.ChannelID,
					IsNode1:      node.PubKeyBytes == e.NodeKey1Bytes,
					OtherNode:    e.NodeKey2Bytes,
					Capacity:     e.Capacity,
					OutPolicySet: p1 != nil,
					InPolicy:     cachedInPolicy,
					InboundFee:   inboundFee,
				}

				if node.PubKeyBytes == e.NodeKey2Bytes {
					directedChannel.OtherNode = e.NodeKey1Bytes
				}

				channels[e.ChannelID] = directedChannel

				return nil
			},
		)
		if err != nil {
			return err
		}

		return cb(node.PubKeyBytes, channels)
	})
}

func (r *Client) NewRoutingGraphSession() (graphdb.RoutingGraph, func() error, error) {
	return &clientSession{r}, func() error {
		return nil
	}, nil
}

type clientSession struct {
	*Client
}

func (c *clientSession) ForEachNodeChannel(ctx context.Context, node route.Vertex,
	cb func(channel *models.DirectedChannel) error) error {

	// Obtain the node info from the remote node and ask it to include
	// channel information.
	info, err := c.lnConn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(node[:]),
		IncludeChannels: true,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return nil
	} else if err != nil {
		return err
	}

	toNodeCallback := func() route.Vertex {
		return node
	}
	toNodeFeatures := unmarshalFeatures(info.Node.Features)

	// Convert each channel to the type expected by the call-back and then
	// apply the call-back.
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

		directedChannel := &models.DirectedChannel{
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

func (r *Client) NewRoutingGraph() graphdb.RoutingGraph {
	return &clientSession{r}
}

func (r *Client) ForEachNodeWithTx(ctx context.Context, cb func(graphdb.NodeTx) error) error {
	return r.ForEachNode(func(node *models.LightningNode) error {
		return cb(&rpcNodeTx{
			r:    r,
			node: node,
		})
	})
}

type rpcNodeTx struct {
	r    *Client
	node *models.LightningNode
}

func (n *rpcNodeTx) Node() *models.LightningNode {
	return n.node
}

func (n *rpcNodeTx) ForEachNodeChannel(ctx context.Context,
	nodePub route.Vertex, cb func(context.Context,
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	return n.r.ForEachNodeChannel(ctx, nodePub, func(
		info *models.ChannelEdgeInfo, policy *models.ChannelEdgePolicy,
		policy2 *models.ChannelEdgePolicy) error {

		return cb(ctx, info, policy, policy2)
	})
}

func (n *rpcNodeTx) FetchLightningNode(_ context.Context,
	nodePub route.Vertex) (graphdb.NodeTx, error) {

	node, err := n.r.FetchLightningNode(nodePub)
	if err != nil {
		return nil, err
	}

	return &rpcNodeTx{
		r:    n.r,
		node: node,
	}, nil
}

var _ graphdb.Source = (*Client)(nil)

func unmarshalChannelInfo(info *lnrpc.ChannelEdge) (*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error) {

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

	// To ensure that Describe graph doesn't filter it out as a private
	// channel, we set the auth proof to a non-nil value if the channel
	// has been announced.
	if info.Announced {
		edge.AuthProof = &models.ChannelAuthProof{}
	}

	var (
		policy1 *models.ChannelEdgePolicy
		policy2 *models.ChannelEdgePolicy
	)
	if info.Node1Policy != nil {
		policy1, err = unmarshalPolicy(
			info.ChannelId, info.Node1Policy, true, info.Node2Pub,
		)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if info.Node2Policy != nil {
		policy2, err = unmarshalPolicy(
			info.ChannelId, info.Node2Policy, false, info.Node1Pub,
		)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return edge, policy1, policy2, nil
}

func unmarshalFeatures(
	features map[uint32]*lnrpc.Feature) *lnwire.FeatureVector {

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

func unmarshalPolicy(channelID uint64, rpcPolicy *lnrpc.RoutingPolicy,
	node1 bool, toNodeStr string) (*models.ChannelEdgePolicy, error) {

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

	toNodeB, err := hex.DecodeString(toNodeStr)
	if err != nil {
		return nil, err
	}
	toNode, err := route.NewVertexFromBytes(toNodeB)
	if err != nil {
		return nil, err
	}

	return &models.ChannelEdgePolicy{
		ChannelID:     channelID,
		TimeLockDelta: uint16(rpcPolicy.TimeLockDelta),
		MinHTLC: lnwire.MilliSatoshi(
			rpcPolicy.MinHtlc,
		),
		MaxHTLC: lnwire.MilliSatoshi(
			rpcPolicy.MaxHtlcMsat,
		),
		FeeBaseMSat: lnwire.MilliSatoshi(
			rpcPolicy.FeeBaseMsat,
		),
		FeeProportionalMillionths: lnwire.MilliSatoshi(
			rpcPolicy.FeeRateMilliMsat,
		),
		LastUpdate:      time.Unix(int64(rpcPolicy.LastUpdate), 0),
		ChannelFlags:    chanFlags,
		ToNode:          toNode,
		ExtraOpaqueData: extra,
	}, nil
}
