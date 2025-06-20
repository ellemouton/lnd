package graphdb

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/macaroons"
	"github.com/lightningnetwork/lnd/routing/route"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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
