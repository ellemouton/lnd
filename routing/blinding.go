package routing

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/fn"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

var (
	// ErrNoBlindedPath is returned when the blinded path in a blinded
	// payment is missing.
	ErrNoBlindedPath = errors.New("blinded path required")

	// ErrInsufficientBlindedHops is returned when a blinded path does
	// not have enough blinded hops.
	ErrInsufficientBlindedHops = errors.New("blinded path requires " +
		"at least one hop")

	// ErrHTLCRestrictions is returned when a blinded path has invalid
	// HTLC maximum and minimum values.
	ErrHTLCRestrictions = errors.New("invalid htlc minimum and maximum")
)

// BlindedPaymentPathSet
type BlindedPaymentPathSet struct {
	// Paths is the set of blinded payment paths for a single payment.
	Paths []*BlindedPayment

	// Features is the set of relay features available for the payment.
	// This is extracted from the set of blinded payment paths. At the
	// moment we require that all paths for the same payment have the
	// same feature set.
	Features *lnwire.FeatureVector

	// TargetPubKey is the ephemeral node pub key that we will inject into
	// each path as the last hop. After a route has been selected, we will
	// map the final node destinations back to the real thing using the
	// finalHopMap below. In the edge case where there is only a single
	// path a single path (the introduction node is the destination node),
	// then this will just be the introduction node's real public key.
	TargetPubKey *btcec.PublicKey

	// finalHopMap maps the index of blinded path in the Paths set to the
	// real blinded pub key of the destination node of that path.
	finalHopMap map[int]*btcec.PublicKey

	// FinalCLTV is the final hop's expiry delta of any path in the set.
	// For any multi-hop path, the final CLTV delta should be seen as zero
	// since the final hops final CLTV delta is accounted for in the
	// accumulated path policy values. The only edge case is for when the
	// final hop in the path is also the introduction node in which case
	// that path's FinalCLTV must be the non-zero min CLTV of the final hop
	// so that it is accounted for in path finding. For this reason, if
	// we have any single path in the set with only one hop, then we throw
	// away all the other paths. This should be fine to do since if there is
	// a path where the intro node is also the destination node, then there
	// isn't any need to try any other longer blinded path.
	FinalCLTV uint16
}

// NewBlindedPaymentPathSet constructs a new BlindedPaymentPathSet from a set of
// BlindedPayments.
func NewBlindedPaymentPathSet(paths []*BlindedPayment) (*BlindedPaymentPathSet,
	error) {

	if len(paths) == 0 {
		return nil, ErrNoBlindedPath
	}

	// Derive an ephemeral target priv key that will be injected into each
	// blinded path final hop.
	targetPriv, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}
	targetPub := targetPriv.PubKey()

	features := paths[0].Features
	noFeatures := features == nil || features.IsEmpty()
	for i := 1; i < len(paths); i++ {
		noFeats := paths[i].Features == nil ||
			paths[i].Features.IsEmpty()

		if noFeatures && !noFeats {
			return nil, fmt.Errorf("all blinded paths must have " +
				"the same set of features")
		}

		if noFeatures {
			continue
		}

		if !features.RawFeatureVector.Equals(
			paths[i].Features.RawFeatureVector,
		) {
			return nil, fmt.Errorf("all blinded paths must have " +
				"the same set of features")
		}
	}

	var (
		pathSet        = paths
		finalCLTVDelta uint16
	)
	// If any provided blinded path only has a single hop (ie, the
	// destination node is also the introduction node), then we discard all
	// other paths since we know the real pub key of the destination node.
	// We also then set the final CLTV delta to the path's delta since
	// there are no other edge hints that will account for it. For a single
	// hop path, there is also no need for the pseudo target pub key
	// replacement, so our target pub key in this case just remains the
	// real introduction node ID.
	for _, path := range paths {
		if len(path.BlindedPath.BlindedHops) != 1 {
			continue
		}

		pathSet = []*BlindedPayment{path}
		finalCLTVDelta = path.CltvExpiryDelta
		targetPub = path.BlindedPath.IntroductionPoint

		break
	}

	return &BlindedPaymentPathSet{
		Paths:        pathSet,
		Features:     features,
		TargetPubKey: targetPub,
		FinalCLTV:    finalCLTVDelta,
		finalHopMap:  make(map[int]*btcec.PublicKey),
	}, nil
}

func (ps *BlindedPaymentPathSet) toRouteHints() RouteHints {
	hints := make(RouteHints)

	ps.finalHopMap = make(map[int]*btcec.PublicKey)
	for pathID, path := range ps.Paths {
		pathHints, realFinalHop := path.toRouteHints(
			ps.TargetPubKey, pathID,
		)
		fmt.Println("real final: ", realFinalHop)

		ps.finalHopMap[pathID] = realFinalHop

		for from, edges := range pathHints {
			hints[from] = append(hints[from], edges...)
		}
	}

	if len(hints) == 0 {
		return nil
	}

	return hints
}

func (ps *BlindedPaymentPathSet) LargestLastHopPayloadPath() *BlindedPayment {
	var (
		largestPath *BlindedPayment
		currentMax  int
	)
	for _, path := range ps.Paths {
		numHops := len(path.BlindedPath.BlindedHops)
		lastHop := path.BlindedPath.BlindedHops[numHops-1]

		if len(lastHop.CipherText) > currentMax {
			largestPath = path
		}
	}

	return largestPath
}

func (ps *BlindedPaymentPathSet) GetRealFinalHopPubKey(
	id int) (*btcec.PublicKey, error) {

	key, ok := ps.finalHopMap[id]
	if !ok {
		return nil, fmt.Errorf("no key mapping found for blinded "+
			"path %d", id)
	}

	return key, nil
}

// BlindedPayment provides the path and payment parameters required to send a
// payment along a blinded path.
type BlindedPayment struct {
	// BlindedPath contains the unblinded introduction point and blinded
	// hops for the blinded section of the payment.
	BlindedPath *sphinx.BlindedPath

	// BaseFee is the total base fee to be paid for payments made over the
	// blinded path.
	BaseFee uint32

	// ProportionalFeeRate is the aggregated proportional fee rate for
	// payments made over the blinded path.
	ProportionalFeeRate uint32

	// CltvExpiryDelta is the total expiry delta for the blinded path. This
	// field includes the CLTV for the blinded hops *and* the final cltv
	// delta for the receiver.
	CltvExpiryDelta uint16

	// HtlcMinimum is the highest HLTC minimum supported along the blinded
	// path (while some hops may have lower values, we're effectively
	// bounded by the highest minimum).
	HtlcMinimum uint64

	// HtlcMaximum is the lowest HTLC maximum supported along the blinded
	// path (while some hops may have higher values, we're effectively
	// bounded by the lowest maximum).
	HtlcMaximum uint64

	// Features is the set of relay features available for the payment.
	Features *lnwire.FeatureVector
}

// Validate performs validation on a blinded payment.
func (b *BlindedPayment) Validate() error {
	if b.BlindedPath == nil {
		return ErrNoBlindedPath
	}

	// The sphinx library inserts the introduction node as the first hop,
	// so we expect at least one hop.
	if len(b.BlindedPath.BlindedHops) < 1 {
		return fmt.Errorf("%w got: %v", ErrInsufficientBlindedHops,
			len(b.BlindedPath.BlindedHops))
	}

	if b.HtlcMaximum < b.HtlcMinimum {
		return fmt.Errorf("%w: %v < %v", ErrHTLCRestrictions,
			b.HtlcMaximum, b.HtlcMinimum)
	}

	return nil
}

// toRouteHints produces a set of chained route hints that represent a blinded
// path. In the case of a single hop blinded route (which is paying directly
// to the introduction point), no hints will be returned. In this case callers
// *must* account for the blinded route's CLTV delta elsewhere (as this is
// effectively the final_cltv_delta for the receiving introduction node). In
// the case of multiple blinded hops, CLTV delta is fully accounted for in the
// hints (both for intermediate hops and the final_cltv_delta for the receiving
// node).
func (b *BlindedPayment) toRouteHints(pseudoTarget *btcec.PublicKey,
	pathIdentifier int) (RouteHints, *btcec.PublicKey) {

	// If we just have a single hop in our blinded route, it just contains
	// an introduction node (this is a valid path according to the spec).
	// Since we have the un-blinded node ID for the introduction node, we
	// don't need to add any route hints.
	if len(b.BlindedPath.BlindedHops) == 1 {
		return nil, b.BlindedPath.IntroductionPoint
	}

	hintCount := len(b.BlindedPath.BlindedHops) - 1
	hints := make(
		RouteHints, hintCount,
	)

	// Start at the unblinded introduction node, because our pathfinding
	// will be able to locate this point in the graph.
	fromNode := route.NewVertex(b.BlindedPath.IntroductionPoint)

	features := lnwire.EmptyFeatureVector()
	if b.Features != nil {
		features = b.Features.Clone()
	}

	// Use the total aggregate relay parameters for the entire blinded
	// route as the policy for the hint from our introduction node. This
	// will ensure that pathfinding provides sufficient fees/delay for the
	// blinded portion to the introduction node.
	firstBlindedHop := b.BlindedPath.BlindedHops[1].BlindedNodePub
	edgePolicy := &models.CachedEdgePolicy{
		TimeLockDelta: b.CltvExpiryDelta,
		MinHTLC:       lnwire.MilliSatoshi(b.HtlcMinimum),
		MaxHTLC:       lnwire.MilliSatoshi(b.HtlcMaximum),
		FeeBaseMSat:   lnwire.MilliSatoshi(b.BaseFee),
		FeeProportionalMillionths: lnwire.MilliSatoshi(
			b.ProportionalFeeRate,
		),
		ToNodePubKey: func() route.Vertex {
			return route.NewVertex(
				// The first node in this slice is
				// the introduction node, so we start
				// at index 1 to get the first blinded
				// relaying node.
				firstBlindedHop,
			)
		},
		ToNodeFeatures: features,
		BlindedEdgeID:  fn.Some[int](pathIdentifier),
	}

	lastEdge := &BlindedEdge{
		policy:        edgePolicy,
		cipherText:    b.BlindedPath.BlindedHops[0].CipherText,
		blindingPoint: b.BlindedPath.BlindingPoint,
	}
	lastEdgePubKey := firstBlindedHop
	hints[fromNode] = []AdditionalEdge{lastEdge}

	// Start at an offset of 1 because the first node in our blinded hops
	// is the introduction node and terminate at the second-last node
	// because we're dealing with hops as pairs.
	for i := 1; i < hintCount; i++ {
		// Set our origin node to the current
		fromNode = route.NewVertex(
			b.BlindedPath.BlindedHops[i].BlindedNodePub,
		)

		// Create a hint which has no fee or cltv delta. We
		// specifically want zero values here because our relay
		// parameters are expressed in encrypted blobs rather than the
		// route itself for blinded routes.
		nextHopIdx := i + 1
		nextNode := route.NewVertex(
			b.BlindedPath.BlindedHops[nextHopIdx].BlindedNodePub,
		)

		edgePolicy := &models.CachedEdgePolicy{
			ToNodePubKey: func() route.Vertex {
				return nextNode
			},
			ToNodeFeatures: features,
			BlindedEdgeID:  fn.Some[int](pathIdentifier),
		}

		lastEdge = &BlindedEdge{
			policy: edgePolicy,
			cipherText: b.BlindedPath.BlindedHops[i].
				CipherText,
		}
		lastEdgePubKey = b.BlindedPath.BlindedHops[nextHopIdx].
			BlindedNodePub

		hints[fromNode] = []AdditionalEdge{lastEdge}
	}

	// For the very last hop on the path, switch out the ToNodePub for the
	// pseudo target pub key.
	lastEdge.policy.ToNodePubKey = func() route.Vertex {
		return route.NewVertex(pseudoTarget)
	}

	// Then override the final hint with this updated edge.
	hints[fromNode] = []AdditionalEdge{lastEdge}

	return hints, lastEdgePubKey
}
