package routing

import (
	"bytes"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-errors/errors"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/amp"
	"github.com/lightningnetwork/lnd/batch"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/clock"
	"github.com/lightningnetwork/lnd/fn"
	"github.com/lightningnetwork/lnd/graph"
	"github.com/lightningnetwork/lnd/htlcswitch"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwallet"
	"github.com/lightningnetwork/lnd/lnwallet/btcwallet"
	"github.com/lightningnetwork/lnd/lnwallet/chanvalidate"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/multimutex"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/chainview"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/routing/shards"
	"github.com/lightningnetwork/lnd/ticker"
	"github.com/lightningnetwork/lnd/zpay32"
)

const (
	// DefaultPayAttemptTimeout is the default payment attempt timeout. The
	// payment attempt timeout defines the duration after which we stop
	// trying more routes for a payment.
	DefaultPayAttemptTimeout = time.Duration(time.Second * 60)

	// defaultStatInterval governs how often the router will log non-empty
	// stats related to processing new channels, updates, or node
	// announcements.
	defaultStatInterval = time.Minute

	// MinCLTVDelta is the minimum CLTV value accepted by LND for all
	// timelock deltas. This includes both forwarding CLTV deltas set on
	// channel updates, as well as final CLTV deltas used to create BOLT 11
	// payment requests.
	//
	// NOTE: For payment requests, BOLT 11 stipulates that a final CLTV
	// delta of 9 should be used when no value is decoded. This however
	// leads to inflexibility in upgrading this default parameter, since it
	// can create inconsistencies around the assumed value between sender
	// and receiver. Specifically, if the receiver assumes a higher value
	// than the sender, the receiver will always see the received HTLCs as
	// invalid due to their timelock not meeting the required delta.
	//
	// We skirt this by always setting an explicit CLTV delta when creating
	// invoices. This allows LND nodes to freely update the minimum without
	// creating incompatibilities during the upgrade process. For some time
	// LND has used an explicit default final CLTV delta of 40 blocks for
	// bitcoin, though we now clamp the lower end of this
	// range for user-chosen deltas to 18 blocks to be conservative.
	MinCLTVDelta = 18

	// MaxCLTVDelta is the maximum CLTV value accepted by LND for all
	// timelock deltas.
	MaxCLTVDelta = math.MaxUint16
)

var (
	// ErrRouterShuttingDown is returned if the router is in the process of
	// shutting down.
	ErrRouterShuttingDown = fmt.Errorf("router shutting down")

	// ErrSelfIntro is a failure returned when the source node of a
	// route request is also the introduction node. This is not yet
	// supported because LND does not support blinded forwardingg.
	ErrSelfIntro = errors.New("introduction point as own node not " +
		"supported")

	// ErrHintsAndBlinded is returned if a route request has both
	// bolt 11 route hints and a blinded path set.
	ErrHintsAndBlinded = errors.New("bolt 11 route hints and blinded " +
		"paths are mutually exclusive")

	// ErrExpiryAndBlinded is returned if a final cltv and a blinded path
	// are provided, as the cltv should be provided within the blinded
	// path.
	ErrExpiryAndBlinded = errors.New("final cltv delta and blinded " +
		"paths are mutually exclusive")

	// ErrTargetAndBlinded is returned is a target destination and a
	// blinded path are both set (as the target is inferred from the
	// blinded path).
	ErrTargetAndBlinded = errors.New("target node and blinded paths " +
		"are mutually exclusive")

	// ErrNoTarget is returned when the target node for a route is not
	// provided by either a blinded route or a cleartext pubkey.
	ErrNoTarget = errors.New("destination not set in target or blinded " +
		"path")

	// ErrSkipTempErr is returned when a non-MPP is made yet the
	// skipTempErr flag is set.
	ErrSkipTempErr = errors.New("cannot skip temp error for non-MPP")
)

// ChannelGraphSource represents the source of information about the topology
// of the lightning network. It's responsible for the addition of nodes, edges,
// applying edge updates, and returning the current block height with which the
// topology is synchronized.
type ChannelGraphSource interface {
	// AddNode is used to add information about a node to the router
	// database. If the node with this pubkey is not present in an existing
	// channel, it will be ignored.
	AddNode(node *channeldb.LightningNode,
		op ...batch.SchedulerOption) error

	// AddEdge is used to add edge/channel to the topology of the router,
	// after all information about channel will be gathered this
	// edge/channel might be used in construction of payment path.
	AddEdge(edge *models.ChannelEdgeInfo,
		op ...batch.SchedulerOption) error

	// AddProof updates the channel edge info with proof which is needed to
	// properly announce the edge to the rest of the network.
	AddProof(chanID lnwire.ShortChannelID,
		proof *models.ChannelAuthProof) error

	// UpdateEdge is used to update edge information, without this message
	// edge considered as not fully constructed.
	UpdateEdge(policy *models.ChannelEdgePolicy,
		op ...batch.SchedulerOption) error

	// IsStaleNode returns true if the graph source has a node announcement
	// for the target node with a more recent timestamp. This method will
	// also return true if we don't have an active channel announcement for
	// the target node.
	IsStaleNode(node route.Vertex, timestamp time.Time) bool

	// IsPublicNode determines whether the given vertex is seen as a public
	// node in the graph from the graph's source node's point of view.
	IsPublicNode(node route.Vertex) (bool, error)

	// IsKnownEdge returns true if the graph source already knows of the
	// passed channel ID either as a live or zombie edge.
	IsKnownEdge(chanID lnwire.ShortChannelID) bool

	// MarkEdgeLive clears an edge from our zombie index, deeming it as
	// live.
	MarkEdgeLive(chanID lnwire.ShortChannelID) error

	// ForAllOutgoingChannels is used to iterate over all channels
	// emanating from the "source" node which is the center of the
	// star-graph.
	ForAllOutgoingChannels(cb func(tx kvdb.RTx,
		c *models.ChannelEdgeInfo,
		e *models.ChannelEdgePolicy) error) error

	// CurrentBlockHeight returns the block height from POV of the router
	// subsystem.
	CurrentBlockHeight() (uint32, error)

	// GetChannelByID return the channel by the channel id.
	GetChannelByID(chanID lnwire.ShortChannelID) (
		*models.ChannelEdgeInfo, *models.ChannelEdgePolicy,
		*models.ChannelEdgePolicy, error)

	// FetchLightningNode attempts to look up a target node by its identity
	// public key. channeldb.ErrGraphNodeNotFound is returned if the node
	// doesn't exist within the graph.
	FetchLightningNode(route.Vertex) (*channeldb.LightningNode, error)

	// ForEachNode is used to iterate over every node in the known graph.
	ForEachNode(func(node *channeldb.LightningNode) error) error
}

// PaymentAttemptDispatcher is used by the router to send payment attempts onto
// the network, and receive their results.
type PaymentAttemptDispatcher interface {
	// SendHTLC is a function that directs a link-layer switch to
	// forward a fully encoded payment to the first hop in the route
	// denoted by its public key. A non-nil error is to be returned if the
	// payment was unsuccessful.
	SendHTLC(firstHop lnwire.ShortChannelID,
		attemptID uint64,
		htlcAdd *lnwire.UpdateAddHTLC) error

	// GetAttemptResult returns the result of the payment attempt with
	// the given attemptID. The paymentHash should be set to the payment's
	// overall hash, or in case of AMP payments the payment's unique
	// identifier.
	//
	// The method returns a channel where the payment result will be sent
	// when available, or an error is encountered during forwarding. When a
	// result is received on the channel, the HTLC is guaranteed to no
	// longer be in flight.  The switch shutting down is signaled by
	// closing the channel. If the attemptID is unknown,
	// ErrPaymentIDNotFound will be returned.
	GetAttemptResult(attemptID uint64, paymentHash lntypes.Hash,
		deobfuscator htlcswitch.ErrorDecrypter) (
		<-chan *htlcswitch.PaymentResult, error)

	// CleanStore calls the underlying result store, telling it is safe to
	// delete all entries except the ones in the keepPids map. This should
	// be called periodically to let the switch clean up payment results
	// that we have handled.
	// NOTE: New payment attempts MUST NOT be made after the keepPids map
	// has been created and this method has returned.
	CleanStore(keepPids map[uint64]struct{}) error
}

// PaymentSessionSource is an interface that defines a source for the router to
// retrieve new payment sessions.
type PaymentSessionSource interface {
	// NewPaymentSession creates a new payment session that will produce
	// routes to the given target. An optional set of routing hints can be
	// provided in order to populate additional edges to explore when
	// finding a path to the payment's destination.
	NewPaymentSession(p *LightningPayment) (PaymentSession, error)

	// NewPaymentSessionEmpty creates a new paymentSession instance that is
	// empty, and will be exhausted immediately. Used for failure reporting
	// to missioncontrol for resumed payment we don't want to make more
	// attempts for.
	NewPaymentSessionEmpty() PaymentSession
}

// MissionController is an interface that exposes failure reporting and
// probability estimation.
type MissionController interface {
	// ReportPaymentFail reports a failed payment to mission control as
	// input for future probability estimates. It returns a bool indicating
	// whether this error is a final error and no further payment attempts
	// need to be made.
	ReportPaymentFail(attemptID uint64, rt *route.Route,
		failureSourceIdx *int, failure lnwire.FailureMessage) (
		*channeldb.FailureReason, error)

	// ReportPaymentSuccess reports a successful payment to mission control
	// as input for future probability estimates.
	ReportPaymentSuccess(attemptID uint64, rt *route.Route) error

	// GetProbability is expected to return the success probability of a
	// payment from fromNode along edge.
	GetProbability(fromNode, toNode route.Vertex,
		amt lnwire.MilliSatoshi, capacity btcutil.Amount) float64
}

// FeeSchema is the set fee configuration for a Lightning Node on the network.
// Using the coefficients described within the schema, the required fee to
// forward outgoing payments can be derived.
type FeeSchema struct {
	// BaseFee is the base amount of milli-satoshis that will be chained
	// for ANY payment forwarded.
	BaseFee lnwire.MilliSatoshi

	// FeeRate is the rate that will be charged for forwarding payments.
	// This value should be interpreted as the numerator for a fraction
	// (fixed point arithmetic) whose denominator is 1 million. As a result
	// the effective fee rate charged per mSAT will be: (amount *
	// FeeRate/1,000,000).
	FeeRate uint32

	// InboundFee is the inbound fee schedule that applies to forwards
	// coming in through a channel to which this FeeSchema pertains.
	InboundFee fn.Option[models.InboundFee]
}

// ChannelPolicy holds the parameters that determine the policy we enforce
// when forwarding payments on a channel. These parameters are communicated
// to the rest of the network in ChannelUpdate messages.
type ChannelPolicy struct {
	// FeeSchema holds the fee configuration for a channel.
	FeeSchema

	// TimeLockDelta is the required HTLC timelock delta to be used
	// when forwarding payments.
	TimeLockDelta uint32

	// MaxHTLC is the maximum HTLC size including fees we are allowed to
	// forward over this channel.
	MaxHTLC lnwire.MilliSatoshi

	// MinHTLC is the minimum HTLC size including fees we are allowed to
	// forward over this channel.
	MinHTLC *lnwire.MilliSatoshi
}

// Config defines the configuration for the ChannelRouter. ALL elements within
// the configuration MUST be non-nil for the ChannelRouter to carry out its
// duties.
type Config struct {
	GraphMgr graph.Graph

	// RoutingGraph is a graph source that will be used for pathfinding.
	RoutingGraph RoutingGraph

	// Graph is the channel graph that the ChannelRouter will use to gather
	// metrics from and also to carry out path finding queries.
	// TODO(roasbeef): make into an interface
	Graph *channeldb.ChannelGraph

	// Chain is the router's source to the most up-to-date blockchain data.
	// All incoming advertised channels will be checked against the chain
	// to ensure that the channels advertised are still open.
	Chain lnwallet.BlockChainIO

	BestBlockView chainntnfs.BestBlockView

	// ChainView is an instance of a FilteredChainView which is used to
	// watch the sub-set of the UTXO set (the set of active channels) that
	// we need in order to properly maintain the channel graph.
	// The ChainView MUST have already been started.
	ChainView chainview.FilteredChainView

	// Payer is an instance of a PaymentAttemptDispatcher and is used by
	// the router to send payment attempts onto the network, and receive
	// their results.
	Payer PaymentAttemptDispatcher

	// Control keeps track of the status of ongoing payments, ensuring we
	// can properly resume them across restarts.
	Control ControlTower

	// MissionControl is a shared memory of sorts that executions of
	// payment path finding use in order to remember which vertexes/edges
	// were pruned from prior attempts. During SendPayment execution,
	// errors sent by nodes are mapped into a vertex or edge to be pruned.
	// Each run will then take into account this set of pruned
	// vertexes/edges to reduce route failure and pass on graph information
	// gained to the next execution.
	MissionControl MissionController

	// SessionSource defines a source for the router to retrieve new payment
	// sessions.
	SessionSource PaymentSessionSource

	// ChannelPruneExpiry is the duration used to determine if a channel
	// should be pruned or not. If the delta between now and when the
	// channel was last updated is greater than ChannelPruneExpiry, then
	// the channel is marked as a zombie channel eligible for pruning.
	ChannelPruneExpiry time.Duration

	// QueryBandwidth is a method that allows the router to query the lower
	// link layer to determine the up to date available bandwidth at a
	// prospective link to be traversed. If the  link isn't available, then
	// a value of zero should be returned. Otherwise, the current up to
	// date knowledge of the available bandwidth of the link should be
	// returned.
	GetLink getLinkQuery

	// NextPaymentID is a method that guarantees to return a new, unique ID
	// each time it is called. This is used by the router to generate a
	// unique payment ID for each payment it attempts to send, such that
	// the switch can properly handle the HTLC.
	NextPaymentID func() (uint64, error)

	// AssumeChannelValid toggles whether or not the router will check for
	// spentness of channel outpoints. For neutrino, this saves long rescans
	// from blocking initial usage of the daemon.
	AssumeChannelValid bool

	// PathFindingConfig defines global path finding parameters.
	PathFindingConfig PathFindingConfig

	// Clock is mockable time provider.
	Clock clock.Clock

	// IsAlias returns whether a passed ShortChannelID is an alias. This is
	// only used for our local channels.
	IsAlias func(scid lnwire.ShortChannelID) bool
}

// EdgeLocator is a struct used to identify a specific edge.
type EdgeLocator struct {
	// ChannelID is the channel of this edge.
	ChannelID uint64

	// Direction takes the value of 0 or 1 and is identical in definition to
	// the channel direction flag. A value of 0 means the direction from the
	// lower node pubkey to the higher.
	Direction uint8
}

// String returns a human readable version of the edgeLocator values.
func (e *EdgeLocator) String() string {
	return fmt.Sprintf("%v:%v", e.ChannelID, e.Direction)
}

// ChannelRouter is the layer 3 router within the Lightning stack. Below the
// ChannelRouter is the HtlcSwitch, and below that is the Bitcoin blockchain
// itself. The primary role of the ChannelRouter is to respond to queries for
// potential routes that can support a payment amount, and also general graph
// reachability questions. The router will prune the channel graph
// automatically as new blocks are discovered which spend certain known funding
// outpoints, thereby closing their respective channels.
type ChannelRouter struct {
	started uint32 // To be used atomically.
	stopped uint32 // To be used atomically.

	// cfg is a copy of the configuration struct that the ChannelRouter was
	// initialized with.
	cfg *Config

	// selfNode is the center of the star-graph centered around the
	// ChannelRouter. The ChannelRouter uses this node as a starting point
	// when doing any path finding.
	selfNode *channeldb.LightningNode

	// networkUpdates is a channel that carries new topology updates
	// messages from outside the ChannelRouter to be processed by the
	// networkHandler.
	networkUpdates chan *routingMsg

	// channelEdgeMtx is a mutex we use to make sure we process only one
	// ChannelEdgePolicy at a time for a given channelID, to ensure
	// consistency between the various database accesses.
	channelEdgeMtx *multimutex.Mutex[uint64]

	// statTicker is a resumable ticker that logs the router's progress as
	// it discovers channels or receives updates.
	statTicker ticker.Ticker

	// stats tracks newly processed channels, updates, and node
	// announcements over a window of defaultStatInterval.
	stats *routerStats

	quit chan struct{}
	wg   sync.WaitGroup
}

// A compile time check to ensure ChannelRouter implements the
// ChannelGraphSource interface.
var _ ChannelGraphSource = (*ChannelRouter)(nil)

// New creates a new instance of the ChannelRouter with the specified
// configuration parameters. As part of initialization, if the router detects
// that the channel graph isn't fully in sync with the latest UTXO (since the
// channel graph is a subset of the UTXO set) set, then the router will proceed
// to fully sync to the latest state of the UTXO set.
func New(cfg Config) (*ChannelRouter, error) {
	selfNode, err := cfg.Graph.SourceNode()
	if err != nil {
		return nil, err
	}

	r := &ChannelRouter{
		cfg:            &cfg,
		networkUpdates: make(chan *routingMsg),
		channelEdgeMtx: multimutex.NewMutex[uint64](),
		selfNode:       selfNode,
		statTicker:     ticker.New(defaultStatInterval),
		stats:          new(routerStats),
		quit:           make(chan struct{}),
	}

	return r, nil
}

// Start launches all the goroutines the ChannelRouter requires to carry out
// its duties. If the router has already been started, then this method is a
// noop.
func (r *ChannelRouter) Start() error {
	if !atomic.CompareAndSwapUint32(&r.started, 0, 1) {
		return nil
	}

	log.Info("Channel Router starting")

	// If any payments are still in flight, we resume, to make sure their
	// results are properly handled.
	payments, err := r.cfg.Control.FetchInFlightPayments()
	if err != nil {
		return err
	}

	// Before we restart existing payments and start accepting more
	// payments to be made, we clean the network result store of the
	// Switch. We do this here at startup to ensure no more payments can be
	// made concurrently, so we know the toKeep map will be up-to-date
	// until the cleaning has finished.
	toKeep := make(map[uint64]struct{})
	for _, p := range payments {
		for _, a := range p.HTLCs {
			toKeep[a.AttemptID] = struct{}{}
		}
	}

	log.Debugf("Cleaning network result store.")
	if err := r.cfg.Payer.CleanStore(toKeep); err != nil {
		return err
	}

	for _, payment := range payments {
		log.Infof("Resuming payment %v", payment.Info.PaymentIdentifier)
		r.wg.Add(1)
		go func(payment *channeldb.MPPayment) {
			defer r.wg.Done()

			// Get the hashes used for the outstanding HTLCs.
			htlcs := make(map[uint64]lntypes.Hash)
			for _, a := range payment.HTLCs {
				a := a

				// We check whether the individual attempts
				// have their HTLC hash set, if not we'll fall
				// back to the overall payment hash.
				hash := payment.Info.PaymentIdentifier
				if a.Hash != nil {
					hash = *a.Hash
				}

				htlcs[a.AttemptID] = hash
			}

			// Since we are not supporting creating more shards
			// after a restart (only receiving the result of the
			// shards already outstanding), we create a simple
			// shard tracker that will map the attempt IDs to
			// hashes used for the HTLCs. This will be enough also
			// for AMP payments, since we only need the hashes for
			// the individual HTLCs to regenerate the circuits, and
			// we don't currently persist the root share necessary
			// to re-derive them.
			shardTracker := shards.NewSimpleShardTracker(
				payment.Info.PaymentIdentifier, htlcs,
			)

			// We create a dummy, empty payment session such that
			// we won't make another payment attempt when the
			// result for the in-flight attempt is received.
			paySession := r.cfg.SessionSource.NewPaymentSessionEmpty()

			// We pass in a zero timeout value, to indicate we
			// don't need it to timeout. It will stop immediately
			// after the existing attempt has finished anyway. We
			// also set a zero fee limit, as no more routes should
			// be tried.
			_, _, err := r.sendPayment(
				0, payment.Info.PaymentIdentifier, 0,
				paySession, shardTracker,
			)
			if err != nil {
				log.Errorf("Resuming payment %v failed: %v.",
					payment.Info.PaymentIdentifier, err)
				return
			}

			log.Infof("Resumed payment %v completed.",
				payment.Info.PaymentIdentifier)
		}(payment)
	}

	r.wg.Add(1)
	go r.networkHandler()

	return nil
}

// Stop signals the ChannelRouter to gracefully halt all routines. This method
// will *block* until all goroutines have excited. If the channel router has
// already stopped then this method will return immediately.
func (r *ChannelRouter) Stop() error {
	if !atomic.CompareAndSwapUint32(&r.stopped, 0, 1) {
		return nil
	}

	log.Info("Channel Router shutting down...")
	defer log.Debug("Channel Router shutdown complete")

	close(r.quit)
	r.wg.Wait()

	return nil
}

// handleNetworkUpdate is responsible for processing the update message and
// notifies topology changes, if any.
//
// NOTE: must be run inside goroutine.
func (r *ChannelRouter) handleNetworkUpdate(vb *ValidationBarrier,
	update *routingMsg) {

	defer r.wg.Done()
	defer vb.CompleteJob()

	// If this message has an existing dependency, then we'll wait until
	// that has been fully validated before we proceed.
	err := vb.WaitForDependants(update.msg)
	if err != nil {
		switch {
		case IsError(err, ErrVBarrierShuttingDown):
			update.err <- err

		case IsError(err, ErrParentValidationFailed):
			update.err <- newErrf(ErrIgnored, err.Error())

		default:
			log.Warnf("unexpected error during validation "+
				"barrier shutdown: %v", err)
			update.err <- err
		}

		return
	}

	// Process the routing update to determine if this is either a new
	// update from our PoV or an update to a prior vertex/edge we
	// previously accepted.
	err = r.processUpdate(update.msg, update.op...)
	update.err <- err

	// If this message had any dependencies, then we can now signal them to
	// continue.
	allowDependents := err == nil || IsError(err, ErrIgnored, ErrOutdated)
	vb.SignalDependants(update.msg, allowDependents)

	// If the error is not nil here, there's no need to send topology
	// change.
	if err != nil {
		// We now decide to log an error or not. If allowDependents is
		// false, it means there is an error and the error is neither
		// ErrIgnored or ErrOutdated. In this case, we'll log an error.
		// Otherwise, we'll add debug log only.
		if allowDependents {
			log.Debugf("process network updates got: %v", err)
		} else {
			log.Errorf("process network updates got: %v", err)
		}

		return
	}

	// Otherwise, we'll send off a new notification for the newly accepted
	// update, if any.
	r.cfg.GraphMgr.AddTopologyChange(update.msg)
}

// networkHandler is the primary goroutine for the ChannelRouter. The roles of
// this goroutine include answering queries related to the state of the
// network, pruning the graph on new block notification, applying network
// updates, and registering new topology clients.
//
// NOTE: This MUST be run as a goroutine.
func (r *ChannelRouter) networkHandler() {
	defer r.wg.Done()
	defer r.statTicker.Stop()

	r.stats.Reset()

	// We'll use this validation barrier to ensure that we process all jobs
	// in the proper order during parallel validation.
	//
	// NOTE: For AssumeChannelValid, we bump up the maximum number of
	// concurrent validation requests since there are no blocks being
	// fetched. This significantly increases the performance of IGD for
	// neutrino nodes.
	//
	// However, we dial back to use multiple of the number of cores when
	// fully validating, to avoid fetching up to 1000 blocks from the
	// backend. On bitcoind, this will empirically cause massive latency
	// spikes when executing this many concurrent RPC calls. Critical
	// subsystems or basic rpc calls that rely on calls such as GetBestBlock
	// will hang due to excessive load.
	//
	// See https://github.com/lightningnetwork/lnd/issues/4892.
	var validationBarrier *ValidationBarrier
	if r.cfg.AssumeChannelValid {
		validationBarrier = NewValidationBarrier(1000, r.quit)
	} else {
		validationBarrier = NewValidationBarrier(
			4*runtime.NumCPU(), r.quit,
		)
	}

	for {

		// If there are stats, resume the statTicker.
		if !r.stats.Empty() {
			r.statTicker.Resume()
		}

		select {
		// A new fully validated network update has just arrived. As a
		// result we'll modify the channel graph accordingly depending
		// on the exact type of the message.
		case update := <-r.networkUpdates:
			// We'll set up any dependants, and wait until a free
			// slot for this job opens up, this allows us to not
			// have thousands of goroutines active.
			validationBarrier.InitJobDependencies(update.msg)

			r.wg.Add(1)
			go r.handleNetworkUpdate(validationBarrier, update)

			// TODO(roasbeef): remove all unconnected vertexes
			// after N blocks pass with no corresponding
			// announcements.

		// Log any stats if we've processed a non-empty number of
		// channels, updates, or nodes. We'll only pause the ticker if
		// the last window contained no updates to avoid resuming and
		// pausing while consecutive windows contain new info.
		case <-r.statTicker.Ticks():
			if !r.stats.Empty() {
				log.Infof(r.stats.String())
			} else {
				r.statTicker.Pause()
			}
			r.stats.Reset()

		// The router has been signalled to exit, to we exit our main
		// loop so the wait group can be decremented.
		case <-r.quit:
			return
		}
	}
}

// assertNodeAnnFreshness returns a non-nil error if we have an announcement in
// the database for the passed node with a timestamp newer than the passed
// timestamp. ErrIgnored will be returned if we already have the node, and
// ErrOutdated will be returned if we have a timestamp that's after the new
// timestamp.
func (r *ChannelRouter) assertNodeAnnFreshness(node route.Vertex,
	msgTimestamp time.Time) error {

	// If we are not already aware of this node, it means that we don't
	// know about any channel using this node. To avoid a DoS attack by
	// node announcements, we will ignore such nodes. If we do know about
	// this node, check that this update brings info newer than what we
	// already have.
	lastUpdate, exists, err := r.cfg.Graph.HasLightningNode(node)
	if err != nil {
		return errors.Errorf("unable to query for the "+
			"existence of node: %v", err)
	}
	if !exists {
		return newErrf(ErrIgnored, "Ignoring node announcement"+
			" for node not found in channel graph (%x)",
			node[:])
	}

	// If we've reached this point then we're aware of the vertex being
	// advertised. So we now check if the new message has a new time stamp,
	// if not then we won't accept the new data as it would override newer
	// data.
	if !lastUpdate.Before(msgTimestamp) {
		return newErrf(ErrOutdated, "Ignoring outdated "+
			"announcement for %x", node[:])
	}

	return nil
}

// addZombieEdge adds a channel that failed complete validation into the zombie
// index so we can avoid having to re-validate it in the future.
func (r *ChannelRouter) addZombieEdge(chanID uint64) error {
	// If the edge fails validation we'll mark the edge itself as a zombie
	// so we don't continue to request it. We use the "zero key" for both
	// node pubkeys so this edge can't be resurrected.
	var zeroKey [33]byte
	err := r.cfg.Graph.MarkEdgeZombie(chanID, zeroKey, zeroKey)
	if err != nil {
		return fmt.Errorf("unable to mark spent chan(id=%v) as a "+
			"zombie: %w", chanID, err)
	}

	return nil
}

// makeFundingScript is used to make the funding script for both segwit v0 and
// segwit v1 (taproot) channels.
//
// TODO(roasbeef: export and use elsewhere?
func makeFundingScript(bitcoinKey1, bitcoinKey2 []byte,
	chanFeatures []byte) ([]byte, error) {

	legacyFundingScript := func() ([]byte, error) {
		witnessScript, err := input.GenMultiSigScript(
			bitcoinKey1, bitcoinKey2,
		)
		if err != nil {
			return nil, err
		}
		pkScript, err := input.WitnessScriptHash(witnessScript)
		if err != nil {
			return nil, err
		}

		return pkScript, nil
	}

	if len(chanFeatures) == 0 {
		return legacyFundingScript()
	}

	// In order to make the correct funding script, we'll need to parse the
	// chanFeatures bytes into a feature vector we can interact with.
	rawFeatures := lnwire.NewRawFeatureVector()
	err := rawFeatures.Decode(bytes.NewReader(chanFeatures))
	if err != nil {
		return nil, fmt.Errorf("unable to parse chan feature "+
			"bits: %w", err)
	}

	chanFeatureBits := lnwire.NewFeatureVector(
		rawFeatures, lnwire.Features,
	)
	if chanFeatureBits.HasFeature(
		lnwire.SimpleTaprootChannelsOptionalStaging,
	) {

		pubKey1, err := btcec.ParsePubKey(bitcoinKey1)
		if err != nil {
			return nil, err
		}
		pubKey2, err := btcec.ParsePubKey(bitcoinKey2)
		if err != nil {
			return nil, err
		}

		fundingScript, _, err := input.GenTaprootFundingScript(
			pubKey1, pubKey2, 0,
		)
		if err != nil {
			return nil, err
		}

		return fundingScript, nil
	}

	return legacyFundingScript()
}

// processUpdate processes a new relate authenticated channel/edge, node or
// channel/edge update network update. If the update didn't affect the internal
// state of the draft due to either being out of date, invalid, or redundant,
// then error is returned.
func (r *ChannelRouter) processUpdate(msg interface{},
	op ...batch.SchedulerOption) error {

	switch msg := msg.(type) {
	case *channeldb.LightningNode:
		// Before we add the node to the database, we'll check to see
		// if the announcement is "fresh" or not. If it isn't, then
		// we'll return an error.
		err := r.assertNodeAnnFreshness(msg.PubKeyBytes, msg.LastUpdate)
		if err != nil {
			return err
		}

		if err := r.cfg.Graph.AddLightningNode(msg, op...); err != nil {
			return errors.Errorf("unable to add node %x to the "+
				"graph: %v", msg.PubKeyBytes, err)
		}

		log.Tracef("Updated vertex data for node=%x", msg.PubKeyBytes)
		r.stats.incNumNodeUpdates()

	case *models.ChannelEdgeInfo:
		log.Debugf("Received ChannelEdgeInfo for channel %v",
			msg.ChannelID)

		// Prior to processing the announcement we first check if we
		// already know of this channel, if so, then we can exit early.
		_, _, exists, isZombie, err := r.cfg.Graph.HasChannelEdge(
			msg.ChannelID,
		)
		if err != nil && err != channeldb.ErrGraphNoEdgesFound {
			return errors.Errorf("unable to check for edge "+
				"existence: %v", err)
		}
		if isZombie {
			return newErrf(ErrIgnored, "ignoring msg for zombie "+
				"chan_id=%v", msg.ChannelID)
		}
		if exists {
			return newErrf(ErrIgnored, "ignoring msg for known "+
				"chan_id=%v", msg.ChannelID)
		}

		// If AssumeChannelValid is present, then we are unable to
		// perform any of the expensive checks below, so we'll
		// short-circuit our path straight to adding the edge to our
		// graph. If the passed ShortChannelID is an alias, then we'll
		// skip validation as it will not map to a legitimate tx. This
		// is not a DoS vector as only we can add an alias
		// ChannelAnnouncement from the gossiper.
		scid := lnwire.NewShortChanIDFromInt(msg.ChannelID)
		if r.cfg.AssumeChannelValid || r.cfg.IsAlias(scid) {
			if err := r.cfg.Graph.AddChannelEdge(msg, op...); err != nil {
				return fmt.Errorf("unable to add edge: %w", err)
			}
			log.Tracef("New channel discovered! Link "+
				"connects %x and %x with ChannelID(%v)",
				msg.NodeKey1Bytes, msg.NodeKey2Bytes,
				msg.ChannelID)
			r.stats.incNumEdgesDiscovered()

			break
		}

		// Before we can add the channel to the channel graph, we need
		// to obtain the full funding outpoint that's encoded within
		// the channel ID.
		channelID := lnwire.NewShortChanIDFromInt(msg.ChannelID)
		fundingTx, err := r.fetchFundingTxWrapper(&channelID)
		if err != nil {
			// In order to ensure we don't erroneously mark a
			// channel as a zombie due to an RPC failure, we'll
			// attempt to string match for the relevant errors.
			//
			// * btcd:
			//    * https://github.com/btcsuite/btcd/blob/master/rpcserver.go#L1316
			//    * https://github.com/btcsuite/btcd/blob/master/rpcserver.go#L1086
			// * bitcoind:
			//    * https://github.com/bitcoin/bitcoin/blob/7fcf53f7b4524572d1d0c9a5fdc388e87eb02416/src/rpc/blockchain.cpp#L770
			//     * https://github.com/bitcoin/bitcoin/blob/7fcf53f7b4524572d1d0c9a5fdc388e87eb02416/src/rpc/blockchain.cpp#L954
			switch {
			case strings.Contains(err.Error(), "not found"):
				fallthrough

			case strings.Contains(err.Error(), "out of range"):
				// If the funding transaction isn't found at
				// all, then we'll mark the edge itself as a
				// zombie so we don't continue to request it.
				// We use the "zero key" for both node pubkeys
				// so this edge can't be resurrected.
				zErr := r.addZombieEdge(msg.ChannelID)
				if zErr != nil {
					return zErr
				}

			default:
			}

			return newErrf(ErrNoFundingTransaction, "unable to "+
				"locate funding tx: %v", err)
		}

		// Recreate witness output to be sure that declared in channel
		// edge bitcoin keys and channel value corresponds to the
		// reality.
		fundingPkScript, err := makeFundingScript(
			msg.BitcoinKey1Bytes[:], msg.BitcoinKey2Bytes[:],
			msg.Features,
		)
		if err != nil {
			return err
		}

		// Next we'll validate that this channel is actually well
		// formed. If this check fails, then this channel either
		// doesn't exist, or isn't the one that was meant to be created
		// according to the passed channel proofs.
		fundingPoint, err := chanvalidate.Validate(&chanvalidate.Context{
			Locator: &chanvalidate.ShortChanIDChanLocator{
				ID: channelID,
			},
			MultiSigPkScript: fundingPkScript,
			FundingTx:        fundingTx,
		})
		if err != nil {
			// Mark the edge as a zombie so we won't try to
			// re-validate it on start up.
			if err := r.addZombieEdge(msg.ChannelID); err != nil {
				return err
			}

			return newErrf(ErrInvalidFundingOutput, "output "+
				"failed validation: %w", err)
		}

		// Now that we have the funding outpoint of the channel, ensure
		// that it hasn't yet been spent. If so, then this channel has
		// been closed so we'll ignore it.
		chanUtxo, err := r.cfg.Chain.GetUtxo(
			fundingPoint, fundingPkScript, channelID.BlockHeight,
			r.quit,
		)
		if err != nil {
			if errors.Is(err, btcwallet.ErrOutputSpent) {
				zErr := r.addZombieEdge(msg.ChannelID)
				if zErr != nil {
					return zErr
				}
			}

			return newErrf(ErrChannelSpent, "unable to fetch utxo "+
				"for chan_id=%v, chan_point=%v: %v",
				msg.ChannelID, fundingPoint, err)
		}

		// TODO(roasbeef): this is a hack, needs to be removed
		// after commitment fees are dynamic.
		msg.Capacity = btcutil.Amount(chanUtxo.Value)
		msg.ChannelPoint = *fundingPoint
		if err := r.cfg.Graph.AddChannelEdge(msg, op...); err != nil {
			return errors.Errorf("unable to add edge: %v", err)
		}

		log.Debugf("New channel discovered! Link "+
			"connects %x and %x with ChannelPoint(%v): "+
			"chan_id=%v, capacity=%v",
			msg.NodeKey1Bytes, msg.NodeKey2Bytes,
			fundingPoint, msg.ChannelID, msg.Capacity)
		r.stats.incNumEdgesDiscovered()

		// As a new edge has been added to the channel graph, we'll
		// update the current UTXO filter within our active
		// FilteredChainView so we are notified if/when this channel is
		// closed.
		filterUpdate := []channeldb.EdgePoint{
			{
				FundingPkScript: fundingPkScript,
				OutPoint:        *fundingPoint,
			},
		}

		bestHeight, err := r.cfg.BestBlockView.BestHeight()
		if err != nil {
			return errors.Errorf("could not get best height: %v",
				err)
		}

		err = r.cfg.ChainView.UpdateFilter(filterUpdate, bestHeight)
		if err != nil {
			return errors.Errorf("unable to update chain view: %v",
				err)
		}

	case *models.ChannelEdgePolicy:
		log.Debugf("Received ChannelEdgePolicy for channel %v",
			msg.ChannelID)

		// We make sure to hold the mutex for this channel ID,
		// such that no other goroutine is concurrently doing
		// database accesses for the same channel ID.
		r.channelEdgeMtx.Lock(msg.ChannelID)
		defer r.channelEdgeMtx.Unlock(msg.ChannelID)

		edge1Timestamp, edge2Timestamp, exists, isZombie, err :=
			r.cfg.Graph.HasChannelEdge(msg.ChannelID)
		if err != nil && err != channeldb.ErrGraphNoEdgesFound {
			return errors.Errorf("unable to check for edge "+
				"existence: %v", err)

		}

		// If the channel is marked as a zombie in our database, and
		// we consider this a stale update, then we should not apply the
		// policy.
		isStaleUpdate := time.Since(msg.LastUpdate) > r.cfg.ChannelPruneExpiry
		if isZombie && isStaleUpdate {
			return newErrf(ErrIgnored, "ignoring stale update "+
				"(flags=%v|%v) for zombie chan_id=%v",
				msg.MessageFlags, msg.ChannelFlags,
				msg.ChannelID)
		}

		// If the channel doesn't exist in our database, we cannot
		// apply the updated policy.
		if !exists {
			return newErrf(ErrIgnored, "ignoring update "+
				"(flags=%v|%v) for unknown chan_id=%v",
				msg.MessageFlags, msg.ChannelFlags,
				msg.ChannelID)
		}

		// As edges are directional edge node has a unique policy for
		// the direction of the edge they control. Therefore we first
		// check if we already have the most up to date information for
		// that edge. If this message has a timestamp not strictly
		// newer than what we already know of we can exit early.
		switch {

		// A flag set of 0 indicates this is an announcement for the
		// "first" node in the channel.
		case msg.ChannelFlags&lnwire.ChanUpdateDirection == 0:

			// Ignore outdated message.
			if !edge1Timestamp.Before(msg.LastUpdate) {
				return newErrf(ErrOutdated, "Ignoring "+
					"outdated update (flags=%v|%v) for "+
					"known chan_id=%v", msg.MessageFlags,
					msg.ChannelFlags, msg.ChannelID)
			}

		// Similarly, a flag set of 1 indicates this is an announcement
		// for the "second" node in the channel.
		case msg.ChannelFlags&lnwire.ChanUpdateDirection == 1:

			// Ignore outdated message.
			if !edge2Timestamp.Before(msg.LastUpdate) {
				return newErrf(ErrOutdated, "Ignoring "+
					"outdated update (flags=%v|%v) for "+
					"known chan_id=%v", msg.MessageFlags,
					msg.ChannelFlags, msg.ChannelID)
			}
		}

		// Now that we know this isn't a stale update, we'll apply the
		// new edge policy to the proper directional edge within the
		// channel graph.
		if err = r.cfg.Graph.UpdateEdgePolicy(msg, op...); err != nil {
			err := errors.Errorf("unable to add channel: %v", err)
			log.Error(err)
			return err
		}

		log.Tracef("New channel update applied: %v",
			newLogClosure(func() string { return spew.Sdump(msg) }))
		r.stats.incNumChannelUpdates()

	default:
		return errors.Errorf("wrong routing update message type")
	}

	return nil
}

// fetchFundingTxWrapper is a wrapper around fetchFundingTx, except that it
// will exit if the router has stopped.
func (r *ChannelRouter) fetchFundingTxWrapper(chanID *lnwire.ShortChannelID) (
	*wire.MsgTx, error) {

	txChan := make(chan *wire.MsgTx, 1)
	errChan := make(chan error, 1)

	go func() {
		tx, err := r.fetchFundingTx(chanID)
		if err != nil {
			errChan <- err
			return
		}

		txChan <- tx
	}()

	select {
	case tx := <-txChan:
		return tx, nil

	case err := <-errChan:
		return nil, err

	case <-r.quit:
		return nil, ErrRouterShuttingDown
	}
}

// fetchFundingTx returns the funding transaction identified by the passed
// short channel ID.
//
// TODO(roasbeef): replace with call to GetBlockTransaction? (would allow to
// later use getblocktxn)
func (r *ChannelRouter) fetchFundingTx(
	chanID *lnwire.ShortChannelID) (*wire.MsgTx, error) {

	// First fetch the block hash by the block number encoded, then use
	// that hash to fetch the block itself.
	blockNum := int64(chanID.BlockHeight)
	blockHash, err := r.cfg.Chain.GetBlockHash(blockNum)
	if err != nil {
		return nil, err
	}
	fundingBlock, err := r.cfg.Chain.GetBlock(blockHash)
	if err != nil {
		return nil, err
	}

	// As a sanity check, ensure that the advertised transaction index is
	// within the bounds of the total number of transactions within a
	// block.
	numTxns := uint32(len(fundingBlock.Transactions))
	if chanID.TxIndex > numTxns-1 {
		return nil, fmt.Errorf("tx_index=#%v "+
			"is out of range (max_index=%v), network_chan_id=%v",
			chanID.TxIndex, numTxns-1, chanID)
	}

	return fundingBlock.Transactions[chanID.TxIndex].Copy(), nil
}

// routingMsg couples a routing related routing topology update to the
// error channel.
type routingMsg struct {
	msg interface{}
	op  []batch.SchedulerOption
	err chan error
}

// RouteRequest contains the parameters for a pathfinding request. It may
// describe a request to make a regular payment or one to a blinded path
// (incdicated by a non-nil BlindedPayment field).
type RouteRequest struct {
	// Source is the node that the path originates from.
	Source route.Vertex

	// Target is the node that the path terminates at. If the route
	// includes a blinded path, target will be the blinded node id of the
	// final hop in the blinded route.
	Target route.Vertex

	// Amount is the Amount in millisatoshis to be delivered to the target
	// node.
	Amount lnwire.MilliSatoshi

	// TimePreference expresses the caller's time preference for
	// pathfinding.
	TimePreference float64

	// Restrictions provides a set of additional Restrictions that the
	// route must adhere to.
	Restrictions *RestrictParams

	// CustomRecords is a set of custom tlv records to include for the
	// final hop.
	CustomRecords record.CustomSet

	// RouteHints contains an additional set of edges to include in our
	// view of the graph. This may either be a set of hints for private
	// channels or a "virtual" hop hint that represents a blinded route.
	RouteHints RouteHints

	// FinalExpiry is the cltv delta for the final hop. If paying to a
	// blinded path, this value is a duplicate of the delta provided
	// in blinded payment.
	FinalExpiry uint16

	// BlindedPayment contains an optional blinded path and parameters
	// used to reach a target node via a blinded path. This field is
	// mutually exclusive with the Target field.
	BlindedPayment *BlindedPayment
}

// RouteHints is an alias type for a set of route hints, with the source node
// as the map's key and the details of the hint(s) in the edge policy.
type RouteHints map[route.Vertex][]AdditionalEdge

// NewRouteRequest produces a new route request for a regular payment or one
// to a blinded route, validating that the target, routeHints and finalExpiry
// parameters are mutually exclusive with the blindedPayment parameter (which
// contains these values for blinded payments).
func NewRouteRequest(source route.Vertex, target *route.Vertex,
	amount lnwire.MilliSatoshi, timePref float64,
	restrictions *RestrictParams, customRecords record.CustomSet,
	routeHints RouteHints, blindedPayment *BlindedPayment,
	finalExpiry uint16) (*RouteRequest, error) {

	var (
		// Assume that we're starting off with a regular payment.
		requestHints  = routeHints
		requestExpiry = finalExpiry
	)

	if blindedPayment != nil {
		if err := blindedPayment.Validate(); err != nil {
			return nil, fmt.Errorf("invalid blinded payment: %w",
				err)
		}

		introVertex := route.NewVertex(
			blindedPayment.BlindedPath.IntroductionPoint,
		)
		if source == introVertex {
			return nil, ErrSelfIntro
		}

		// Check that the values for a clear path have not been set,
		// as this is an ambiguous signal from the caller.
		if routeHints != nil {
			return nil, ErrHintsAndBlinded
		}

		if finalExpiry != 0 {
			return nil, ErrExpiryAndBlinded
		}

		// If we have a blinded path with 1 hop, the cltv expiry
		// will not be included in any hop hints (since we're just
		// sending to the introduction node and need no blinded hints).
		// In this case, we include it to make sure that the final
		// cltv delta is accounted for (since it's part of the blinded
		// delta). In the case of a multi-hop route, we set our final
		// cltv to zero, since it's going to be accounted for in the
		// delta for our hints.
		if len(blindedPayment.BlindedPath.BlindedHops) == 1 {
			requestExpiry = blindedPayment.CltvExpiryDelta
		}

		requestHints = blindedPayment.toRouteHints()
	}

	requestTarget, err := getTargetNode(target, blindedPayment)
	if err != nil {
		return nil, err
	}

	return &RouteRequest{
		Source:         source,
		Target:         requestTarget,
		Amount:         amount,
		TimePreference: timePref,
		Restrictions:   restrictions,
		CustomRecords:  customRecords,
		RouteHints:     requestHints,
		FinalExpiry:    requestExpiry,
		BlindedPayment: blindedPayment,
	}, nil
}

func getTargetNode(target *route.Vertex, blindedPayment *BlindedPayment) (
	route.Vertex, error) {

	var (
		blinded   = blindedPayment != nil
		targetSet = target != nil
	)

	switch {
	case blinded && targetSet:
		return route.Vertex{}, ErrTargetAndBlinded

	case blinded:
		// If we're dealing with an edge-case blinded path that just
		// has an introduction node (first hop expected to be the intro
		// hop), then we return the unblinded introduction node as our
		// target.
		hops := blindedPayment.BlindedPath.BlindedHops
		if len(hops) == 1 {
			return route.NewVertex(
				blindedPayment.BlindedPath.IntroductionPoint,
			), nil
		}

		return route.NewVertex(hops[len(hops)-1].BlindedNodePub), nil

	case targetSet:
		return *target, nil

	default:
		return route.Vertex{}, ErrNoTarget
	}
}

// blindedPath returns the request's blinded path, which is set if the payment
// is to a blinded route.
func (r *RouteRequest) blindedPath() *sphinx.BlindedPath {
	if r.BlindedPayment == nil {
		return nil
	}

	return r.BlindedPayment.BlindedPath
}

// FindRoute attempts to query the ChannelRouter for the optimum path to a
// particular target destination to which it is able to send `amt` after
// factoring in channel capacities and cumulative fees along the route.
func (r *ChannelRouter) FindRoute(req *RouteRequest) (*route.Route, float64,
	error) {

	log.Debugf("Searching for path to %v, sending %v", req.Target,
		req.Amount)

	// We'll attempt to obtain a set of bandwidth hints that can help us
	// eliminate certain routes early on in the path finding process.
	bandwidthHints, err := newBandwidthManager(
		r.cfg.RoutingGraph, r.selfNode.PubKeyBytes, r.cfg.GetLink,
	)
	if err != nil {
		return nil, 0, err
	}

	// We'll fetch the current block height so we can properly calculate the
	// required HTLC time locks within the route.
	_, currentHeight, err := r.cfg.Chain.GetBestBlock()
	if err != nil {
		return nil, 0, err
	}

	// Now that we know the destination is reachable within the graph, we'll
	// execute our path finding algorithm.
	finalHtlcExpiry := currentHeight + int32(req.FinalExpiry)

	// Validate time preference.
	timePref := req.TimePreference
	if timePref < -1 || timePref > 1 {
		return nil, 0, errors.New("time preference out of range")
	}

	path, probability, err := findPath(
		&graphParams{
			additionalEdges: req.RouteHints,
			bandwidthHints:  bandwidthHints,
			graph:           r.cfg.RoutingGraph,
		},
		req.Restrictions, &r.cfg.PathFindingConfig,
		r.selfNode.PubKeyBytes, req.Source, req.Target, req.Amount,
		req.TimePreference, finalHtlcExpiry,
	)
	if err != nil {
		return nil, 0, err
	}

	// Create the route with absolute time lock values.
	route, err := newRoute(
		req.Source, path, uint32(currentHeight),
		finalHopParams{
			amt:       req.Amount,
			totalAmt:  req.Amount,
			cltvDelta: req.FinalExpiry,
			records:   req.CustomRecords,
		}, req.blindedPath(),
	)
	if err != nil {
		return nil, 0, err
	}

	go log.Tracef("Obtained path to send %v to %x: %v",
		req.Amount, req.Target, newLogClosure(func() string {
			return spew.Sdump(route)
		}),
	)

	return route, probability, nil
}

// generateNewSessionKey generates a new ephemeral private key to be used for a
// payment attempt.
func generateNewSessionKey() (*btcec.PrivateKey, error) {
	// Generate a new random session key to ensure that we don't trigger
	// any replay.
	//
	// TODO(roasbeef): add more sources of randomness?
	return btcec.NewPrivateKey()
}

// generateSphinxPacket generates then encodes a sphinx packet which encodes
// the onion route specified by the passed layer 3 route. The blob returned
// from this function can immediately be included within an HTLC add packet to
// be sent to the first hop within the route.
func generateSphinxPacket(rt *route.Route, paymentHash []byte,
	sessionKey *btcec.PrivateKey) ([]byte, *sphinx.Circuit, error) {

	// Now that we know we have an actual route, we'll map the route into a
	// sphinx payment path which includes per-hop payloads for each hop
	// that give each node within the route the necessary information
	// (fees, CLTV value, etc) to properly forward the payment.
	sphinxPath, err := rt.ToSphinxPath()
	if err != nil {
		return nil, nil, err
	}

	log.Tracef("Constructed per-hop payloads for payment_hash=%x: %v",
		paymentHash[:], newLogClosure(func() string {
			path := make(
				[]sphinx.OnionHop, sphinxPath.TrueRouteLength(),
			)
			for i := range path {
				hopCopy := sphinxPath[i]
				path[i] = hopCopy
			}
			return spew.Sdump(path)
		}),
	)

	// Next generate the onion routing packet which allows us to perform
	// privacy preserving source routing across the network.
	sphinxPacket, err := sphinx.NewOnionPacket(
		sphinxPath, sessionKey, paymentHash,
		sphinx.DeterministicPacketFiller,
	)
	if err != nil {
		return nil, nil, err
	}

	// Finally, encode Sphinx packet using its wire representation to be
	// included within the HTLC add packet.
	var onionBlob bytes.Buffer
	if err := sphinxPacket.Encode(&onionBlob); err != nil {
		return nil, nil, err
	}

	log.Tracef("Generated sphinx packet: %v",
		newLogClosure(func() string {
			// We make a copy of the ephemeral key and unset the
			// internal curve here in order to keep the logs from
			// getting noisy.
			key := *sphinxPacket.EphemeralKey
			packetCopy := *sphinxPacket
			packetCopy.EphemeralKey = &key
			return spew.Sdump(packetCopy)
		}),
	)

	return onionBlob.Bytes(), &sphinx.Circuit{
		SessionKey:  sessionKey,
		PaymentPath: sphinxPath.NodeKeys(),
	}, nil
}

// LightningPayment describes a payment to be sent through the network to the
// final destination.
type LightningPayment struct {
	// Target is the node in which the payment should be routed towards.
	Target route.Vertex

	// Amount is the value of the payment to send through the network in
	// milli-satoshis.
	Amount lnwire.MilliSatoshi

	// FeeLimit is the maximum fee in millisatoshis that the payment should
	// accept when sending it through the network. The payment will fail
	// if there isn't a route with lower fees than this limit.
	FeeLimit lnwire.MilliSatoshi

	// CltvLimit is the maximum time lock that is allowed for attempts to
	// complete this payment.
	CltvLimit uint32

	// paymentHash is the r-hash value to use within the HTLC extended to
	// the first hop. This won't be set for AMP payments.
	paymentHash *lntypes.Hash

	// amp is an optional field that is set if and only if this is am AMP
	// payment.
	amp *AMPOptions

	// FinalCLTVDelta is the CTLV expiry delta to use for the _final_ hop
	// in the route. This means that the final hop will have a CLTV delta
	// of at least: currentHeight + FinalCLTVDelta.
	FinalCLTVDelta uint16

	// PayAttemptTimeout is a timeout value that we'll use to determine
	// when we should should abandon the payment attempt after consecutive
	// payment failure. This prevents us from attempting to send a payment
	// indefinitely. A zero value means the payment will never time out.
	//
	// TODO(halseth): make wallclock time to allow resume after startup.
	PayAttemptTimeout time.Duration

	// RouteHints represents the different routing hints that can be used to
	// assist a payment in reaching its destination successfully. These
	// hints will act as intermediate hops along the route.
	//
	// NOTE: This is optional unless required by the payment. When providing
	// multiple routes, ensure the hop hints within each route are chained
	// together and sorted in forward order in order to reach the
	// destination successfully.
	RouteHints [][]zpay32.HopHint

	// OutgoingChannelIDs is the list of channels that are allowed for the
	// first hop. If nil, any channel may be used.
	OutgoingChannelIDs []uint64

	// LastHop is the pubkey of the last node before the final destination
	// is reached. If nil, any node may be used.
	LastHop *route.Vertex

	// DestFeatures specifies the set of features we assume the final node
	// has for pathfinding. Typically these will be taken directly from an
	// invoice, but they can also be manually supplied or assumed by the
	// sender. If a nil feature vector is provided, the router will try to
	// fallback to the graph in order to load a feature vector for a node in
	// the public graph.
	DestFeatures *lnwire.FeatureVector

	// PaymentAddr is the payment address specified by the receiver. This
	// field should be a random 32-byte nonce presented in the receiver's
	// invoice to prevent probing of the destination.
	PaymentAddr *[32]byte

	// PaymentRequest is an optional payment request that this payment is
	// attempting to complete.
	PaymentRequest []byte

	// DestCustomRecords are TLV records that are to be sent to the final
	// hop in the new onion payload format. If the destination does not
	// understand this new onion payload format, then the payment will
	// fail.
	DestCustomRecords record.CustomSet

	// MaxParts is the maximum number of partial payments that may be used
	// to complete the full amount.
	MaxParts uint32

	// MaxShardAmt is the largest shard that we'll attempt to split using.
	// If this field is set, and we need to split, rather than attempting
	// half of the original payment amount, we'll use this value if half
	// the payment amount is greater than it.
	//
	// NOTE: This field is _optional_.
	MaxShardAmt *lnwire.MilliSatoshi

	// TimePref is the time preference for this payment. Set to -1 to
	// optimize for fees only, to 1 to optimize for reliability only or a
	// value in between for a mix.
	TimePref float64

	// Metadata is additional data that is sent along with the payment to
	// the payee.
	Metadata []byte
}

// AMPOptions houses information that must be known in order to send an AMP
// payment.
type AMPOptions struct {
	SetID     [32]byte
	RootShare [32]byte
}

// SetPaymentHash sets the given hash as the payment's overall hash. This
// should only be used for non-AMP payments.
func (l *LightningPayment) SetPaymentHash(hash lntypes.Hash) error {
	if l.amp != nil {
		return fmt.Errorf("cannot set payment hash for AMP payment")
	}

	l.paymentHash = &hash
	return nil
}

// SetAMP sets the given AMP options for the payment.
func (l *LightningPayment) SetAMP(amp *AMPOptions) error {
	if l.paymentHash != nil {
		return fmt.Errorf("cannot set amp options for payment " +
			"with payment hash")
	}

	l.amp = amp
	return nil
}

// Identifier returns a 32-byte slice that uniquely identifies this single
// payment. For non-AMP payments this will be the payment hash, for AMP
// payments this will be the used SetID.
func (l *LightningPayment) Identifier() [32]byte {
	if l.amp != nil {
		return l.amp.SetID
	}

	return *l.paymentHash
}

// SendPayment attempts to send a payment as described within the passed
// LightningPayment. This function is blocking and will return either: when the
// payment is successful, or all candidates routes have been attempted and
// resulted in a failed payment. If the payment succeeds, then a non-nil Route
// will be returned which describes the path the successful payment traversed
// within the network to reach the destination. Additionally, the payment
// preimage will also be returned.
func (r *ChannelRouter) SendPayment(payment *LightningPayment) ([32]byte,
	*route.Route, error) {

	paySession, shardTracker, err := r.PreparePayment(payment)
	if err != nil {
		return [32]byte{}, nil, err
	}

	log.Tracef("Dispatching SendPayment for lightning payment: %v",
		spewPayment(payment))

	// Since this is the first time this payment is being made, we pass nil
	// for the existing attempt.
	return r.sendPayment(
		payment.FeeLimit, payment.Identifier(),
		payment.PayAttemptTimeout, paySession, shardTracker,
	)
}

// SendPaymentAsync is the non-blocking version of SendPayment. The payment
// result needs to be retrieved via the control tower.
func (r *ChannelRouter) SendPaymentAsync(payment *LightningPayment,
	ps PaymentSession, st shards.ShardTracker) {

	// Since this is the first time this payment is being made, we pass nil
	// for the existing attempt.
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		log.Tracef("Dispatching SendPayment for lightning payment: %v",
			spewPayment(payment))

		_, _, err := r.sendPayment(
			payment.FeeLimit, payment.Identifier(),
			payment.PayAttemptTimeout, ps, st,
		)
		if err != nil {
			log.Errorf("Payment %x failed: %v",
				payment.Identifier(), err)
		}
	}()
}

// spewPayment returns a log closures that provides a spewed string
// representation of the passed payment.
func spewPayment(payment *LightningPayment) logClosure {
	return newLogClosure(func() string {
		// Make a copy of the payment with a nilled Curve
		// before spewing.
		var routeHints [][]zpay32.HopHint
		for _, routeHint := range payment.RouteHints {
			var hopHints []zpay32.HopHint
			for _, hopHint := range routeHint {
				h := hopHint.Copy()
				hopHints = append(hopHints, h)
			}
			routeHints = append(routeHints, hopHints)
		}
		p := *payment
		p.RouteHints = routeHints
		return spew.Sdump(p)
	})
}

// PreparePayment creates the payment session and registers the payment with the
// control tower.
func (r *ChannelRouter) PreparePayment(payment *LightningPayment) (
	PaymentSession, shards.ShardTracker, error) {

	// Before starting the HTLC routing attempt, we'll create a fresh
	// payment session which will report our errors back to mission
	// control.
	paySession, err := r.cfg.SessionSource.NewPaymentSession(payment)
	if err != nil {
		return nil, nil, err
	}

	// Record this payment hash with the ControlTower, ensuring it is not
	// already in-flight.
	//
	// TODO(roasbeef): store records as part of creation info?
	info := &channeldb.PaymentCreationInfo{
		PaymentIdentifier: payment.Identifier(),
		Value:             payment.Amount,
		CreationTime:      r.cfg.Clock.Now(),
		PaymentRequest:    payment.PaymentRequest,
	}

	// Create a new ShardTracker that we'll use during the life cycle of
	// this payment.
	var shardTracker shards.ShardTracker
	switch {
	// If this is an AMP payment, we'll use the AMP shard tracker.
	case payment.amp != nil:
		shardTracker = amp.NewShardTracker(
			payment.amp.RootShare, payment.amp.SetID,
			*payment.PaymentAddr, payment.Amount,
		)

	// Otherwise we'll use the simple tracker that will map each attempt to
	// the same payment hash.
	default:
		shardTracker = shards.NewSimpleShardTracker(
			payment.Identifier(), nil,
		)
	}

	err = r.cfg.Control.InitPayment(payment.Identifier(), info)
	if err != nil {
		return nil, nil, err
	}

	return paySession, shardTracker, nil
}

// SendToRoute sends a payment using the provided route and fails the payment
// when an error is returned from the attempt.
func (r *ChannelRouter) SendToRoute(htlcHash lntypes.Hash,
	rt *route.Route) (*channeldb.HTLCAttempt, error) {

	return r.sendToRoute(htlcHash, rt, false)
}

// SendToRouteSkipTempErr sends a payment using the provided route and fails
// the payment ONLY when a terminal error is returned from the attempt.
func (r *ChannelRouter) SendToRouteSkipTempErr(htlcHash lntypes.Hash,
	rt *route.Route) (*channeldb.HTLCAttempt, error) {

	return r.sendToRoute(htlcHash, rt, true)
}

// sendToRoute attempts to send a payment with the given hash through the
// provided route. This function is blocking and will return the attempt
// information as it is stored in the database. For a successful htlc, this
// information will contain the preimage. If an error occurs after the attempt
// was initiated, both return values will be non-nil. If skipTempErr is true,
// the payment won't be failed unless a terminal error has occurred.
func (r *ChannelRouter) sendToRoute(htlcHash lntypes.Hash, rt *route.Route,
	skipTempErr bool) (*channeldb.HTLCAttempt, error) {

	// Calculate amount paid to receiver.
	amt := rt.ReceiverAmt()

	// If this is meant as a MP payment shard, we set the amount
	// for the creating info to the total amount of the payment.
	finalHop := rt.Hops[len(rt.Hops)-1]
	mpp := finalHop.MPP
	if mpp != nil {
		amt = mpp.TotalMsat()
	}

	// For non-MPP, there's no such thing as temp error as there's only one
	// HTLC attempt being made. When this HTLC is failed, the payment is
	// failed hence cannot be retried.
	if skipTempErr && mpp == nil {
		return nil, ErrSkipTempErr
	}

	// For non-AMP payments the overall payment identifier will be the same
	// hash as used for this HTLC.
	paymentIdentifier := htlcHash

	// For AMP-payments, we'll use the setID as the unique ID for the
	// overall payment.
	amp := finalHop.AMP
	if amp != nil {
		paymentIdentifier = amp.SetID()
	}

	// Record this payment hash with the ControlTower, ensuring it is not
	// already in-flight.
	info := &channeldb.PaymentCreationInfo{
		PaymentIdentifier: paymentIdentifier,
		Value:             amt,
		CreationTime:      r.cfg.Clock.Now(),
		PaymentRequest:    nil,
	}

	err := r.cfg.Control.InitPayment(paymentIdentifier, info)
	switch {
	// If this is an MPP attempt and the hash is already registered with
	// the database, we can go on to launch the shard.
	case mpp != nil && errors.Is(err, channeldb.ErrPaymentInFlight):
	case mpp != nil && errors.Is(err, channeldb.ErrPaymentExists):

	// Any other error is not tolerated.
	case err != nil:
		return nil, err
	}

	log.Tracef("Dispatching SendToRoute for HTLC hash %v: %v",
		htlcHash, newLogClosure(func() string {
			return spew.Sdump(rt)
		}),
	)

	// Since the HTLC hashes and preimages are specified manually over the
	// RPC for SendToRoute requests, we don't have to worry about creating
	// a ShardTracker that can generate hashes for AMP payments. Instead we
	// create a simple tracker that can just return the hash for the single
	// shard we'll now launch.
	shardTracker := shards.NewSimpleShardTracker(htlcHash, nil)

	// Create a payment lifecycle using the given route with,
	// - zero fee limit as we are not requesting routes.
	// - nil payment session (since we already have a route).
	// - no payment timeout.
	// - no current block height.
	p := newPaymentLifecycle(
		r, 0, paymentIdentifier, nil, shardTracker, 0, 0,
	)

	// We found a route to try, create a new HTLC attempt to try.
	//
	// NOTE: we use zero `remainingAmt` here to simulate the same effect of
	// setting the lastShard to be false, which is used by previous
	// implementation.
	attempt, err := p.registerAttempt(rt, 0)
	if err != nil {
		return nil, err
	}

	// Once the attempt is created, send it to the htlcswitch. Notice that
	// the `err` returned here has already been processed by
	// `handleSwitchErr`, which means if there's a terminal failure, the
	// payment has been failed.
	result, err := p.sendAttempt(attempt)
	if err != nil {
		return nil, err
	}

	// We now lookup the payment to see if it's already failed.
	payment, err := p.router.cfg.Control.FetchPayment(p.identifier)
	if err != nil {
		return result.attempt, err
	}

	// Exit if the above error has caused the payment to be failed, we also
	// return the error from sending attempt to mimic the old behavior of
	// this method.
	_, failedReason := payment.TerminalInfo()
	if failedReason != nil {
		return result.attempt, result.err
	}

	// Since for SendToRoute we won't retry in case the shard fails, we'll
	// mark the payment failed with the control tower immediately if the
	// skipTempErr is false.
	reason := channeldb.FailureReasonError

	// If we failed to send the HTLC, we need to further decide if we want
	// to fail the payment.
	if result.err != nil {
		// If skipTempErr, we'll return the attempt and the temp error.
		if skipTempErr {
			return result.attempt, result.err
		}

		// Otherwise we need to fail the payment.
		err := r.cfg.Control.FailPayment(paymentIdentifier, reason)
		if err != nil {
			return nil, err
		}

		return result.attempt, result.err
	}

	// The attempt was successfully sent, wait for the result to be
	// available.
	result, err = p.collectResult(attempt)
	if err != nil {
		return nil, err
	}

	// We got a successful result.
	if result.err == nil {
		return result.attempt, nil
	}

	// An error returned from collecting the result, we'll mark the payment
	// as failed if we don't skip temp error.
	if !skipTempErr {
		err := r.cfg.Control.FailPayment(paymentIdentifier, reason)
		if err != nil {
			return nil, err
		}
	}

	return result.attempt, result.err
}

// sendPayment attempts to send a payment to the passed payment hash. This
// function is blocking and will return either: when the payment is successful,
// or all candidates routes have been attempted and resulted in a failed
// payment. If the payment succeeds, then a non-nil Route will be returned
// which describes the path the successful payment traversed within the network
// to reach the destination. Additionally, the payment preimage will also be
// returned.
//
// This method relies on the ControlTower's internal payment state machine to
// carry out its execution. After restarts it is safe, and assumed, that the
// router will call this method for every payment still in-flight according to
// the ControlTower.
func (r *ChannelRouter) sendPayment(feeLimit lnwire.MilliSatoshi,
	identifier lntypes.Hash, timeout time.Duration,
	paySession PaymentSession,
	shardTracker shards.ShardTracker) ([32]byte, *route.Route, error) {

	// We'll also fetch the current block height so we can properly
	// calculate the required HTLC time locks within the route.
	_, currentHeight, err := r.cfg.Chain.GetBestBlock()
	if err != nil {
		return [32]byte{}, nil, err
	}

	// Now set up a paymentLifecycle struct with these params, such that we
	// can resume the payment from the current state.
	p := newPaymentLifecycle(
		r, feeLimit, identifier, paySession,
		shardTracker, timeout, currentHeight,
	)

	return p.resumePayment()
}

// extractChannelUpdate examines the error and extracts the channel update.
func (r *ChannelRouter) extractChannelUpdate(
	failure lnwire.FailureMessage) *lnwire.ChannelUpdate {

	var update *lnwire.ChannelUpdate
	switch onionErr := failure.(type) {
	case *lnwire.FailExpiryTooSoon:
		update = &onionErr.Update
	case *lnwire.FailAmountBelowMinimum:
		update = &onionErr.Update
	case *lnwire.FailFeeInsufficient:
		update = &onionErr.Update
	case *lnwire.FailIncorrectCltvExpiry:
		update = &onionErr.Update
	case *lnwire.FailChannelDisabled:
		update = &onionErr.Update
	case *lnwire.FailTemporaryChannelFailure:
		update = onionErr.Update
	}

	return update
}

// applyChannelUpdate validates a channel update and if valid, applies it to the
// database. It returns a bool indicating whether the updates were successful.
func (r *ChannelRouter) applyChannelUpdate(msg *lnwire.ChannelUpdate) bool {
	ch, _, _, err := r.GetChannelByID(msg.ShortChannelID)
	if err != nil {
		log.Errorf("Unable to retrieve channel by id: %v", err)
		return false
	}

	var pubKey *btcec.PublicKey

	switch msg.ChannelFlags & lnwire.ChanUpdateDirection {
	case 0:
		pubKey, _ = ch.NodeKey1()

	case 1:
		pubKey, _ = ch.NodeKey2()
	}

	// Exit early if the pubkey cannot be decided.
	if pubKey == nil {
		log.Errorf("Unable to decide pubkey with ChannelFlags=%v",
			msg.ChannelFlags)
		return false
	}

	err = ValidateChannelUpdateAnn(pubKey, ch.Capacity, msg)
	if err != nil {
		log.Errorf("Unable to validate channel update: %v", err)
		return false
	}

	err = r.UpdateEdge(&models.ChannelEdgePolicy{
		SigBytes:                  msg.Signature.ToSignatureBytes(),
		ChannelID:                 msg.ShortChannelID.ToUint64(),
		LastUpdate:                time.Unix(int64(msg.Timestamp), 0),
		MessageFlags:              msg.MessageFlags,
		ChannelFlags:              msg.ChannelFlags,
		TimeLockDelta:             msg.TimeLockDelta,
		MinHTLC:                   msg.HtlcMinimumMsat,
		MaxHTLC:                   msg.HtlcMaximumMsat,
		FeeBaseMSat:               lnwire.MilliSatoshi(msg.BaseFee),
		FeeProportionalMillionths: lnwire.MilliSatoshi(msg.FeeRate),
		ExtraOpaqueData:           msg.ExtraOpaqueData,
	})
	if err != nil && !IsError(err, ErrIgnored, ErrOutdated) {
		log.Errorf("Unable to apply channel update: %v", err)
		return false
	}

	return true
}

// AddNode is used to add information about a node to the router database. If
// the node with this pubkey is not present in an existing channel, it will
// be ignored.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) AddNode(node *channeldb.LightningNode,
	op ...batch.SchedulerOption) error {

	rMsg := &routingMsg{
		msg: node,
		op:  op,
		err: make(chan error, 1),
	}

	select {
	case r.networkUpdates <- rMsg:
		select {
		case err := <-rMsg.err:
			return err
		case <-r.quit:
			return ErrRouterShuttingDown
		}
	case <-r.quit:
		return ErrRouterShuttingDown
	}
}

// AddEdge is used to add edge/channel to the topology of the router, after all
// information about channel will be gathered this edge/channel might be used
// in construction of payment path.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) AddEdge(edge *models.ChannelEdgeInfo,
	op ...batch.SchedulerOption) error {

	rMsg := &routingMsg{
		msg: edge,
		op:  op,
		err: make(chan error, 1),
	}

	select {
	case r.networkUpdates <- rMsg:
		select {
		case err := <-rMsg.err:
			return err
		case <-r.quit:
			return ErrRouterShuttingDown
		}
	case <-r.quit:
		return ErrRouterShuttingDown
	}
}

// UpdateEdge is used to update edge information, without this message edge
// considered as not fully constructed.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) UpdateEdge(update *models.ChannelEdgePolicy,
	op ...batch.SchedulerOption) error {

	rMsg := &routingMsg{
		msg: update,
		op:  op,
		err: make(chan error, 1),
	}

	select {
	case r.networkUpdates <- rMsg:
		select {
		case err := <-rMsg.err:
			return err
		case <-r.quit:
			return ErrRouterShuttingDown
		}
	case <-r.quit:
		return ErrRouterShuttingDown
	}
}

// CurrentBlockHeight returns the block height from POV of the router subsystem.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) CurrentBlockHeight() (uint32, error) {
	_, height, err := r.cfg.Chain.GetBestBlock()
	return uint32(height), err
}

// GetChannelByID return the channel by the channel id.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) GetChannelByID(chanID lnwire.ShortChannelID) (
	*models.ChannelEdgeInfo,
	*models.ChannelEdgePolicy,
	*models.ChannelEdgePolicy, error) {

	return r.cfg.Graph.FetchChannelEdgesByID(chanID.ToUint64())
}

// FetchLightningNode attempts to look up a target node by its identity public
// key. channeldb.ErrGraphNodeNotFound is returned if the node doesn't exist
// within the graph.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) FetchLightningNode(
	node route.Vertex) (*channeldb.LightningNode, error) {

	return r.cfg.Graph.FetchLightningNode(nil, node)
}

// ForEachNode is used to iterate over every node in router topology.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) ForEachNode(
	cb func(*channeldb.LightningNode) error) error {

	return r.cfg.Graph.ForEachNode(
		func(_ kvdb.RTx, n *channeldb.LightningNode) error {
			return cb(n)
		})
}

// ForAllOutgoingChannels is used to iterate over all outgoing channels owned by
// the router.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) ForAllOutgoingChannels(cb func(kvdb.RTx,
	*models.ChannelEdgeInfo, *models.ChannelEdgePolicy) error) error {

	return r.cfg.Graph.ForEachNodeChannel(nil, r.selfNode.PubKeyBytes,
		func(tx kvdb.RTx, c *models.ChannelEdgeInfo,
			e *models.ChannelEdgePolicy,
			_ *models.ChannelEdgePolicy) error {

			if e == nil {
				return fmt.Errorf("channel from self node " +
					"has no policy")
			}

			return cb(tx, c, e)
		},
	)
}

// AddProof updates the channel edge info with proof which is needed to
// properly announce the edge to the rest of the network.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) AddProof(chanID lnwire.ShortChannelID,
	proof *models.ChannelAuthProof) error {

	info, _, _, err := r.cfg.Graph.FetchChannelEdgesByID(chanID.ToUint64())
	if err != nil {
		return err
	}

	info.AuthProof = proof
	return r.cfg.Graph.UpdateChannelEdge(info)
}

// IsStaleNode returns true if the graph source has a node announcement for the
// target node with a more recent timestamp.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) IsStaleNode(node route.Vertex,
	timestamp time.Time) bool {

	// If our attempt to assert that the node announcement is fresh fails,
	// then we know that this is actually a stale announcement.
	err := r.assertNodeAnnFreshness(node, timestamp)
	if err != nil {
		log.Debugf("Checking stale node %x got %v", node, err)
		return true
	}

	return false
}

// IsPublicNode determines whether the given vertex is seen as a public node in
// the graph from the graph's source node's point of view.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) IsPublicNode(node route.Vertex) (bool, error) {
	return r.cfg.Graph.IsPublicNode(node)
}

// IsKnownEdge returns true if the graph source already knows of the passed
// channel ID either as a live or zombie edge.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) IsKnownEdge(chanID lnwire.ShortChannelID) bool {
	_, _, exists, isZombie, _ := r.cfg.Graph.HasChannelEdge(
		chanID.ToUint64(),
	)
	return exists || isZombie
}

// MarkEdgeLive clears an edge from our zombie index, deeming it as live.
//
// NOTE: This method is part of the ChannelGraphSource interface.
func (r *ChannelRouter) MarkEdgeLive(chanID lnwire.ShortChannelID) error {
	return r.cfg.Graph.MarkEdgeLive(chanID.ToUint64())
}

// ErrNoChannel is returned when a route cannot be built because there are no
// channels that satisfy all requirements.
type ErrNoChannel struct {
	position int
	fromNode route.Vertex
}

// Error returns a human readable string describing the error.
func (e ErrNoChannel) Error() string {
	return fmt.Sprintf("no matching outgoing channel available for "+
		"node %v (%v)", e.position, e.fromNode)
}

// BuildRoute returns a fully specified route based on a list of pubkeys. If
// amount is nil, the minimum routable amount is used. To force a specific
// outgoing channel, use the outgoingChan parameter.
func (r *ChannelRouter) BuildRoute(amt *lnwire.MilliSatoshi,
	hops []route.Vertex, outgoingChan *uint64,
	finalCltvDelta int32, payAddr *[32]byte) (*route.Route, error) {

	log.Tracef("BuildRoute called: hopsCount=%v, amt=%v",
		len(hops), amt)

	var outgoingChans map[uint64]struct{}
	if outgoingChan != nil {
		outgoingChans = map[uint64]struct{}{
			*outgoingChan: {},
		}
	}

	// If no amount is specified, we need to build a route for the minimum
	// amount that this route can carry.
	useMinAmt := amt == nil

	var runningAmt lnwire.MilliSatoshi
	if useMinAmt {
		// For minimum amount routes, aim to deliver at least 1 msat to
		// the destination. There are nodes in the wild that have a
		// min_htlc channel policy of zero, which could lead to a zero
		// amount payment being made.
		runningAmt = 1
	} else {
		// If an amount is specified, we need to build a route that
		// delivers exactly this amount to the final destination.
		runningAmt = *amt
	}

	// We'll attempt to obtain a set of bandwidth hints that helps us select
	// the best outgoing channel to use in case no outgoing channel is set.
	bandwidthHints, err := newBandwidthManager(
		r.cfg.RoutingGraph, r.selfNode.PubKeyBytes, r.cfg.GetLink,
	)
	if err != nil {
		return nil, err
	}

	// Fetch the current block height outside the routing transaction, to
	// prevent the rpc call blocking the database.
	_, height, err := r.cfg.Chain.GetBestBlock()
	if err != nil {
		return nil, err
	}

	sourceNode := r.selfNode.PubKeyBytes
	unifiers, senderAmt, err := getRouteUnifiers(
		sourceNode, hops, useMinAmt, runningAmt, outgoingChans,
		r.cfg.RoutingGraph, bandwidthHints,
	)
	if err != nil {
		return nil, err
	}

	pathEdges, receiverAmt, err := getPathEdges(
		sourceNode, senderAmt, unifiers, bandwidthHints, hops,
	)
	if err != nil {
		return nil, err
	}

	// Build and return the final route.
	return newRoute(
		sourceNode, pathEdges, uint32(height),
		finalHopParams{
			amt:         receiverAmt,
			totalAmt:    receiverAmt,
			cltvDelta:   uint16(finalCltvDelta),
			records:     nil,
			paymentAddr: payAddr,
		}, nil,
	)
}

// getRouteUnifiers returns a list of edge unifiers for the given route.
func getRouteUnifiers(source route.Vertex, hops []route.Vertex,
	useMinAmt bool, runningAmt lnwire.MilliSatoshi,
	outgoingChans map[uint64]struct{}, graph RoutingGraph,
	bandwidthHints *bandwidthManager) ([]*edgeUnifier, lnwire.MilliSatoshi,
	error) {

	// Allocate a list that will contain the edge unifiers for this route.
	unifiers := make([]*edgeUnifier, len(hops))

	// Traverse hops backwards to accumulate fees in the running amounts.
	for i := len(hops) - 1; i >= 0; i-- {
		toNode := hops[i]

		var fromNode route.Vertex
		if i == 0 {
			fromNode = source
		} else {
			fromNode = hops[i-1]
		}

		localChan := i == 0

		// Build unified policies for this hop based on the channels
		// known in the graph. Don't use inbound fees.
		//
		// TODO: Add inbound fees support for BuildRoute.
		u := newNodeEdgeUnifier(
			source, toNode, false, outgoingChans,
		)

		err := u.addGraphPolicies(graph)
		if err != nil {
			return nil, 0, err
		}

		// Exit if there are no channels.
		edgeUnifier, ok := u.edgeUnifiers[fromNode]
		if !ok {
			log.Errorf("Cannot find policy for node %v", fromNode)
			return nil, 0, ErrNoChannel{
				fromNode: fromNode,
				position: i,
			}
		}

		// If using min amt, increase amt if needed.
		if useMinAmt {
			min := edgeUnifier.minAmt()
			if min > runningAmt {
				runningAmt = min
			}
		}

		// Get an edge for the specific amount that we want to forward.
		edge := edgeUnifier.getEdge(runningAmt, bandwidthHints, 0)
		if edge == nil {
			log.Errorf("Cannot find policy with amt=%v for node %v",
				runningAmt, fromNode)

			return nil, 0, ErrNoChannel{
				fromNode: fromNode,
				position: i,
			}
		}

		// Add fee for this hop.
		if !localChan {
			runningAmt += edge.policy.ComputeFee(runningAmt)
		}

		log.Tracef("Select channel %v at position %v",
			edge.policy.ChannelID, i)

		unifiers[i] = edgeUnifier
	}

	return unifiers, runningAmt, nil
}

// getPathEdges returns the edges that make up the path and the total amount,
// including fees, to send the payment.
func getPathEdges(source route.Vertex, receiverAmt lnwire.MilliSatoshi,
	unifiers []*edgeUnifier, bandwidthHints *bandwidthManager,
	hops []route.Vertex) ([]*unifiedEdge,
	lnwire.MilliSatoshi, error) {

	// Now that we arrived at the start of the route and found out the route
	// total amount, we make a forward pass. Because the amount may have
	// been increased in the backward pass, fees need to be recalculated and
	// amount ranges re-checked.
	var pathEdges []*unifiedEdge
	for i, unifier := range unifiers {
		edge := unifier.getEdge(receiverAmt, bandwidthHints, 0)
		if edge == nil {
			fromNode := source
			if i > 0 {
				fromNode = hops[i-1]
			}

			return nil, 0, ErrNoChannel{
				fromNode: fromNode,
				position: i,
			}
		}

		if i > 0 {
			// Decrease the amount to send while going forward.
			receiverAmt -= edge.policy.ComputeFeeFromIncoming(
				receiverAmt,
			)
		}

		pathEdges = append(pathEdges, edge)
	}

	return pathEdges, receiverAmt, nil
}
