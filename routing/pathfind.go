package routing

import (
	"bytes"
	"container/heap"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/feature"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing/route"
)

const (
	// infinity is used as a starting distance in our shortest path search.
	infinity = math.MaxInt64

	// RiskFactorBillionths controls the influence of time lock delta
	// of a channel on route selection. It is expressed as billionths
	// of msat per msat sent through the channel per time lock delta
	// block. See edgeWeight function below for more details.
	// The chosen value is based on the previous incorrect weight function
	// 1 + timelock + fee * fee. In this function, the fee penalty
	// diminishes the time lock penalty for all but the smallest amounts.
	// To not change the behaviour of path finding too drastically, a
	// relatively small value is chosen which is still big enough to give
	// some effect with smaller time lock values. The value may need
	// tweaking and/or be made configurable in the future.
	RiskFactorBillionths = 15

	// estimatedNodeCount is used to preallocate the path finding structures
	// to avoid resizing and copies. It should be number on the same order as
	// the number of active nodes in the network.
	estimatedNodeCount = 10000

	// fakeHopHintCapacity is the capacity we assume for hop hint channels.
	// This is a high number, which expresses that a hop hint channel should
	// be able to route payments.
	fakeHopHintCapacity = btcutil.Amount(10 * btcutil.SatoshiPerBitcoin)
)

// pathFinder defines the interface of a path finding algorithm.
type pathFinder = func(g *graphParams, r *RestrictParams,
	cfg *PathFindingConfig, source, target route.Vertex,
	amt lnwire.MilliSatoshi, timePref float64, finalHtlcExpiry int32) (
	[]*unifiedEdge, float64, error)

var (
	// DefaultEstimator is the default estimator used for computing
	// probabilities in pathfinding.
	DefaultEstimator = AprioriEstimatorName

	// DefaultAttemptCost is the default fixed virtual cost in path finding
	// of a failed payment attempt. It is used to trade off potentially
	// better routes against their probability of succeeding.
	DefaultAttemptCost = lnwire.NewMSatFromSatoshis(100)

	// DefaultAttemptCostPPM is the default proportional virtual cost in
	// path finding weight units of executing a payment attempt that fails.
	// It is used to trade off potentially better routes against their
	// probability of succeeding. This parameter is expressed in parts per
	// million of the payment amount.
	//
	// It is impossible to pick a perfect default value. The current value
	// of 0.1% is based on the idea that a transaction fee of 1% is within
	// reasonable territory and that a payment shouldn't need more than 10
	// attempts.
	DefaultAttemptCostPPM = int64(1000)

	// DefaultMinRouteProbability is the default minimum probability for routes
	// returned from findPath.
	DefaultMinRouteProbability = float64(0.01)

	// DefaultAprioriHopProbability is the default a priori probability for
	// a hop.
	DefaultAprioriHopProbability = float64(0.6)
)

// edgePolicyWithSource is a helper struct to keep track of the source node
// of a channel edge. ChannelEdgePolicy only contains to destination node
// of the edge.
type edgePolicyWithSource struct {
	sourceNode route.Vertex
	edge       AdditionalEdge
}

// finalHopParams encapsulates various parameters for route construction that
// apply to the final hop in a route. These features include basic payment data
// such as amounts and cltvs, as well as more complex features like destination
// custom records and payment address.
type finalHopParams struct {
	amt         lnwire.MilliSatoshi
	totalAmt    lnwire.MilliSatoshi
	cltvDelta   uint16
	records     record.CustomSet
	paymentAddr *[32]byte

	// metadata is additional data that is sent along with the payment to
	// the payee.
	metadata []byte
}

// newRoute constructs a route using the provided path and final hop constraints.
// Any destination specific fields from the final hop params  will be attached
// assuming the destination's feature vector signals support, otherwise this
// method will fail.  If the route is too long, or the selected path cannot
// support the fully payment including fees, then a non-nil error is returned.
// If the route is to a blinded path, the blindedPath parameter is used to
// back fill additional fields that are required for a blinded payment. This is
// done in a separate pass to keep our route construction simple, as blinded
// paths require zero expiry and amount values for intermediate hops (which
// makes calculating the totals during route construction difficult if we
// include blinded paths on the first pass).
//
// NOTE: The passed slice of unified edges MUST be sorted in forward order: from
// the source to the target node of the path finding attempt. It is assumed that
// any feature vectors on all hops have been validated for transitive
// dependencies.
// NOTE: If a non-nil blinded path is provided it is assumed to have been
// validated by the caller.
func newRoute(sourceVertex route.Vertex,
	pathEdges []*unifiedEdge, currentHeight uint32,
	finalHop finalHopParams, blindedPath *sphinx.BlindedPath) (
	*route.Route, error) {

	var (
		hops []*route.Hop

		// totalTimeLock will accumulate the cumulative time lock
		// across the entire route. This value represents how long the
		// sender will need to wait in the *worst* case.
		totalTimeLock = currentHeight

		// nextIncomingAmount is the amount that will need to flow into
		// the *next* hop. Since we're going to be walking the route
		// backwards below, this next hop gets closer and closer to the
		// sender of the payment.
		nextIncomingAmount lnwire.MilliSatoshi
	)

	pathLength := len(pathEdges)
	for i := pathLength - 1; i >= 0; i-- {
		// Now we'll start to calculate the items within the per-hop
		// payload for the hop this edge is leading to.
		edge := pathEdges[i].policy

		// We'll calculate the amounts, timelocks, and fees for each hop
		// in the route. The base case is the final hop which includes
		// their amount and timelocks. These values will accumulate
		// contributions from the preceding hops back to the sender as
		// we compute the route in reverse.
		var (
			amtToForward        lnwire.MilliSatoshi
			fee                 int64
			totalAmtMsatBlinded lnwire.MilliSatoshi
			outgoingTimeLock    uint32
			tlvPayload          bool
			customRecords       record.CustomSet
			mpp                 *record.MPP
			metadata            []byte
		)

		// isTLVOptionBit returns true if the given feature bit is
		// either one of the required or optional TLV Onion feature
		// bits.
		isTLVOptionBit := func(feature lnwire.FeatureBit) bool {
			switch feature {
			case lnwire.TLVOnionPayloadRequired,
				lnwire.TLVOnionPayloadOptional:

				return true
			default:
				return false
			}
		}

		// Define a helper function that checks this edge's feature
		// vector for support for a given feature. We assume at this
		// point that the feature vectors transitive dependencies have
		// been validated.
		supports := func(feature lnwire.FeatureBit) bool {
			// Since the `option_route_blinding` feature bit depends
			// on `var_onion_option`, we can assume that all nodes
			// in a blinded path support TLV onions without this
			// being explicitly communicated to us.
			if edge.IsBlindedEdge && isTLVOptionBit(feature) {
				return true
			}

			// If this edge comes from router hints, the features
			// could be nil.
			if edge.ToNodeFeatures == nil {
				return false
			}
			return edge.ToNodeFeatures.HasFeature(feature)
		}

		// We start by assuming the node doesn't support TLV. We'll now
		// inspect the node's feature vector to see if we can promote
		// the hop. We assume already that the feature vector's
		// transitive dependencies have already been validated by path
		// finding or some other means.
		tlvPayload = supports(lnwire.TLVOnionPayloadOptional)

		if i == len(pathEdges)-1 {
			// If this is the last hop, then the hop payload will
			// contain the exact amount. In BOLT #4: Onion Routing
			// Protocol / "Payload for the Last Node", this is
			// detailed.
			amtToForward = finalHop.amt

			// Fee is not part of the hop payload, but only used for
			// reporting through RPC. Set to zero for the final hop.
			fee = 0

			// As this is the last hop, we'll use the specified
			// final CLTV delta value instead of the value from the
			// last link in the route.
			totalTimeLock += uint32(finalHop.cltvDelta)
			outgoingTimeLock = totalTimeLock

			// Attach any custom records to the final hop if the
			// receiver supports TLV.
			if !tlvPayload && finalHop.records != nil {
				return nil, errors.New("cannot attach " +
					"custom records")
			}
			customRecords = finalHop.records

			// If we're attaching a payment addr but the receiver
			// doesn't support both TLV and payment addrs, fail.
			payAddr := supports(lnwire.PaymentAddrOptional)
			if !payAddr && finalHop.paymentAddr != nil {
				return nil, errors.New("cannot attach " +
					"payment addr")
			}

			// Otherwise attach the mpp record if it exists.
			// TODO(halseth): move this to payment life cycle,
			// where AMP options are set.
			if finalHop.paymentAddr != nil {
				mpp = record.NewMPP(
					finalHop.totalAmt,
					*finalHop.paymentAddr,
				)
			}

			metadata = finalHop.metadata

			if blindedPath != nil {
				totalAmtMsatBlinded = finalHop.totalAmt
			}
		} else {
			// The amount that the current hop needs to forward is
			// equal to the incoming amount of the next hop.
			amtToForward = nextIncomingAmount

			// The fee that needs to be paid to the current hop is
			// based on the amount that this hop needs to forward
			// and its policy for the outgoing channel. This policy
			// is stored as part of the incoming channel of
			// the next hop.
			outboundFee := pathEdges[i+1].policy.ComputeFee(
				amtToForward,
			)

			inboundFee := pathEdges[i].inboundFees.CalcFee(
				amtToForward + outboundFee,
			)

			fee = int64(outboundFee) + inboundFee
			if fee < 0 {
				fee = 0
			}

			// We'll take the total timelock of the preceding hop as
			// the outgoing timelock or this hop. Then we'll
			// increment the total timelock incurred by this hop.
			outgoingTimeLock = totalTimeLock
			totalTimeLock += uint32(
				pathEdges[i+1].policy.TimeLockDelta,
			)
		}

		// Since we're traversing the path backwards atm, we prepend
		// each new hop such that, the final slice of hops will be in
		// the forwards order.
		currentHop := &route.Hop{
			PubKeyBytes:      edge.ToNodePubKey(),
			ChannelID:        edge.ChannelID,
			AmtToForward:     amtToForward,
			OutgoingTimeLock: outgoingTimeLock,
			LegacyPayload:    !tlvPayload,
			CustomRecords:    customRecords,
			MPP:              mpp,
			Metadata:         metadata,
			TotalAmtMsat:     totalAmtMsatBlinded,
		}

		hops = append([]*route.Hop{currentHop}, hops...)

		// Finally, we update the amount that needs to flow into the
		// *next* hop, which is the amount this hop needs to forward,
		// accounting for the fee that it takes.
		nextIncomingAmount = amtToForward + lnwire.MilliSatoshi(fee)
	}

	// If we are creating a route to a blinded path, we need to add some
	// additional data to the route that is required for blinded forwarding.
	// We do another pass on our edges to append this data.
	if blindedPath != nil {
		var (
			inBlindedRoute bool
			dataIndex      = 0

			introVertex = route.NewVertex(
				blindedPath.IntroductionPoint,
			)
		)

		for i, hop := range hops {
			// Once we locate our introduction node, we know that
			// every hop after this is part of the blinded route.
			if bytes.Equal(hop.PubKeyBytes[:], introVertex[:]) {
				inBlindedRoute = true
				hop.BlindingPoint = blindedPath.BlindingPoint
			}

			// We don't need to modify edges outside of our blinded
			// route.
			if !inBlindedRoute {
				continue
			}

			payload := blindedPath.BlindedHops[dataIndex].CipherText
			hop.EncryptedData = payload

			// All of the hops in a blinded route *except* the
			// final hop should have zero amounts / time locks.
			if i != len(hops)-1 {
				hop.AmtToForward = 0
				hop.OutgoingTimeLock = 0
			}

			dataIndex++
		}
	}

	// With the base routing data expressed as hops, build the full route
	newRoute, err := route.NewRouteFromHops(
		nextIncomingAmount, totalTimeLock, route.Vertex(sourceVertex),
		hops,
	)
	if err != nil {
		return nil, err
	}

	return newRoute, nil
}

// edgeWeight computes the weight of an edge. This value is used when searching
// for the shortest path within the channel graph between two nodes. Weight is
// is the fee itself plus a time lock penalty added to it. This benefits
// channels with shorter time lock deltas and shorter (hops) routes in general.
// RiskFactor controls the influence of time lock on route selection. This is
// currently a fixed value, but might be configurable in the future.
func edgeWeight(lockedAmt lnwire.MilliSatoshi, fee lnwire.MilliSatoshi,
	timeLockDelta uint16) int64 {
	// timeLockPenalty is the penalty for the time lock delta of this channel.
	// It is controlled by RiskFactorBillionths and scales proportional
	// to the amount that will pass through channel. Rationale is that it if
	// a twice as large amount gets locked up, it is twice as bad.
	timeLockPenalty := int64(lockedAmt) * int64(timeLockDelta) *
		RiskFactorBillionths / 1000000000

	return int64(fee) + timeLockPenalty
}

// graphParams wraps the set of graph parameters passed to findPath.
type graphParams struct {
	// graph is the ChannelGraph to be used during path finding.
	graph routingGraph

	// additionalEdges is an optional set of edges that should be
	// considered during path finding, that is not already found in the
	// channel graph. These can either be private edges for bolt 11 invoices
	// or blinded edges when a payment to a blinded path is made.
	additionalEdges map[route.Vertex][]AdditionalEdge

	// bandwidthHints is an interface that provides bandwidth hints that
	// can provide a better estimate of the current channel bandwidth than
	// what is found in the graph. It will override the capacities and
	// disabled flags found in the graph for local channels when doing
	// path finding if it has updated values for that channel. In
	// particular, it should be set to the current available sending
	// bandwidth for active local channels, and 0 for inactive channels.
	bandwidthHints bandwidthHints
}

// RestrictParams wraps the set of restrictions passed to findPath that the
// found path must adhere to.
type RestrictParams struct {
	// ProbabilitySource is a callback that is expected to return the
	// success probability of traversing the channel from the node.
	ProbabilitySource func(route.Vertex, route.Vertex,
		lnwire.MilliSatoshi, btcutil.Amount) float64

	// FeeLimit is a maximum fee amount allowed to be used on the path from
	// the source to the target.
	FeeLimit lnwire.MilliSatoshi

	// OutgoingChannelIDs is the list of channels that are allowed for the
	// first hop. If nil, any channel may be used.
	OutgoingChannelIDs []uint64

	// LastHop is the pubkey of the last node before the final destination
	// is reached. If nil, any node may be used.
	LastHop *route.Vertex

	// CltvLimit is the maximum time lock of the route excluding the final
	// ctlv. After path finding is complete, the caller needs to increase
	// all cltv expiry heights with the required final cltv delta.
	CltvLimit uint32

	// DestCustomRecords contains the custom records to drop off at the
	// final hop, if any.
	DestCustomRecords record.CustomSet

	// DestFeatures is a feature vector describing what the final hop
	// supports. If none are provided, pathfinding will try to inspect any
	// features on the node announcement instead.
	DestFeatures *lnwire.FeatureVector

	// PaymentAddr is a random 32-byte value generated by the receiver to
	// mitigate probing vectors and payment sniping attacks on overpaid
	// invoices.
	PaymentAddr *[32]byte

	// Amp signals to the pathfinder that this payment is an AMP payment
	// and therefore it needs to account for additional AMP data in the
	// final hop payload size calculation.
	Amp *AMPOptions

	// Metadata is additional data that is sent along with the payment to
	// the payee.
	Metadata []byte

	// BlindedPayment is necessary to determine the hop size of the
	// last/exit hop.
	BlindedPayment *BlindedPayment
}

// PathFindingConfig defines global parameters that control the trade-off in
// path finding between fees and probability.
type PathFindingConfig struct {
	// AttemptCost is the fixed virtual cost in path finding of a failed
	// payment attempt. It is used to trade off potentially better routes
	// against their probability of succeeding.
	AttemptCost lnwire.MilliSatoshi

	// AttemptCostPPM is the proportional virtual cost in path finding of a
	// failed payment attempt. It is used to trade off potentially better
	// routes against their probability of succeeding. This parameter is
	// expressed in parts per million of the total payment amount.
	AttemptCostPPM int64

	// MinProbability defines the minimum success probability of the
	// returned route.
	MinProbability float64
}

// getOutgoingBalance returns the maximum available balance in any of the
// channels of the given node. The second return parameters is the total
// available balance.
func getOutgoingBalance(node route.Vertex, outgoingChans map[uint64]struct{},
	bandwidthHints bandwidthHints,
	g routingGraph) (lnwire.MilliSatoshi, lnwire.MilliSatoshi, error) {

	var max, total lnwire.MilliSatoshi
	cb := func(channel *channeldb.DirectedChannel) error {
		if !channel.OutPolicySet {
			return nil
		}

		chanID := channel.ChannelID

		// Enforce outgoing channel restriction.
		if outgoingChans != nil {
			if _, ok := outgoingChans[chanID]; !ok {
				return nil
			}
		}

		bandwidth, ok := bandwidthHints.availableChanBandwidth(
			chanID, 0,
		)

		// If the bandwidth is not available, use the channel capacity.
		// This can happen when a channel is added to the graph after
		// we've already queried the bandwidth hints.
		if !ok {
			bandwidth = lnwire.NewMSatFromSatoshis(channel.Capacity)
		}

		if bandwidth > max {
			max = bandwidth
		}

		total += bandwidth

		return nil
	}

	// Iterate over all channels of the to node.
	err := g.forEachNodeChannel(node, cb)
	if err != nil {
		return 0, 0, err
	}
	return max, total, err
}

// findPath attempts to find a path from the source node within the ChannelGraph
// to the target node that's capable of supporting a payment of `amt` value. The
// current approach implemented is modified version of Dijkstra's algorithm to
// find a single shortest path between the source node and the destination. The
// distance metric used for edges is related to the time-lock+fee costs along a
// particular edge. If a path is found, this function returns a slice of
// ChannelHop structs which encoded the chosen path from the target to the
// source. The search is performed backwards from destination node back to
// source. This is to properly accumulate fees that need to be paid along the
// path and accurately check the amount to forward at every node against the
// available bandwidth.
func findPath(g *graphParams, r *RestrictParams, cfg *PathFindingConfig,
	source, target route.Vertex, amt lnwire.MilliSatoshi, timePref float64,
	finalHtlcExpiry int32) ([]*unifiedEdge, float64, error) {

	// Pathfinding can be a significant portion of the total payment
	// latency, especially on low-powered devices. Log several metrics to
	// aid in the analysis performance problems in this area.
	start := time.Now()
	nodesVisited := 0
	edgesExpanded := 0
	defer func() {
		timeElapsed := time.Since(start)
		log.Debugf("Pathfinding perf metrics: nodes=%v, edges=%v, "+
			"time=%v", nodesVisited, edgesExpanded, timeElapsed)
	}()

	// If no destination features are provided, we will load what features
	// we have for the target node from our graph.
	features := r.DestFeatures
	if features == nil {
		var err error
		features, err = g.graph.fetchNodeFeatures(target)
		if err != nil {
			return nil, 0, err
		}
	}

	// Ensure that the destination's features don't include unknown
	// required features.
	err := feature.ValidateRequired(features)
	if err != nil {
		log.Warnf("Pathfinding destination node features: %v", err)
		return nil, 0, errUnknownRequiredFeature
	}

	// Ensure that all transitive dependencies are set.
	err = feature.ValidateDeps(features)
	if err != nil {
		log.Warnf("Pathfinding destination node features: %v", err)
		return nil, 0, errMissingDependentFeature
	}

	// Now that we know the feature vector is well formed, we'll proceed in
	// checking that it supports the features we need, given our
	// restrictions on the final hop.

	// If the caller needs to send custom records, check that our
	// destination feature vector supports TLV.
	if len(r.DestCustomRecords) > 0 &&
		!features.HasFeature(lnwire.TLVOnionPayloadOptional) {

		return nil, 0, errNoTlvPayload
	}

	// If the caller has a payment address to attach, check that our
	// destination feature vector supports them.
	if r.PaymentAddr != nil &&
		!features.HasFeature(lnwire.PaymentAddrOptional) {

		return nil, 0, errNoPaymentAddr
	}

	// If the caller needs to send custom records, check that our
	// destination feature vector supports TLV.
	if r.Metadata != nil &&
		!features.HasFeature(lnwire.TLVOnionPayloadOptional) {

		return nil, 0, errNoTlvPayload
	}

	// Set up outgoing channel map for quicker access.
	var outgoingChanMap map[uint64]struct{}
	if len(r.OutgoingChannelIDs) > 0 {
		outgoingChanMap = make(map[uint64]struct{})
		for _, outChan := range r.OutgoingChannelIDs {
			outgoingChanMap[outChan] = struct{}{}
		}
	}

	// If we are routing from ourselves, check that we have enough local
	// balance available.
	self := g.graph.sourceNode()

	if source == self {
		max, total, err := getOutgoingBalance(
			self, outgoingChanMap, g.bandwidthHints, g.graph,
		)
		if err != nil {
			return nil, 0, err
		}

		// If the total outgoing balance isn't sufficient, it will be
		// impossible to complete the payment.
		if total < amt {
			log.Warnf("Not enough outbound balance to send "+
				"htlc of amount: %v, only have local "+
				"balance: %v", amt, total)

			return nil, 0, errInsufficientBalance
		}

		// If there is only not enough capacity on a single route, it
		// may still be possible to complete the payment by splitting.
		if max < amt {
			return nil, 0, errNoPathFound
		}
	}

	// First we'll initialize an empty heap which'll help us to quickly
	// locate the next edge we should visit next during our graph
	// traversal.
	nodeHeap := newDistanceHeap(estimatedNodeCount)

	// Holds the current best distance for a given node.
	distance := make(map[route.Vertex]*nodeWithDist, estimatedNodeCount)

	additionalEdgesWithSrc := make(map[route.Vertex][]*edgePolicyWithSource)
	for vertex, additionalEdges := range g.additionalEdges {
		// Edges connected to self are always included in the graph,
		// therefore can be skipped. This prevents us from trying
		// routes to malformed hop hints.
		if vertex == self {
			continue
		}

		// Build reverse lookup to find incoming edges. Needed because
		// search is taken place from target to source.
		for _, additionalEdge := range additionalEdges {
			outgoingEdgePolicy := additionalEdge.EdgePolicy()
			toVertex := outgoingEdgePolicy.ToNodePubKey()

			incomingEdgePolicy := &edgePolicyWithSource{
				sourceNode: vertex,
				edge:       additionalEdge,
			}

			additionalEdgesWithSrc[toVertex] =
				append(additionalEdgesWithSrc[toVertex],
					incomingEdgePolicy)
		}
	}

	// The payload size of the final hop differ from intermediate hops
	// and depends on whether the destination is blinded or not.
	lastHopPayloadSize := lastHopPayloadSize(r, finalHtlcExpiry, amt,
		!features.HasFeature(lnwire.TLVOnionPayloadOptional))

	// We can't always assume that the end destination is publicly
	// advertised to the network so we'll manually include the target node.
	// The target node charges no fee. Distance is set to 0, because this is
	// the starting point of the graph traversal. We are searching backwards
	// to get the fees first time right and correctly match channel
	// bandwidth.
	//
	// Don't record the initial partial path in the distance map and reserve
	// that key for the source key in the case we route to ourselves.
	partialPath := &nodeWithDist{
		dist:              0,
		weight:            0,
		node:              target,
		netAmountReceived: amt,
		incomingCltv:      finalHtlcExpiry,
		probability:       1,
		routingInfoSize:   lastHopPayloadSize,
	}

	// Calculate the absolute cltv limit. Use uint64 to prevent an overflow
	// if the cltv limit is MaxUint32.
	absoluteCltvLimit := uint64(r.CltvLimit) + uint64(finalHtlcExpiry)

	// Calculate the default attempt cost as configured globally.
	defaultAttemptCost := float64(
		cfg.AttemptCost +
			amt*lnwire.MilliSatoshi(cfg.AttemptCostPPM)/1000000,
	)

	// Validate time preference value.
	if math.Abs(timePref) > 1 {
		return nil, 0, fmt.Errorf("time preference %v out of range "+
			"[-1, 1]", timePref)
	}

	// Scale to avoid the extremes -1 and 1 which run into infinity issues.
	timePref *= 0.9

	// Apply time preference. At 0, the default attempt cost will
	// be used.
	absoluteAttemptCost := defaultAttemptCost * (1/(0.5-timePref/2) - 1)

	log.Debugf("Pathfinding absolute attempt cost: %v sats",
		absoluteAttemptCost/1000)

	// processEdge is a helper closure that will be used to make sure edges
	// satisfy our specific requirements.
	processEdge := func(fromVertex route.Vertex,
		fromFeatures *lnwire.FeatureVector,
		edge *unifiedEdge, toNodeDist *nodeWithDist) {

		edgesExpanded++

		// Calculate inbound fee charged by "to" node. The exit hop
		// doesn't charge inbound fees. If the "to" node is the exit
		// hop, its inbound fees have already been set to zero by
		// nodeEdgeUnifier.
		inboundFee := edge.inboundFees.CalcFee(
			toNodeDist.netAmountReceived,
		)

		// Make sure that the node total fee is never negative.
		// Routing nodes treat a total fee that turns out
		// negative as a zero fee and pathfinding should do the
		// same.
		minInboundFee := -int64(toNodeDist.outboundFee)
		if inboundFee < minInboundFee {
			inboundFee = minInboundFee
		}

		// Calculate amount that the candidate node would have to send
		// out.
		amountToSend := toNodeDist.netAmountReceived +
			lnwire.MilliSatoshi(inboundFee)

		// Request the success probability for this edge.
		edgeProbability := r.ProbabilitySource(
			fromVertex, toNodeDist.node, amountToSend,
			edge.capacity,
		)

		log.Trace(newLogClosure(func() string {
			return fmt.Sprintf("path finding probability: fromnode=%v,"+
				" tonode=%v, amt=%v, cap=%v, probability=%v",
				fromVertex, toNodeDist.node, amountToSend,
				edge.capacity, edgeProbability)
		}))

		// If the probability is zero, there is no point in trying.
		if edgeProbability == 0 {
			return
		}

		// Compute fee that fromVertex is charging. It is based on the
		// amount that needs to be sent to the next node in the route.
		//
		// Source node has no predecessor to pay a fee. Therefore set
		// fee to zero, because it should not be included in the fee
		// limit check and edge weight.
		//
		// Also determine the time lock delta that will be added to the
		// route if fromVertex is selected. If fromVertex is the source
		// node, no additional timelock is required.
		var (
			timeLockDelta uint16
			outboundFee   int64
		)

		if fromVertex != source {
			outboundFee = int64(
				edge.policy.ComputeFee(amountToSend),
			)
			timeLockDelta = edge.policy.TimeLockDelta
		}

		incomingCltv := toNodeDist.incomingCltv + int32(timeLockDelta)

		// Check that we are within our CLTV limit.
		if uint64(incomingCltv) > absoluteCltvLimit {
			return
		}

		// netAmountToReceive is the amount that the node that is added
		// to the distance map needs to receive from a (to be found)
		// previous node in the route. The inbound fee of the receiving
		// node is already subtracted from this value. The previous node
		// will need to pay the amount that this node forwards plus the
		// fee it charges plus this node's inbound fee.
		netAmountToReceive := amountToSend +
			lnwire.MilliSatoshi(outboundFee)

		// Check if accumulated fees would exceed fee limit when this
		// node would be added to the path.
		totalFee := int64(netAmountToReceive) - int64(amt)
		if totalFee > 0 && lnwire.MilliSatoshi(totalFee) > r.FeeLimit {
			return
		}

		// Calculate total probability of successfully reaching target
		// by multiplying the probabilities. Both this edge and the rest
		// of the route must succeed.
		probability := toNodeDist.probability * edgeProbability

		// If the probability is below the specified lower bound, we can
		// abandon this direction. Adding further nodes can only lower
		// the probability more.
		if probability < cfg.MinProbability {
			return
		}

		// Calculate the combined fee for this edge. Dijkstra does not
		// support negative edge weights. Because this fee feeds into
		// the edge weight calculation, we don't allow it to be
		// negative.
		signedFee := inboundFee + outboundFee
		fee := lnwire.MilliSatoshi(0)
		if signedFee > 0 {
			fee = lnwire.MilliSatoshi(signedFee)
		}

		// By adding fromVertex in the route, there will be an extra
		// weight composed of the fee that this node will charge and
		// the amount that will be locked for timeLockDelta blocks in
		// the HTLC that is handed out to fromVertex.
		weight := edgeWeight(netAmountToReceive, fee, timeLockDelta)

		// Compute the tentative weight to this new channel/edge
		// which is the weight from our toNode to the target node
		// plus the weight of this edge.
		tempWeight := toNodeDist.weight + weight

		// Add an extra factor to the weight to take into account the
		// probability. Another reason why we rounded the fee up to zero
		// is to prevent a highly negative fee from cancelling out the
		// extra factor. We don't want an always-failing node to attract
		// traffic using a highly negative fee and escape penalization.
		tempDist := getProbabilityBasedDist(
			tempWeight, probability,
			absoluteAttemptCost,
		)

		// If there is already a best route stored, compare this
		// candidate route with the best route so far.
		current, ok := distance[fromVertex]
		if ok {
			// If this route is worse than what we already found,
			// skip this route.
			if tempDist > current.dist {
				return
			}

			// If the route is equally good and the probability
			// isn't better, skip this route. It is important to
			// also return if both cost and probability are equal,
			// because otherwise the algorithm could run into an
			// endless loop.
			probNotBetter := probability <= current.probability
			if tempDist == current.dist && probNotBetter {
				return
			}
		}

		// Calculate the total routing info size if this hop were to be
		// included. If we are coming from the source hop, the payload
		// size is zero, because the original htlc isn't in the onion
		// blob.
		var payloadSize uint64
		if fromVertex != source {
			// In case the unifiedEdge does not have a payload size
			// function supplied we request a graceful shutdown
			// because this should never happen.
			if edge.hopPayloadSizeFn == nil {
				log.Criticalf("No payload size function "+
					"available for edge=%v unable to "+
					"determine payload size: %v", edge,
					ErrNoPayLoadSizeFunc)

				return
			}

			supportsTlv := fromFeatures.HasFeature(
				lnwire.TLVOnionPayloadOptional,
			)

			payloadSize = edge.hopPayloadSizeFn(
				amountToSend,
				uint32(toNodeDist.incomingCltv),
				!supportsTlv, edge.policy.ChannelID,
			)
		}

		routingInfoSize := toNodeDist.routingInfoSize + payloadSize
		// Skip paths that would exceed the maximum routing info size.
		if routingInfoSize > sphinx.MaxPayloadSize {
			return
		}

		// All conditions are met and this new tentative distance is
		// better than the current best known distance to this node.
		// The new better distance is recorded, and also our "next hop"
		// map is populated with this edge.
		withDist := &nodeWithDist{
			dist:              tempDist,
			weight:            tempWeight,
			node:              fromVertex,
			netAmountReceived: netAmountToReceive,
			outboundFee:       lnwire.MilliSatoshi(outboundFee),
			incomingCltv:      incomingCltv,
			probability:       probability,
			nextHop:           edge,
			routingInfoSize:   routingInfoSize,
		}
		distance[fromVertex] = withDist

		// Either push withDist onto the heap if the node
		// represented by fromVertex is not already on the heap OR adjust
		// its position within the heap via heap.Fix.
		nodeHeap.PushOrFix(withDist)
	}

	// TODO(roasbeef): also add path caching
	//  * similar to route caching, but doesn't factor in the amount

	// Cache features because we visit nodes multiple times.
	featureCache := make(map[route.Vertex]*lnwire.FeatureVector)

	// getGraphFeatures returns (cached) node features from the graph.
	getGraphFeatures := func(node route.Vertex) (*lnwire.FeatureVector,
		error) {

		// Check cache for features of the fromNode.
		fromFeatures, ok := featureCache[node]
		if ok {
			return fromFeatures, nil
		}

		// Fetch node features fresh from the graph.
		fromFeatures, err := g.graph.fetchNodeFeatures(node)
		if err != nil {
			return nil, err
		}

		// Don't route through nodes that contain unknown required
		// features and mark as nil in the cache.
		err = feature.ValidateRequired(fromFeatures)
		if err != nil {
			featureCache[node] = nil
			return nil, nil
		}

		// Don't route through nodes that don't properly set all
		// transitive feature dependencies and mark as nil in the cache.
		err = feature.ValidateDeps(fromFeatures)
		if err != nil {
			featureCache[node] = nil
			return nil, nil
		}

		// Update cache.
		featureCache[node] = fromFeatures

		return fromFeatures, nil
	}

	routeToSelf := source == target
	for {
		nodesVisited++

		pivot := partialPath.node
		isExitHop := partialPath.nextHop == nil

		// Create unified policies for all incoming connections. Don't
		// use inbound fees for the exit hop.
		u := newNodeEdgeUnifier(
			self, pivot, !isExitHop, outgoingChanMap,
		)

		err := u.addGraphPolicies(g.graph)
		if err != nil {
			return nil, 0, err
		}

		// We add hop hints that were supplied externally.
		for _, reverseEdge := range additionalEdgesWithSrc[pivot] {
			// Assume zero inbound fees for route hints. If inbound
			// fees would apply, they couldn't be communicated in
			// bolt11 invoices currently.
			inboundFee := models.InboundFee{}

			// Hop hints don't contain a capacity. We set one here,
			// since a capacity is needed for probability
			// calculations. We set a high capacity to act as if
			// there is enough liquidity, otherwise the hint would
			// not have been added by a wallet.
			// We also pass the payload size function to the
			// graph data so that we calculate the exact payload
			// size when evaluating this hop for a route.
			u.addPolicy(
				reverseEdge.sourceNode,
				reverseEdge.edge.EdgePolicy(),
				inboundFee,
				fakeHopHintCapacity,
				reverseEdge.edge.IntermediatePayloadSize,
			)
		}

		netAmountReceived := partialPath.netAmountReceived

		// Expand all connections using the optimal policy for each
		// connection.
		for fromNode, edgeUnifier := range u.edgeUnifiers {
			// The target node is not recorded in the distance map.
			// Therefore we need to have this check to prevent
			// creating a cycle. Only when we intend to route to
			// self, we allow this cycle to form. In that case we'll
			// also break out of the search loop below.
			if !routeToSelf && fromNode == target {
				continue
			}

			// Apply last hop restriction if set.
			if r.LastHop != nil &&
				pivot == target && fromNode != *r.LastHop {

				continue
			}

			edge := edgeUnifier.getEdge(
				netAmountReceived, g.bandwidthHints,
				partialPath.outboundFee,
			)

			if edge == nil {
				continue
			}

			// Get feature vector for fromNode.
			fromFeatures, err := getGraphFeatures(fromNode)
			if err != nil {
				return nil, 0, err
			}

			// If there are no valid features, skip this node.
			if fromFeatures == nil {
				continue
			}

			// Check if this candidate node is better than what we
			// already have.
			processEdge(fromNode, fromFeatures, edge, partialPath)
		}

		if nodeHeap.Len() == 0 {
			break
		}

		// Fetch the node within the smallest distance from our source
		// from the heap.
		partialPath = heap.Pop(&nodeHeap).(*nodeWithDist)

		// If we've reached our source (or we don't have any incoming
		// edges), then we're done here and can exit the graph
		// traversal early.
		if partialPath.node == source {
			break
		}
	}

	// Use the distance map to unravel the forward path from source to
	// target.
	var pathEdges []*unifiedEdge
	currentNode := source
	for {
		// Determine the next hop forward using the next map.
		currentNodeWithDist, ok := distance[currentNode]
		if !ok {
			// If the node doesn't have a next hop it means we
			// didn't find a path.
			return nil, 0, errNoPathFound
		}

		// Add the next hop to the list of path edges.
		pathEdges = append(pathEdges, currentNodeWithDist.nextHop)

		// Advance current node.
		currentNode = currentNodeWithDist.nextHop.policy.ToNodePubKey()

		// Check stop condition at the end of this loop. This prevents
		// breaking out too soon for self-payments that have target set
		// to source.
		if currentNode == target {
			break
		}
	}

	// For the final hop, we'll set the node features to those determined
	// above. These are either taken from the destination features, e.g.
	// virtual or invoice features, or loaded as a fallback from the graph.
	// The transitive dependencies were already validated above, so no need
	// to do so now.
	//
	// NOTE: This may overwrite features loaded from the graph if
	// destination features were provided. This is fine though, since our
	// route construction does not care where the features are actually
	// taken from. In the future we may wish to do route construction within
	// findPath, and avoid using ChannelEdgePolicy altogether.
	pathEdges[len(pathEdges)-1].policy.ToNodeFeatures = features

	log.Debugf("Found route: probability=%v, hops=%v, fee=%v",
		distance[source].probability, len(pathEdges),
		distance[source].netAmountReceived-amt)

	return pathEdges, distance[source].probability, nil
}

// getProbabilityBasedDist converts a weight into a distance that takes into
// account the success probability and the (virtual) cost of a failed payment
// attempt.
//
// Derivation:
//
// Suppose there are two routes A and B with fees Fa and Fb and success
// probabilities Pa and Pb.
//
// Is the expected cost of trying route A first and then B lower than trying the
// other way around?
//
// The expected cost of A-then-B is: Pa*Fa + (1-Pa)*Pb*(c+Fb)
//
// The expected cost of B-then-A is: Pb*Fb + (1-Pb)*Pa*(c+Fa)
//
// In these equations, the term representing the case where both A and B fail is
// left out because its value would be the same in both cases.
//
// Pa*Fa + (1-Pa)*Pb*(c+Fb) < Pb*Fb + (1-Pb)*Pa*(c+Fa)
//
// Pa*Fa + Pb*c + Pb*Fb - Pa*Pb*c - Pa*Pb*Fb < Pb*Fb + Pa*c + Pa*Fa - Pa*Pb*c - Pa*Pb*Fa
//
// Removing terms that cancel out:
// Pb*c - Pa*Pb*Fb < Pa*c - Pa*Pb*Fa
//
// Divide by Pa*Pb:
// c/Pa - Fb < c/Pb - Fa
//
// Move terms around:
// Fa + c/Pa < Fb + c/Pb
//
// So the value of F + c/P can be used to compare routes.
func getProbabilityBasedDist(weight int64, probability float64,
	penalty float64) int64 {

	// Prevent divide by zero by returning early.
	if probability == 0 {
		return infinity
	}

	// Calculate distance.
	dist := float64(weight) + penalty/probability

	// Avoid cast if an overflow would occur. The maxFloat constant is
	// chosen to stay well below the maximum float64 value that is still
	// convertible to int64.
	const maxFloat = 9000000000000000000
	if dist > maxFloat {
		return infinity
	}

	return int64(dist)
}

// lastHopPayloadSize calculates the payload size of the final hop in a route.
// It depends on the tlv types which are present and also whether the hop is
// part of a blinded route or not.
func lastHopPayloadSize(r *RestrictParams, finalHtlcExpiry int32,
	amount lnwire.MilliSatoshi, legacy bool) uint64 {

	if r.BlindedPayment != nil {
		blindedPath := r.BlindedPayment.BlindedPath.BlindedHops
		blindedPoint := r.BlindedPayment.BlindedPath.BlindingPoint

		encryptedData := blindedPath[len(blindedPath)-1].CipherText
		finalHop := route.Hop{
			AmtToForward:     amount,
			OutgoingTimeLock: uint32(finalHtlcExpiry),
			LegacyPayload:    false,
			EncryptedData:    encryptedData,
		}
		if len(blindedPath) == 1 {
			finalHop.BlindingPoint = blindedPoint
		}

		// The final hop does not have a short chanID set.
		return finalHop.PayloadSize(0)
	}

	var mpp *record.MPP
	if r.PaymentAddr != nil {
		mpp = record.NewMPP(amount, *r.PaymentAddr)
	}

	var amp *record.AMP
	if r.Amp != nil {
		// The AMP payload is not easy accessible at this point but we
		// are only interested in the size of the payload so we just use
		// the AMP record dummy.
		amp = &record.MaxAmpPayLoadSize
	}

	finalHop := route.Hop{
		AmtToForward:     amount,
		OutgoingTimeLock: uint32(finalHtlcExpiry),
		CustomRecords:    r.DestCustomRecords,
		LegacyPayload:    legacy,
		MPP:              mpp,
		AMP:              amp,
		Metadata:         r.Metadata,
	}

	// The final hop does not have a short chanID set.
	return finalHop.PayloadSize(0)
}
