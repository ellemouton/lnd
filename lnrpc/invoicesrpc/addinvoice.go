package invoicesrpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	mathRand "math/rand"
	"sort"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/chainreg"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/invoices"
	"github.com/lightningnetwork/lnd/lntypes"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/netann"
	"github.com/lightningnetwork/lnd/record"
	"github.com/lightningnetwork/lnd/routing"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/zpay32"
)

const (
	// DefaultInvoiceExpiry is the default invoice expiry for new MPP
	// invoices.
	DefaultInvoiceExpiry = 24 * time.Hour

	// DefaultAMPInvoiceExpiry is the default invoice expiry for new AMP
	// invoices.
	DefaultAMPInvoiceExpiry = 30 * 24 * time.Hour

	// hopHintFactor is factor by which we scale the total amount of
	// inbound capacity we want our hop hints to represent, allowing us to
	// have some leeway if peers go offline.
	hopHintFactor = 2

	// maxHopHints is the maximum number of hint paths that will be included
	// in an invoice.
	maxHopHints = 20
)

// AddInvoiceConfig contains dependencies for invoice creation.
type AddInvoiceConfig struct {
	// AddInvoice is called to add the invoice to the registry.
	AddInvoice func(ctx context.Context, invoice *invoices.Invoice,
		paymentHash lntypes.Hash) (uint64, error)

	// IsChannelActive is used to generate valid hop hints.
	IsChannelActive func(chanID lnwire.ChannelID) bool

	// ChainParams are required to properly decode invoice payment requests
	// that are marshalled over rpc.
	ChainParams *chaincfg.Params

	// NodeSigner is an implementation of the MessageSigner implementation
	// that's backed by the identity private key of the running lnd node.
	NodeSigner *netann.NodeSigner

	// DefaultCLTVExpiry is the default invoice expiry if no values is
	// specified.
	DefaultCLTVExpiry uint32

	// ChanDB is a global boltdb instance which is needed to access the
	// channel graph.
	ChanDB *channeldb.ChannelStateDB

	// Graph holds a reference to the ChannelGraph database.
	Graph *channeldb.ChannelGraph

	// GenInvoiceFeatures returns a feature containing feature bits that
	// should be advertised on freshly generated invoices.
	GenInvoiceFeatures func() *lnwire.FeatureVector

	// GenAmpInvoiceFeatures returns a feature containing feature bits that
	// should be advertised on freshly generated AMP invoices.
	GenAmpInvoiceFeatures func() *lnwire.FeatureVector

	// GetAlias allows the peer's alias SCID to be retrieved for private
	// option_scid_alias channels.
	GetAlias func(lnwire.ChannelID) (lnwire.ShortChannelID, error)

	// QueryBlindedRoutes can be used to generate a few routes to this node
	// that can then be used in the construction of a blinded payment path.
	QueryBlindedRoutes func(lnwire.MilliSatoshi) ([]*route.Route, error)

	// BlindedRoutePolicyMultiplier is the amount by which policy values for
	// hops in a blinded route will be bumped to avoid easy probing. For
	// example, a multiplier of 1.1 will bump all the values (base fee, fee
	// rate and CLTV delta by 10%).
	BlindedRoutePolicyMultiplier float64
}

// AddInvoiceData contains the required data to create a new invoice.
type AddInvoiceData struct {
	// An optional memo to attach along with the invoice. Used for record
	// keeping purposes for the invoice's creator, and will also be set in
	// the description field of the encoded payment request if the
	// description_hash field is not being used.
	Memo string

	// The preimage which will allow settling an incoming HTLC payable to
	// this preimage. If Preimage is set, Hash should be nil. If both
	// Preimage and Hash are nil, a random preimage is generated.
	Preimage *lntypes.Preimage

	// The hash of the preimage. If Hash is set, Preimage should be nil.
	// This condition indicates that we have a 'hold invoice' for which the
	// htlc will be accepted and held until the preimage becomes known.
	Hash *lntypes.Hash

	// The value of this invoice in millisatoshis.
	Value lnwire.MilliSatoshi

	// Hash (SHA-256) of a description of the payment. Used if the
	// description of payment (memo) is too long to naturally fit within the
	// description field of an encoded payment request.
	DescriptionHash []byte

	// Payment request expiry time in seconds. Default is 3600 (1 hour).
	Expiry int64

	// Fallback on-chain address.
	FallbackAddr string

	// Delta to use for the time-lock of the CLTV extended to the final hop.
	CltvExpiry uint64

	// Whether this invoice should include routing hints for private
	// channels.
	Private bool

	// HodlInvoice signals that this invoice shouldn't be settled
	// immediately upon receiving the payment.
	HodlInvoice bool

	// Amp signals whether to create an AMP invoice.
	//
	// NOTE: Preimage should always be set to nil when this value is true.
	Amp bool

	// Blinded signals that this invoice should disguise the location of the
	// recipient by adding blinded payment paths to the invoice instead of
	// revealing the destination node's real pub key.
	Blinded bool

	// RouteHints are optional route hints that can each be individually
	// used to assist in reaching the invoice's destination.
	RouteHints [][]zpay32.HopHint
}

// paymentHashAndPreimage returns the payment hash and preimage for this invoice
// depending on the configuration.
//
// For AMP invoices (when Amp flag is true), this method always returns a nil
// preimage. The hash value can be set externally by the user using the Hash
// field, or one will be generated randomly. The payment hash here only serves
// as a unique identifier for insertion into the invoice index, as there is
// no universal preimage for an AMP payment.
//
// For MPP invoices (when Amp flag is false), this method may return nil
// preimage when create a hodl invoice, but otherwise will always return a
// non-nil preimage and the corresponding payment hash. The valid combinations
// are parsed as follows:
//   - Preimage == nil && Hash == nil -> (random preimage, H(random preimage))
//   - Preimage != nil && Hash == nil -> (Preimage, H(Preimage))
//   - Preimage == nil && Hash != nil -> (nil, Hash)
func (d *AddInvoiceData) paymentHashAndPreimage() (
	*lntypes.Preimage, lntypes.Hash, error) {

	switch {
	// For now, disallow an AMP payment for a payment to a blinded path.
	case d.Amp && d.Blinded:
		return nil, lntypes.Hash{}, fmt.Errorf("cannot currently do " +
			"an AMP payment to a blinded path")

	case d.Amp:
		return d.ampPaymentHashAndPreimage()

	default:
		return d.mppPaymentHashAndPreimage()
	}
}

// ampPaymentHashAndPreimage returns the payment hash to use for an AMP invoice.
// The preimage will always be nil.
func (d *AddInvoiceData) ampPaymentHashAndPreimage() (*lntypes.Preimage,
	lntypes.Hash, error) {

	switch {
	// Preimages cannot be set on AMP invoice.
	case d.Preimage != nil:
		return nil, lntypes.Hash{},
			errors.New("preimage set on AMP invoice")

	// If a specific hash was requested, use that.
	case d.Hash != nil:
		return nil, *d.Hash, nil

	// Otherwise generate a random hash value, just needs to be unique to be
	// added to the invoice index.
	default:
		var paymentHash lntypes.Hash
		if _, err := rand.Read(paymentHash[:]); err != nil {
			return nil, lntypes.Hash{}, err
		}

		return nil, paymentHash, nil
	}
}

// mppPaymentHashAndPreimage returns the payment hash and preimage to use for an
// MPP invoice.
func (d *AddInvoiceData) mppPaymentHashAndPreimage() (*lntypes.Preimage,
	lntypes.Hash, error) {

	var (
		paymentPreimage *lntypes.Preimage
		paymentHash     lntypes.Hash
	)

	switch {

	// Only either preimage or hash can be set.
	case d.Preimage != nil && d.Hash != nil:
		return nil, lntypes.Hash{},
			errors.New("preimage and hash both set")

	// If no hash or preimage is given, generate a random preimage.
	case d.Preimage == nil && d.Hash == nil:
		paymentPreimage = &lntypes.Preimage{}
		if _, err := rand.Read(paymentPreimage[:]); err != nil {
			return nil, lntypes.Hash{}, err
		}
		paymentHash = paymentPreimage.Hash()

	// If just a hash is given, we create a hold invoice by setting the
	// preimage to unknown.
	case d.Preimage == nil && d.Hash != nil:
		paymentHash = *d.Hash

	// A specific preimage was supplied. Use that for the invoice.
	case d.Preimage != nil && d.Hash == nil:
		preimage := *d.Preimage
		paymentPreimage = &preimage
		paymentHash = d.Preimage.Hash()
	}

	return paymentPreimage, paymentHash, nil
}

// AddInvoice attempts to add a new invoice to the invoice database. Any
// duplicated invoices are rejected, therefore all invoices *must* have a
// unique payment preimage.
func AddInvoice(ctx context.Context, cfg *AddInvoiceConfig,
	invoice *AddInvoiceData) (*lntypes.Hash, *invoices.Invoice, error) {

	paymentPreimage, paymentHash, err := invoice.paymentHashAndPreimage()
	if err != nil {
		return nil, nil, err
	}

	// The size of the memo, receipt and description hash attached must not
	// exceed the maximum values for either of the fields.
	if len(invoice.Memo) > invoices.MaxMemoSize {
		return nil, nil, fmt.Errorf("memo too large: %v bytes "+
			"(maxsize=%v)", len(invoice.Memo),
			invoices.MaxMemoSize)
	}
	if len(invoice.DescriptionHash) > 0 &&
		len(invoice.DescriptionHash) != 32 {

		return nil, nil, fmt.Errorf("description hash is %v bytes, "+
			"must be 32", len(invoice.DescriptionHash))
	}

	// We set the max invoice amount to 100k BTC, which itself is several
	// multiples off the current block reward.
	maxInvoiceAmt := btcutil.Amount(btcutil.SatoshiPerBitcoin * 100000)

	switch {
	// The value of the invoice must not be negative.
	case int64(invoice.Value) < 0:
		return nil, nil, fmt.Errorf("payments of negative value "+
			"are not allowed, value is %v", int64(invoice.Value))

	// Also ensure that the invoice is actually realistic, while preventing
	// any issues due to underflow.
	case invoice.Value.ToSatoshis() > maxInvoiceAmt:
		return nil, nil, fmt.Errorf("invoice amount %v is "+
			"too large, max is %v", invoice.Value.ToSatoshis(),
			maxInvoiceAmt)
	}

	amtMSat := invoice.Value

	// We also create an encoded payment request which allows the
	// caller to compactly send the invoice to the payer. We'll create a
	// list of options to be added to the encoded payment request. For now
	// we only support the required fields description/description_hash,
	// expiry, fallback address, and the amount field.
	var options []func(*zpay32.Invoice)

	// We only include the amount in the invoice if it is greater than 0.
	// By not including the amount, we enable the creation of invoices that
	// allow the payer to specify the amount of satoshis they wish to send.
	if amtMSat > 0 {
		options = append(options, zpay32.Amount(amtMSat))
	}

	// If specified, add a fallback address to the payment request.
	if len(invoice.FallbackAddr) > 0 {
		addr, err := btcutil.DecodeAddress(
			invoice.FallbackAddr, cfg.ChainParams,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid fallback "+
				"address: %v", err)
		}

		if !addr.IsForNet(cfg.ChainParams) {
			return nil, nil, fmt.Errorf("fallback address is not "+
				"for %s", cfg.ChainParams.Name)
		}

		options = append(options, zpay32.FallbackAddr(addr))
	}

	switch {
	// If expiry is set, specify it. If it is not provided, no expiry time
	// will be explicitly added to this payment request, which will imply
	// the default 3600 seconds.
	case invoice.Expiry > 0:

		// We'll ensure that the specified expiry is restricted to sane
		// number of seconds. As a result, we'll reject an invoice with
		// an expiry greater than 1 year.
		maxExpiry := time.Hour * 24 * 365
		expSeconds := invoice.Expiry

		if float64(expSeconds) > maxExpiry.Seconds() {
			return nil, nil, fmt.Errorf("expiry of %v seconds "+
				"greater than max expiry of %v seconds",
				float64(expSeconds), maxExpiry.Seconds())
		}

		expiry := time.Duration(invoice.Expiry) * time.Second
		options = append(options, zpay32.Expiry(expiry))

	// If no custom expiry is provided, use the default MPP expiry.
	case !invoice.Amp:
		options = append(options, zpay32.Expiry(DefaultInvoiceExpiry))

	// Otherwise, use the default AMP expiry.
	default:
		defaultExpiry := zpay32.Expiry(DefaultAMPInvoiceExpiry)
		options = append(options, defaultExpiry)
	}

	// If the description hash is set, then we add it do the list of
	// options. If not, use the memo field as the payment request
	// description.
	if len(invoice.DescriptionHash) > 0 {
		var descHash [32]byte
		copy(descHash[:], invoice.DescriptionHash[:])
		options = append(options, zpay32.DescriptionHash(descHash))
	} else {
		// Use the memo field as the description. If this is not set
		// this will just be an empty string.
		options = append(options, zpay32.Description(invoice.Memo))
	}

	// We'll use our current default CLTV value unless one was specified as
	// an option on the command line when creating an invoice.
	switch {
	case invoice.CltvExpiry > routing.MaxCLTVDelta:
		return nil, nil, fmt.Errorf("CLTV delta of %v is too large, "+
			"max accepted is: %v", invoice.CltvExpiry,
			math.MaxUint16)

	case invoice.CltvExpiry != 0:
		// Disallow user-chosen final CLTV deltas below the required
		// minimum.
		if invoice.CltvExpiry < routing.MinCLTVDelta {
			return nil, nil, fmt.Errorf("CLTV delta of %v must be "+
				"greater than minimum of %v",
				routing.MinCLTVDelta, invoice.CltvExpiry)
		}

		options = append(options,
			zpay32.CLTVExpiry(invoice.CltvExpiry))

	default:
		// TODO(roasbeef): assumes set delta between versions
		defaultCLTVExpiry := uint64(cfg.DefaultCLTVExpiry)
		options = append(options, zpay32.CLTVExpiry(defaultCLTVExpiry))
	}

	// We make sure that the given invoice routing hints number is within
	// the valid range
	if len(invoice.RouteHints) > maxHopHints {
		return nil, nil, fmt.Errorf("number of routing hints must "+
			"not exceed maximum of %v", maxHopHints)
	}

	// Include route hints if needed.
	if len(invoice.RouteHints) > 0 || invoice.Private {
		if invoice.Blinded {
			return nil, nil, fmt.Errorf("can't set both hop " +
				"hints and add blinded paths")
		}

		// Validate provided hop hints.
		for _, hint := range invoice.RouteHints {
			if len(hint) == 0 {
				return nil, nil, fmt.Errorf("number of hop " +
					"hint within a route must be positive")
			}
		}

		totalHopHints := len(invoice.RouteHints)
		if invoice.Private {
			totalHopHints = maxHopHints
		}

		hopHintsCfg := newSelectHopHintsCfg(cfg, totalHopHints)
		hopHints, err := PopulateHopHints(
			hopHintsCfg, amtMSat, invoice.RouteHints,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to populate hop "+
				"hints: %v", err)
		}

		// Convert our set of selected hop hints into route
		// hints and add to our invoice options.
		for _, hopHint := range hopHints {
			routeHint := zpay32.RouteHint(hopHint)

			options = append(
				options, routeHint,
			)
		}
	}

	// Set our desired invoice features and add them to our list of options.
	var invoiceFeatures *lnwire.FeatureVector
	if invoice.Amp {
		invoiceFeatures = cfg.GenAmpInvoiceFeatures()
	} else {
		invoiceFeatures = cfg.GenInvoiceFeatures()
	}
	options = append(options, zpay32.Features(invoiceFeatures))

	// Generate and set a random payment address for this invoice. If the
	// sender understands payment addresses, this can be used to avoid
	// intermediaries probing the receiver. This is only done if the invoice
	// does not include a blinded path since in that case, the recipient is
	// responsible for doing this check by including an appropriate path ID
	// within the encrypted data of the blinded payment path for the final
	// hop.
	var paymentAddr [32]byte
	if _, err := rand.Read(paymentAddr[:]); err != nil {
		return nil, nil, err
	}

	// If the invoice has blinded paths, then we will embed the payment
	// address into the encrypted recipient path ID, but we will not add it
	// to the actual invoice itself.
	if !invoice.Blinded {
		options = append(options, zpay32.PaymentAddr(paymentAddr))
	}

	if invoice.Blinded {
		paths, err := buildBlindedPaymentPaths(&buildBlindedPathCfg{
			findRoutes:            cfg.QueryBlindedRoutes,
			fetchChannelEdgesByID: cfg.Graph.FetchChannelEdgesByID,
			pathID:                paymentAddr[:],
			value:                 invoice.Value,
			policyMultiplier:      cfg.BlindedRoutePolicyMultiplier,
		})
		if err != nil {
			return nil, nil, err
		}

		for _, path := range paths {
			options = append(options, zpay32.WithBlindedPaymentPath(
				path,
			))
		}
	}

	// Create and encode the payment request as a bech32 (zpay32) string.
	creationDate := time.Now()
	payReq, err := zpay32.NewInvoice(
		cfg.ChainParams, paymentHash, creationDate, options...,
	)
	if err != nil {
		return nil, nil, err
	}

	payReqString, err := payReq.Encode(zpay32.MessageSigner{
		SignCompact: func(msg []byte) ([]byte, error) {
			// For an invoice without a blinded path, the main node
			// key is used to sign the invoice so that the sender
			// can derive the true pub key of the recipient.
			if !invoice.Blinded {
				return cfg.NodeSigner.SignMessageCompact(
					msg, false,
				)
			}

			// For an invoice with a blinded path, we use an
			// ephemeral key to sign the invoice since we don't want
			// the sender to be able to know the real pub key of
			// the recipient.
			ephemKey, err := btcec.NewPrivateKey()
			if err != nil {
				return nil, err
			}

			return ecdsa.SignCompact(
				ephemKey, chainhash.HashB(msg), true,
			)
		},
	})
	if err != nil {
		return nil, nil, err
	}

	newInvoice := &invoices.Invoice{
		CreationDate:   creationDate,
		Memo:           []byte(invoice.Memo),
		PaymentRequest: []byte(payReqString),
		Terms: invoices.ContractTerm{
			FinalCltvDelta:  int32(payReq.MinFinalCLTVExpiry()),
			Expiry:          payReq.Expiry(),
			Value:           amtMSat,
			PaymentPreimage: paymentPreimage,
			PaymentAddr:     paymentAddr,
			Features:        invoiceFeatures,
		},
		HodlInvoice: invoice.HodlInvoice,
	}

	log.Tracef("[addinvoice] adding new invoice %v",
		newLogClosure(func() string {
			return spew.Sdump(newInvoice)
		}),
	)

	// With all sanity checks passed, write the invoice to the database.
	_, err = cfg.AddInvoice(ctx, newInvoice, paymentHash)
	if err != nil {
		return nil, nil, err
	}

	return &paymentHash, newInvoice, nil
}

// chanCanBeHopHint returns true if the target channel is eligible to be a hop
// hint.
func chanCanBeHopHint(channel *HopHintInfo, cfg *SelectHopHintsCfg) (
	*models.ChannelEdgePolicy, bool) {

	// Since we're only interested in our private channels, we'll skip
	// public ones.
	if channel.IsPublic {
		return nil, false
	}

	// Make sure the channel is active.
	if !channel.IsActive {
		log.Debugf("Skipping channel %v due to not "+
			"being eligible to forward payments",
			channel.ShortChannelID)
		return nil, false
	}

	// To ensure we don't leak unadvertised nodes, we'll make sure our
	// counterparty is publicly advertised within the network.  Otherwise,
	// we'll end up leaking information about nodes that intend to stay
	// unadvertised, like in the case of a node only having private
	// channels.
	var remotePub [33]byte
	copy(remotePub[:], channel.RemotePubkey.SerializeCompressed())
	isRemoteNodePublic, err := cfg.IsPublicNode(remotePub)
	if err != nil {
		log.Errorf("Unable to determine if node %x "+
			"is advertised: %v", remotePub, err)
		return nil, false
	}

	if !isRemoteNodePublic {
		log.Debugf("Skipping channel %v due to "+
			"counterparty %x being unadvertised",
			channel.ShortChannelID, remotePub)
		return nil, false
	}

	// Fetch the policies for each end of the channel.
	info, p1, p2, err := cfg.FetchChannelEdgesByID(channel.ShortChannelID)
	if err != nil {
		// In the case of zero-conf channels, it may be the case that
		// the alias SCID was deleted from the graph, and replaced by
		// the confirmed SCID. Check the Graph for the confirmed SCID.
		confirmedScid := channel.ConfirmedScidZC
		info, p1, p2, err = cfg.FetchChannelEdgesByID(confirmedScid)
		if err != nil {
			log.Errorf("Unable to fetch the routing policies for "+
				"the edges of the channel %v: %v",
				channel.ShortChannelID, err)
			return nil, false
		}
	}

	// Now, we'll need to determine which is the correct policy for HTLCs
	// being sent from the remote node.
	var remotePolicy *models.ChannelEdgePolicy
	if bytes.Equal(remotePub[:], info.NodeKey1Bytes[:]) {
		remotePolicy = p1
	} else {
		remotePolicy = p2
	}

	return remotePolicy, true
}

// HopHintInfo contains the channel information required to create a hop hint.
type HopHintInfo struct {
	// IsPublic indicates whether a channel is advertised to the network.
	IsPublic bool

	// IsActive indicates whether the channel is online and available for
	// use.
	IsActive bool

	// FundingOutpoint is the funding txid:index for the channel.
	FundingOutpoint wire.OutPoint

	// RemotePubkey is the public key of the remote party that this channel
	// is in.
	RemotePubkey *btcec.PublicKey

	// RemoteBalance is the remote party's balance (our current incoming
	// capacity).
	RemoteBalance lnwire.MilliSatoshi

	// ShortChannelID is the short channel ID of the channel.
	ShortChannelID uint64

	// ConfirmedScidZC is the confirmed SCID of a zero-conf channel. This
	// may be used for looking up a channel in the graph.
	ConfirmedScidZC uint64

	// ScidAliasFeature denotes whether the channel has negotiated the
	// option-scid-alias feature bit.
	ScidAliasFeature bool
}

func newHopHintInfo(c *channeldb.OpenChannel, isActive bool) *HopHintInfo {
	isPublic := c.ChannelFlags&lnwire.FFAnnounceChannel != 0

	return &HopHintInfo{
		IsPublic:         isPublic,
		IsActive:         isActive,
		FundingOutpoint:  c.FundingOutpoint,
		RemotePubkey:     c.IdentityPub,
		RemoteBalance:    c.LocalCommitment.RemoteBalance,
		ShortChannelID:   c.ShortChannelID.ToUint64(),
		ConfirmedScidZC:  c.ZeroConfRealScid().ToUint64(),
		ScidAliasFeature: c.ChanType.HasScidAliasFeature(),
	}
}

// newHopHint returns a new hop hint using the relevant data from a hopHintInfo
// and a ChannelEdgePolicy.
func newHopHint(hopHintInfo *HopHintInfo,
	chanPolicy *models.ChannelEdgePolicy) zpay32.HopHint {

	return zpay32.HopHint{
		NodeID:      hopHintInfo.RemotePubkey,
		ChannelID:   hopHintInfo.ShortChannelID,
		FeeBaseMSat: uint32(chanPolicy.FeeBaseMSat),
		FeeProportionalMillionths: uint32(
			chanPolicy.FeeProportionalMillionths,
		),
		CLTVExpiryDelta: chanPolicy.TimeLockDelta,
	}
}

// SelectHopHintsCfg contains the dependencies required to obtain hop hints
// for an invoice.
type SelectHopHintsCfg struct {
	// IsPublicNode is returns a bool indicating whether the node with the
	// given public key is seen as a public node in the graph from the
	// graph's source node's point of view.
	IsPublicNode func(pubKey [33]byte) (bool, error)

	// FetchChannelEdgesByID attempts to lookup the two directed edges for
	// the channel identified by the channel ID.
	FetchChannelEdgesByID func(chanID uint64) (*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy,
		error)

	// GetAlias allows the peer's alias SCID to be retrieved for private
	// option_scid_alias channels.
	GetAlias func(lnwire.ChannelID) (lnwire.ShortChannelID, error)

	// FetchAllChannels retrieves all open channels currently stored
	// within the database.
	FetchAllChannels func() ([]*channeldb.OpenChannel, error)

	// IsChannelActive checks whether the channel identified by the provided
	// ChannelID is considered active.
	IsChannelActive func(chanID lnwire.ChannelID) bool

	// MaxHopHints is the maximum number of hop hints we are interested in.
	MaxHopHints int
}

func newSelectHopHintsCfg(invoicesCfg *AddInvoiceConfig,
	maxHopHints int) *SelectHopHintsCfg {

	return &SelectHopHintsCfg{
		FetchAllChannels:      invoicesCfg.ChanDB.FetchAllChannels,
		IsChannelActive:       invoicesCfg.IsChannelActive,
		IsPublicNode:          invoicesCfg.Graph.IsPublicNode,
		FetchChannelEdgesByID: invoicesCfg.Graph.FetchChannelEdgesByID,
		GetAlias:              invoicesCfg.GetAlias,
		MaxHopHints:           maxHopHints,
	}
}

// sufficientHints checks whether we have sufficient hop hints, based on the
// any of the following criteria:
//   - Hop hint count: the number of hints have reach our max target.
//   - Total incoming capacity (for non-zero invoice amounts): the sum of the
//     remote balance amount in the hints is bigger of equal than our target
//     (currently twice the invoice amount)
//
// We limit our number of hop hints like this to keep our invoice size down,
// and to avoid leaking all our private channels when we don't need to.
func sufficientHints(nHintsLeft int, currentAmount,
	targetAmount lnwire.MilliSatoshi) bool {

	if nHintsLeft <= 0 {
		log.Debugf("Reached targeted number of hop hints")
		return true
	}

	if targetAmount != 0 && currentAmount >= targetAmount {
		log.Debugf("Total hint amount: %v has reached target hint "+
			"bandwidth: %v", currentAmount, targetAmount)
		return true
	}

	return false
}

// getPotentialHints returns a slice of open channels that should be considered
// for the hopHint list in an invoice. The slice is sorted in descending order
// based on the remote balance.
func getPotentialHints(cfg *SelectHopHintsCfg) ([]*channeldb.OpenChannel,
	error) {

	// TODO(positiveblue): get the channels slice already filtered by
	// private == true and sorted by RemoteBalance?
	openChannels, err := cfg.FetchAllChannels()
	if err != nil {
		return nil, err
	}

	privateChannels := make([]*channeldb.OpenChannel, 0, len(openChannels))
	for _, oc := range openChannels {
		isPublic := oc.ChannelFlags&lnwire.FFAnnounceChannel != 0
		if !isPublic {
			privateChannels = append(privateChannels, oc)
		}
	}

	// Sort the channels in descending remote balance.
	compareRemoteBalance := func(i, j int) bool {
		iBalance := privateChannels[i].LocalCommitment.RemoteBalance
		jBalance := privateChannels[j].LocalCommitment.RemoteBalance
		return iBalance > jBalance
	}
	sort.Slice(privateChannels, compareRemoteBalance)

	return privateChannels, nil
}

// shouldIncludeChannel returns true if the channel passes all the checks to
// be a hopHint in a given invoice.
func shouldIncludeChannel(cfg *SelectHopHintsCfg,
	channel *channeldb.OpenChannel,
	alreadyIncluded map[uint64]bool) (zpay32.HopHint, lnwire.MilliSatoshi,
	bool) {

	if _, ok := alreadyIncluded[channel.ShortChannelID.ToUint64()]; ok {
		return zpay32.HopHint{}, 0, false
	}

	chanID := lnwire.NewChanIDFromOutPoint(
		channel.FundingOutpoint,
	)

	hopHintInfo := newHopHintInfo(channel, cfg.IsChannelActive(chanID))

	// If this channel can't be a hop hint, then skip it.
	edgePolicy, canBeHopHint := chanCanBeHopHint(hopHintInfo, cfg)
	if edgePolicy == nil || !canBeHopHint {
		return zpay32.HopHint{}, 0, false
	}

	if hopHintInfo.ScidAliasFeature {
		alias, err := cfg.GetAlias(chanID)
		if err != nil {
			return zpay32.HopHint{}, 0, false
		}

		if alias.IsDefault() || alreadyIncluded[alias.ToUint64()] {
			return zpay32.HopHint{}, 0, false
		}

		hopHintInfo.ShortChannelID = alias.ToUint64()
	}

	// Now that we know this channel use usable, add it as a hop hint and
	// the indexes we'll use later.
	hopHint := newHopHint(hopHintInfo, edgePolicy)
	return hopHint, hopHintInfo.RemoteBalance, true
}

// selectHopHints iterates a list of potential hints selecting the valid hop
// hints until we have enough hints or run out of channels.
//
// NOTE: selectHopHints expects potentialHints to be already sorted in
// descending priority.
func selectHopHints(cfg *SelectHopHintsCfg, nHintsLeft int,
	targetBandwidth lnwire.MilliSatoshi,
	potentialHints []*channeldb.OpenChannel,
	alreadyIncluded map[uint64]bool) [][]zpay32.HopHint {

	currentBandwidth := lnwire.MilliSatoshi(0)
	hopHints := make([][]zpay32.HopHint, 0, nHintsLeft)
	for _, channel := range potentialHints {
		enoughHopHints := sufficientHints(
			nHintsLeft, currentBandwidth, targetBandwidth,
		)
		if enoughHopHints {
			return hopHints
		}

		hopHint, remoteBalance, include := shouldIncludeChannel(
			cfg, channel, alreadyIncluded,
		)

		if include {
			// Now that we now this channel use usable, add it as a hop
			// hint and the indexes we'll use later.
			hopHints = append(hopHints, []zpay32.HopHint{hopHint})
			currentBandwidth += remoteBalance
			nHintsLeft--
		}
	}

	// We do not want to leak information about how our remote balance is
	// distributed in our private channels. We shuffle the selected ones
	// here so they do not appear in order in the invoice.
	mathRand.Shuffle(
		len(hopHints), func(i, j int) {
			hopHints[i], hopHints[j] = hopHints[j], hopHints[i]
		},
	)
	return hopHints
}

// PopulateHopHints will select up to cfg.MaxHophints from the current open
// channels. The set of hop hints will be returned as a slice of functional
// options that'll append the route hint to the set of all route hints.
//
// TODO(roasbeef): do proper sub-set sum max hints usually << numChans.
func PopulateHopHints(cfg *SelectHopHintsCfg, amtMSat lnwire.MilliSatoshi,
	forcedHints [][]zpay32.HopHint) ([][]zpay32.HopHint, error) {

	hopHints := forcedHints

	// If we already have enough hints we don't need to add any more.
	nHintsLeft := cfg.MaxHopHints - len(hopHints)
	if nHintsLeft <= 0 {
		return hopHints, nil
	}

	alreadyIncluded := make(map[uint64]bool)
	for _, hopHint := range hopHints {
		alreadyIncluded[hopHint[0].ChannelID] = true
	}

	potentialHints, err := getPotentialHints(cfg)
	if err != nil {
		return nil, err
	}

	targetBandwidth := amtMSat * hopHintFactor
	selectedHints := selectHopHints(
		cfg, nHintsLeft, targetBandwidth, potentialHints,
		alreadyIncluded,
	)

	hopHints = append(hopHints, selectedHints...)
	return hopHints, nil
}

// buildBlindedPathCfg defines the various resources required to build a blinded
// payment path to this node.
type buildBlindedPathCfg struct {
	// findRoutes returns a set of routes to us that can be used for the
	// construction of blinded paths.
	findRoutes func(value lnwire.MilliSatoshi) ([]*route.Route, error)

	// fetchChannelEdgesByID attempts to look up the two directed edges for
	// the channel identified by the channel ID.
	fetchChannelEdgesByID func(chanID uint64) (*models.ChannelEdgeInfo,
		*models.ChannelEdgePolicy, *models.ChannelEdgePolicy, error)

	pathID []byte

	// value is the payment amount that must be routed. This will be used
	// for selecting appropriate routes to use for the blinded path.
	value lnwire.MilliSatoshi

	// policyMultiplier is the amount by which policy values for hops in a
	// blinded route will be bumped to avoid easy probing. For example, a
	// multiplier of 1.1 will bump all the values (base fee, fee rate and
	// CLTV delta by 10%).
	policyMultiplier float64
}

// buildBlindedPaymentPaths uses the passed config to construct a set of blinded
// payment paths that can be added to the invoice.
func buildBlindedPaymentPaths(cfg *buildBlindedPathCfg) (
	[]*zpay32.BlindedPaymentPath, error) {

	// Find some appropriate routes for the value to be routed.
	routes, err := cfg.findRoutes(cfg.value)
	if err != nil {
		return nil, err
	}

	paths := make([]*zpay32.BlindedPaymentPath, len(routes))

	// For each route returned, we will construct the associated blinded
	// payment path.
	for i, route := range routes {
		path, err := buildBlindedPaymentPath(cfg, route)
		if err != nil {
			return nil, err
		}

		paths[i] = path
	}

	return paths, nil
}

// buildBlindedPaymentPath uses the passed route and config to construct a
// blinded payment path that is ready to add to an invoice.
func buildBlindedPaymentPath(cfg *buildBlindedPathCfg, route *route.Route) (
	*zpay32.BlindedPaymentPath, error) {

	// TODO(elle): I think this is actually allowed... ie, intro node can
	//  be receiver. Are policy values then zero though?
	if len(route.Hops) == 0 {
		return nil, fmt.Errorf("blinded routes must contain at " +
			"least 1 hop")
	}

	var (
		// To account for the introduction node, we add 1 to the number
		// of hops in the route to get the full path length.
		pathLength = len(route.Hops) + 1
		relayInfo  = make([]*record.PaymentRelayInfo, 0, pathLength)
		hopInfo    = make([]*sphinx.HopInfo, 0, pathLength)

		// TODO(elle): actual values here...
		constraints = record.PaymentConstraints{
			MaxCltvExpiry:   math.MaxUint32,
			HtlcMinimumMsat: 0,
		}
	)

	// Before iterating through the route's hops, we'll first collect the
	// data that we need to send to the intro node.
	// The intro node's channel that we care about is the one named in the
	// first hop of the route.
	introChan := lnwire.NewShortChanIDFromInt(route.Hops[0].ChannelID)

	introPolicy, err := getNodeChannelPolicy(
		cfg, route.Hops[0].ChannelID, route.SourcePubKey,
	)
	if err != nil {
		return nil, err
	}

	bufferedIntroPolicy := addPolicyBuffer(
		introPolicy, cfg.policyMultiplier,
	)

	relayInfo = append(relayInfo, bufferedIntroPolicy)

	info, err := getHopInfo(
		introChan, route.SourcePubKey, bufferedIntroPolicy,
		&constraints,
	)
	if err != nil {
		return nil, err
	}

	hopInfo = append(hopInfo, info)

	var (
		minHTLC = introPolicy.MinHTLC
		maxHTLC = introPolicy.MaxHTLC
	)

	// With the introduction node sorted, we will now iterate through the
	// remaining hops and collect the payment relay info for each of them
	// excluding the very last hop since that hop won't be routed through.
	for i := 0; i < len(route.Hops)-1; i++ {
		scid := lnwire.NewShortChanIDFromInt(route.Hops[i+1].ChannelID)

		policy, err := getNodeChannelPolicy(
			cfg, route.Hops[i].ChannelID, route.Hops[i].PubKeyBytes,
		)
		if err != nil {
			return nil, err
		}

		// Update the allowed min & max HTLCs.
		if policy.MinHTLC > minHTLC {
			minHTLC = policy.MinHTLC
		}
		if policy.MaxHTLC < maxHTLC {
			maxHTLC = policy.MaxHTLC
		}

		bufferedRelayInfo := addPolicyBuffer(
			policy, cfg.policyMultiplier,
		)

		relayInfo = append(relayInfo, bufferedRelayInfo)

		info, err := getHopInfo(
			scid, route.Hops[i].PubKeyBytes, bufferedRelayInfo,
			&constraints,
		)
		if err != nil {
			return nil, err
		}

		hopInfo = append(hopInfo, info)
	}

	// For the final hop, we only need to set the Path ID.
	info, err = getFinalHopInfo(
		route.Hops[len(route.Hops)-1].PubKeyBytes, cfg.pathID,
	)
	if err != nil {
		return nil, err
	}

	hopInfo = append(hopInfo, info)

	// Derive an ephemeral session key.
	sessionKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	// Encrypt the hop info.
	blindedPath, err := sphinx.BuildBlindedPath(sessionKey, hopInfo)
	if err != nil {
		return nil, err
	}

	if len(blindedPath.BlindedHops) < 1 {
		return nil, fmt.Errorf("blinded path must have at least one " +
			"hop")
	}

	// Overwrite the introduction point's blinded pub key with the real
	// pub key since then we can use this more compact format in the
	// invoice without needing to encode the un-used blinded node pub key of
	// the intro node.
	blindedPath.BlindedHops[0].BlindedNodePub =
		blindedPath.IntroductionPoint

	baseFee, feeRate, cltvDelta := calcBlindedPathPolicies(relayInfo)

	// Now construct a z32 blinded path.
	return &zpay32.BlindedPaymentPath{
		FeeBaseMsat:                 baseFee,
		FeeRate:                     feeRate,
		CltvExpiryDelta:             cltvDelta,
		HTLCMinMsat:                 uint64(minHTLC),
		HTLCMaxMsat:                 uint64(maxHTLC),
		Features:                    lnwire.EmptyFeatureVector(),
		FirstEphemeralBlindingPoint: blindedPath.BlindingPoint,
		Hops:                        blindedPath.BlindedHops,
	}, nil
}

func calcBlindedPathPolicies(relayInfo []*record.PaymentRelayInfo) (uint32,
	uint32, uint16) {

	var (
		totalFeeBase uint32
		totalFeeProp uint32

		// TODO(elle): get actual config value for this.
		totalCltvDelta = uint16(chainreg.DefaultBitcoinTimeLockDelta)
	)
	for i := len(relayInfo) - 1; i >= 0; i-- {
		info := relayInfo[i]

		totalFeeBase = calcNextTotalBaseFee(
			totalFeeBase, info.BaseFee, info.FeeRate,
		)

		totalFeeProp = calcNextTotalFeeRate(totalFeeProp, info.FeeRate)

		totalCltvDelta += info.CltvExpiryDelta
	}

	return totalFeeBase, totalFeeProp, totalCltvDelta
}

func calcNextTotalBaseFee(currentTotal, hopBaseFee, hopFeeRate uint32) uint32 {
	const m = uint32(1000000)

	numerator := (hopBaseFee * m) +
		(currentTotal * (m + hopFeeRate)) + m - 1

	return numerator / m
}

func calcNextTotalFeeRate(currentTotal, hopFeeRate uint32) uint32 {
	const m = uint32(1000000)

	numerator := (currentTotal+hopFeeRate)*m + currentTotal*hopFeeRate + m - 1

	return numerator / m
}

func getNodeChannelPolicy(cfg *buildBlindedPathCfg, chanID uint64,
	nodeID route.Vertex) (*models.ChannelEdgePolicy, error) {

	_, update1, update2, err := cfg.fetchChannelEdgesByID(chanID)
	if err != nil {
		return nil, err
	}

	var policy *models.ChannelEdgePolicy
	if update1 != nil && !bytes.Equal(update1.ToNode[:], nodeID[:]) {
		policy = update1
	}
	if update2 != nil && !bytes.Equal(update2.ToNode[:], nodeID[:]) {
		policy = update2
	}

	if policy == nil {
		return nil, fmt.Errorf("no chan update for blinded path hop")
	}

	return policy, nil
}

func getFinalHopInfo(node route.Vertex, pathID []byte) (
	*sphinx.HopInfo, error) {

	hopData := record.NewFinalHopBlindedRouteData(pathID[:])

	nodeID, err := btcec.ParsePubKey(node[:])
	if err != nil {
		return nil, err
	}

	plaintxt, err := record.EncodeBlindedRouteData(hopData)
	if err != nil {
		return nil, err
	}

	return &sphinx.HopInfo{
		NodePub:   nodeID,
		PlainText: plaintxt,
	}, nil
}

// getHopInfo builds the record.BlindedRouteData struct for the given node,
// encodes that into a TLV stream and puts that into a sphinx.HopInfo along with
// the nodes real pub key.
func getHopInfo(scid lnwire.ShortChannelID, node route.Vertex,
	relayInfo *record.PaymentRelayInfo,
	constraints *record.PaymentConstraints) (*sphinx.HopInfo, error) {

	// Finally, we can wrap up the data we want to send to this hop.
	hopData := record.NewBlindedRouteData(
		scid, nil, *relayInfo, constraints, nil,
	)

	nodeID, err := btcec.ParsePubKey(node[:])
	if err != nil {
		return nil, err
	}

	plaintxt, err := record.EncodeBlindedRouteData(hopData)
	if err != nil {
		return nil, err
	}

	return &sphinx.HopInfo{
		NodePub:   nodeID,
		PlainText: plaintxt,
	}, nil
}

func addPolicyBuffer(policy *models.ChannelEdgePolicy,
	multiplier float64) *record.PaymentRelayInfo {

	return &record.PaymentRelayInfo{
		CltvExpiryDelta: uint16(math.Ceil(
			float64(policy.TimeLockDelta) * multiplier,
		)),
		FeeRate: uint32(math.Ceil(
			float64(policy.FeeProportionalMillionths) * multiplier,
		)),
		BaseFee: uint32(math.Ceil(
			float64(policy.FeeBaseMSat) * multiplier,
		)),
	}
}
