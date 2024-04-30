package lncfg

import "fmt"

const (
	// DefaultHoldInvoiceExpiryDelta defines the number of blocks before the
	// expiry height of a hold invoice's htlc that lnd will automatically
	// cancel the invoice to prevent the channel from force closing. This
	// value *must* be greater than DefaultIncomingBroadcastDelta to prevent
	// force closes.
	DefaultHoldInvoiceExpiryDelta = DefaultIncomingBroadcastDelta + 2

	// DefaultMinNumBlindedPathHops is the minimum number of hops to include
	// in a blinded payment path.
	DefaultMinNumBlindedPathHops = 1

	// DefaultMaxNumBlindedPathHops is the maximum number of hops to include
	// in a blinded payment path.
	DefaultMaxNumBlindedPathHops = 2

	// DefaultMaxNumPaths is the maximum number of different blinded payment
	// paths to include in an invoice.
	DefaultMaxNumPaths = 3
)

// Invoices holds the configuration options for invoices.
//
//nolint:lll
type Invoices struct {
	HoldExpiryDelta uint32 `long:"holdexpirydelta" description:"The number of blocks before a hold invoice's htlc expires that the invoice should be canceled to prevent a force close. Force closes will not be prevented if this value is not greater than DefaultIncomingBroadcastDelta."`

	BlindedPaths BlindedPaths `group:"blinding" namespace:"blinding"`
}

type BlindedPaths struct {
	MinNumHops  uint8 `long:"min-num-hops"`
	MaxNumHops  uint8 `long:"max-num-hops"`
	MaxNumPaths uint8 `long:"max-num-paths"`
}

func (i *Invoices) Validate() error {
	// Log a warning if our expiry delta is not greater than our incoming
	// broadcast delta. We do not fail here because this value may be set
	// to zero to intentionally keep lnd's behavior unchanged from when we
	// didn't auto-cancel these invoices.
	if i.HoldExpiryDelta <= DefaultIncomingBroadcastDelta {
		log.Warnf("Invoice hold expiry delta: %v <= incoming "+
			"delta: %v, accepted hold invoices will force close "+
			"channels if they are not canceled manually",
			i.HoldExpiryDelta, DefaultIncomingBroadcastDelta)
	}

	if i.BlindedPaths.MinNumHops > i.BlindedPaths.MaxNumHops {
		return fmt.Errorf("the minimum number of blinded path hops " +
			"must be smaller than or equal to the maximum")
	}

	return nil
}
