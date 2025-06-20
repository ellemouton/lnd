package graphdb

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"image/color"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/lightningnetwork/lnd/routing/route"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"gopkg.in/macaroon.v2"
)

const DefaultRemoteGraphRPCTimeout = 5 * time.Second

// RemoteConfig holds the configuration options for the remote graph client.
//
//nolint:ll
type RemoteConfig struct {
	Enable       bool          `long:"enable" description:"Use an external RPC server as a remote graph source"`
	RPCHost      string        `long:"rpchost" description:"The remote graph's RPC host:port"`
	MacaroonPath string        `long:"macaroonpath" description:"The macaroon to use for authenticating with the remote graph source"`
	TLSCertPath  string        `long:"tlscertpath" description:"The TLS certificate to use for establishing the remote graph's identity"`
	Timeout      time.Duration `long:"timeout" description:"The timeout for connecting to and signing requests with the remote graph. Valid time units are {s, m, h}."`

	ParseAddressString func(strAddress string) (net.Addr, error)

	OnNewChannel    fn.Option[func(info *models.CachedEdgeInfo)]
	OnChannelUpdate fn.Option[func(policy *models.CachedEdgePolicy,
		fromNode, toNode route.Vertex, edge1 bool)]
	OnNodeUpsert fn.Option[func(node route.Vertex,
		features *lnwire.FeatureVector)]
	OnChanClose fn.Option[func(chanID uint64)]
}

// Validate checks the values configured for our remote RPC signer.
func (c *RemoteConfig) Validate() error {
	if !c.Enable {
		return nil
	}

	if c.Timeout < time.Millisecond {
		return fmt.Errorf("remote graph: timeout of %v is invalid, "+
			"cannot be smaller than %v", c.Timeout,
			time.Millisecond)
	}

	return nil
}

// RemoteClient implements a GraphSource by wrapping the connection to an
// external RPC server.
type RemoteClient struct {
	started atomic.Bool
	stopped atomic.Bool

	cfg *RemoteConfig

	featureBits map[string]lnwire.FeatureBit

	// conn is a grpc client that implements the LightningClient service.
	conn lnrpc.LightningClient

	wg     sync.WaitGroup
	cancel fn.Option[context.CancelFunc]
}

// NewRemoteClient constructs a new RemoteClient.
func NewRemoteClient(cfg *RemoteConfig) *RemoteClient {
	featureBits := make(map[string]lnwire.FeatureBit)
	for bit, name := range lnwire.Features {
		featureBits[name] = bit
	}

	return &RemoteClient{
		cfg:         cfg,
		featureBits: featureBits,
	}
}

// Start connects the client to the remote graph server and kicks off any
// goroutines that are required to handle updates from the remote graph.
func (c *RemoteClient) Start(ctx context.Context) error {
	if !c.started.CompareAndSwap(false, true) {
		return nil
	}

	log.Info("Graph RPC client starting")

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = fn.Some(cancel)

	conn, err := connectRPC(
		ctx, c.cfg.RPCHost, c.cfg.TLSCertPath, c.cfg.MacaroonPath,
		c.cfg.Timeout,
	)
	if err != nil {
		return err
	}
	c.conn = lnrpc.NewLightningClient(conn)

	c.wg.Add(1)
	go c.handleNetworkUpdates(ctx)

	return nil
}

// Stop cancels any goroutines along with any contexts created by the
// sub-system.
func (c *RemoteClient) Stop() error {
	if !c.stopped.CompareAndSwap(false, true) {
		return nil
	}

	log.Info("Graph RPC client stopping...")
	defer log.Info("Graph RPC client stopped")

	c.cancel.WhenSome(func(cancel context.CancelFunc) { cancel() })
	c.wg.Wait()

	return nil
}

// connectRPC tries to establish an RPC connection to the given host:port with
// the supplied certificate and macaroon.
func connectRPC(ctx context.Context, hostPort, tlsCertPath, macaroonPath string,
	timeout time.Duration) (*grpc.ClientConn, error) {

	certBytes, err := os.ReadFile(tlsCertPath)
	if err != nil {
		return nil, fmt.Errorf("error reading TLS cert file %v: %w",
			tlsCertPath, err)
	}

	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM(certBytes) {
		return nil, fmt.Errorf("credentials: failed to append " +
			"certificate")
	}

	macBytes, err := os.ReadFile(macaroonPath)
	if err != nil {
		return nil, fmt.Errorf("error reading macaroon file %v: %w",
			macaroonPath, err)
	}
	mac := &macaroon.Macaroon{}
	if err := mac.UnmarshalBinary(macBytes); err != nil {
		return nil, fmt.Errorf("error decoding macaroon: %w", err)
	}

	macCred, err := macaroons.NewMacaroonCredential(mac)
	if err != nil {
		return nil, fmt.Errorf("error creating creds: %w", err)
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(
			cp, "",
		)),
		grpc.WithPerRPCCredentials(macCred),
		grpc.WithBlock(),
	}

	ctxt, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctxt, hostPort, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to RPC server: %w",
			err)
	}

	return conn, nil
}

// handleNetworkUpdates subscribes to topology updates from the remote LND
// node. It then notifies any call-backs that have been registered with the
// client.
//
// NOTE: this MUST be run in a goroutine.
func (c *RemoteClient) handleNetworkUpdates(ctx context.Context) {
	defer c.wg.Done()

	// Subscribe to any topology updates from the remote graph.
	client, err := c.conn.SubscribeChannelGraph(
		ctx, &lnrpc.GraphTopologySubscription{},
	)
	if err != nil {
		return
	}

	for {
		// Check if we have exited.
		select {
		case <-ctx.Done():
			return
		default:
		}

		// TODO(elle): do this in a goroutine so that we can exit if
		//  needed? or does the canceling of the context already handle
		// that?
		updates, err := client.Recv()
		if err != nil {
			return
		}

		for _, update := range updates.ChannelUpdates {
			if err := c.handleChannelUpdate(update); err != nil {
				log.Errorf("Error handling channel update: %v",
					err)
			}
		}

		for _, update := range updates.NodeUpdates {
			if err := c.handleNodeUpdate(update); err != nil {
				log.Errorf("Error handling node update: %v",
					err)
			}
		}

		for _, update := range updates.ClosedChans {
			c.cfg.OnChanClose.WhenSome(func(f func(chanID uint64)) {
				f(update.ChanId)
			})
		}
	}
}

// handleNodeUpdate converts any received lnrpc.NodeUpdate to the type expected
// by the OnNodeUpsert call-back and then calls the call-back.
func (c *RemoteClient) handleNodeUpdate(update *lnrpc.NodeUpdate) error {
	// Exit early if no OnNodeUpsert call-back has been set.
	if c.cfg.OnNodeUpsert.IsNone() {
		return nil
	}

	pub, err := route.NewVertexFromStr(update.IdentityKey)
	if err != nil {
		return err
	}

	features := lnwire.NewRawFeatureVector()
	for _, feature := range update.Features {
		bit := c.featureBits[feature.Name]
		if !feature.IsRequired {
			bit ^= 1
		}

		features.Set(bit)
	}

	c.cfg.OnNodeUpsert.WhenSome(
		func(f func(route.Vertex, *lnwire.FeatureVector)) {
			fv := lnwire.NewFeatureVector(features, lnwire.Features)
			f(pub, fv)
		},
	)

	return nil
}

// handleChannelUpdate converts any received lnrpc.ChannelEdgeUpdate to the type
// expected by the OnNewChannel and OnChannelUpdate call-backs and then calls
// the call-backs.
func (c *RemoteClient) handleChannelUpdate(
	update *lnrpc.ChannelEdgeUpdate) error {

	fromNode, err := route.NewVertexFromStr(update.AdvertisingNode)
	if err != nil {
		return err
	}

	toNode, err := route.NewVertexFromStr(update.ConnectingNode)
	if err != nil {
		return err
	}

	// Lexicographically sort the pubkeys and determine if this update is
	// from node 1 or 2.
	var (
		edge1 bool
		node1 route.Vertex
		node2 route.Vertex
	)
	if bytes.Compare(fromNode[:], toNode[:]) == -1 {
		node1 = toNode
		node2 = fromNode
	} else {
		node1 = fromNode
		node2 = toNode
		edge1 = true
	}

	edge := &models.CachedEdgeInfo{
		ChannelID:     update.ChanId,
		Capacity:      btcutil.Amount(update.Capacity),
		NodeKey1Bytes: node1,
		NodeKey2Bytes: node2,
	}

	c.cfg.OnNewChannel.WhenSome(func(f func(info *models.CachedEdgeInfo)) {
		f(edge)
	})

	if update.RoutingPolicy == nil {
		return nil
	}

	policy := makePolicy(update.ChanId, update.RoutingPolicy, toNode, edge1)

	c.cfg.OnChannelUpdate.WhenSome(
		func(f func(policy *models.CachedEdgePolicy,
			fromNode route.Vertex, toNode route.Vertex,
			edge1 bool)) {

			f(policy, fromNode, toNode, edge1)
		},
	)

	return nil
}

func makePolicy(chanID uint64, rpcPolicy *lnrpc.RoutingPolicy,
	toNode [33]byte, node1 bool) *models.CachedEdgePolicy {

	policy := &models.CachedEdgePolicy{
		ChannelID:     chanID,
		TimeLockDelta: uint16(rpcPolicy.TimeLockDelta),
		MinHTLC:       lnwire.MilliSatoshi(rpcPolicy.MinHtlc),
		FeeBaseMSat:   lnwire.MilliSatoshi(rpcPolicy.FeeBaseMsat),
		FeeProportionalMillionths: lnwire.MilliSatoshi(
			rpcPolicy.FeeRateMilliMsat,
		),
		ToNodePubKey: func() route.Vertex {
			return toNode
		},
	}

	if rpcPolicy.MaxHtlcMsat > 0 {
		policy.MaxHTLC = lnwire.MilliSatoshi(rpcPolicy.MaxHtlcMsat)
		policy.MessageFlags |= lnwire.ChanUpdateRequiredMaxHtlc
	}

	if rpcPolicy.Disabled {
		policy.ChannelFlags |= lnwire.ChanUpdateDisabled
	}

	if !node1 {
		policy.ChannelFlags |= lnwire.ChanUpdateDirection
	}

	if rpcPolicy.InboundFeeRateMilliMsat != 0 &&
		rpcPolicy.InboundFeeBaseMsat != 0 {

		policy.InboundFee = fn.Some(lnwire.Fee{
			BaseFee: rpcPolicy.InboundFeeBaseMsat,
			FeeRate: rpcPolicy.InboundFeeRateMilliMsat,
		})
	}

	return policy
}

func (c *RemoteClient) ForEachChannelCacheable(
	cb func(*models.CachedEdgeInfo, *models.CachedEdgePolicy,
		*models.CachedEdgePolicy) error) error {

	ctx := context.TODO()

	graph, err := c.conn.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
		IncludeAuthProof:   true,
	})
	if err != nil {
		return err
	}

	for _, edge := range graph.Edges {
		edgeInfo, policy1, policy2, err := unmarshalChannelInfo(edge)
		if err != nil {
			return err
		}

		var p1, p2 *models.CachedEdgePolicy
		if policy1 != nil {
			p1 = models.NewCachedPolicy(policy1)
		}
		if policy2 != nil {
			p2 = models.NewCachedPolicy(policy2)
		}

		if err := cb(models.NewCachedEdge(edgeInfo), p1, p2); err != nil {
			return err
		}
	}

	return nil
}

func (c *RemoteClient) ForEachNodeCacheable(cb func(route.Vertex,
	*lnwire.FeatureVector) error) error {

	ctx := context.TODO()

	graph, err := c.conn.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{})
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
		var pubKeyBytes route.Vertex
		copy(pubKeyBytes[:], pubKey)

		err = cb(pubKeyBytes, unmarshalFeatures(node.Features))
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *RemoteClient) ForEachChannel(cb func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	ctx := context.TODO()

	graph, err := c.conn.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
		IncludeAuthProof:   true,
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

func (c *RemoteClient) ForEachNodeChannel(nodePub route.Vertex,
	cb func(*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy) error) error {

	ctx := context.TODO()

	info, err := c.conn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
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

// FetchLightningNode queries the remote node for information regarding the node
// in question. If the node isn't found in the database, then
// graphdb.ErrGraphNodeNotFound is returned.
func (c *RemoteClient) FetchLightningNode(nodePub route.Vertex) (
	*models.LightningNode, error) {

	ctx := context.TODO()
	resp, err := c.conn.GetNodeInfo(ctx, &lnrpc.NodeInfoRequest{
		PubKey:          hex.EncodeToString(nodePub[:]),
		IncludeChannels: false,
	})
	if err != nil {
		return nil, err
	}

	return c.unmarshalNode(resp.Node)
}

func (c *RemoteClient) ForEachNode(cb func(tx NodeRTx) error) error {
	ctx := context.TODO()

	graph, err := c.conn.DescribeGraph(ctx, &lnrpc.ChannelGraphRequest{
		IncludeUnannounced: true,
	})
	if err != nil {
		return err
	}

	// Iterate through all the nodes, convert the types to that expected
	// by the call-back and then apply the call-back.
	for _, node := range graph.Nodes {
		lnNode, err := c.unmarshalNode(node)
		if err != nil {
			return err
		}

		err = cb(&remoteNodeRTx{
			c:    c,
			node: lnNode,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

type remoteNodeRTx struct {
	c    *RemoteClient
	node *models.LightningNode
}

func (r *remoteNodeRTx) Node() *models.LightningNode {
	return r.node
}

func (r *remoteNodeRTx) ForEachChannel(f func(*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy, *models.ChannelEdgePolicy) error) error {

	return r.c.ForEachNodeChannel(r.node.PubKeyBytes,
		func(edge *models.ChannelEdgeInfo,
			policy1, policy2 *models.ChannelEdgePolicy) error {

			return f(edge, policy1, policy2)
		},
	)
}

func (r *remoteNodeRTx) FetchNode(node route.Vertex) (NodeRTx, error) {
	n, err := r.c.FetchLightningNode(node)
	if err != nil {
		return nil, err
	}

	return &remoteNodeRTx{
		c:    r.c,
		node: n,
	}, nil
}

func (c *RemoteClient) FetchChannelEdgesByID(chanID uint64) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	info, err := c.conn.GetChanInfo(context.TODO(), &lnrpc.ChanInfoRequest{
		ChanId: chanID,
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return nil, nil, nil, ErrEdgeNotFound
	} else if err != nil {
		return nil, nil, nil, err
	}

	return unmarshalChannelInfo(info)
}

func (c *RemoteClient) FetchChannelEdgesByOutpoint(point *wire.OutPoint) (
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	info, err := c.conn.GetChanInfo(context.TODO(), &lnrpc.ChanInfoRequest{
		ChanPoint: point.String(),
	})
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return nil, nil, nil, ErrEdgeNotFound
	} else if err != nil {
		return nil, nil, nil, err
	}

	return unmarshalChannelInfo(info)
}

var _ NodeRTx = (*remoteNodeRTx)(nil)

var _ GraphSource = (*RemoteClient)(nil)

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

func (c *RemoteClient) unmarshalNode(node *lnrpc.LightningNode) (
	*models.LightningNode, error) {

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

	addrs, err := c.unmarshalAddrs(node.Addresses)
	if err != nil {
		return nil, err
	}

	// If we have addresses for this node, or the node has an alias, or the
	// node has extra data or a non-empty feature vector, then we assume the
	// remote node has a node announcement for this node.
	haveNodeAnnouncement := len(addrs) > 0 || node.Alias != "" ||
		len(extra) > 0 || len(node.Features) > 0

	return &models.LightningNode{
		PubKeyBytes:          pubKeyBytes,
		HaveNodeAnnouncement: haveNodeAnnouncement,
		LastUpdate:           time.Unix(int64(node.LastUpdate), 0),
		Addresses:            addrs,
		Color:                color.RGBA{},
		Alias:                node.Alias,
		Features:             unmarshalFeatures(node.Features),
		ExtraOpaqueData:      extra,
	}, nil
}

func (c *RemoteClient) unmarshalAddrs(addrs []*lnrpc.NodeAddress) ([]net.Addr,
	error) {

	netAddrs := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		netAddr, err := c.cfg.ParseAddressString(addr.Addr)
		if err != nil {
			return nil, err
		}
		netAddrs = append(netAddrs, netAddr)
	}

	return netAddrs, nil
}

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
	if info.AuthProof != nil {
		edge.AuthProof = &models.ChannelAuthProof{
			NodeSig1Bytes:    info.AuthProof.NodeSig1,
			NodeSig2Bytes:    info.AuthProof.NodeSig2,
			BitcoinSig1Bytes: info.AuthProof.BitcoinSig1,
			BitcoinSig2Bytes: info.AuthProof.BitcoinSig2,
		}
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
