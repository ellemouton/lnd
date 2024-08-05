package lncfg

import "fmt"

// Routing holds the configuration options for routing.
//
//nolint:lll
type Routing struct {
	AssumeChannelValid bool `long:"assumechanvalid" description:"DEPRECATED: Skip checking channel spentness during graph validation. This speedup comes at the risk of using an unvalidated view of the network for routing. (default: false)" hidden:"true"`

	StrictZombiePruning bool `long:"strictgraphpruning" description:"If true, then the graph will be pruned more aggressively for zombies. In practice this means that edges with a single stale edge will be considered a zombie."`

	BlindedPaths BlindedPaths `group:"blinding" namespace:"blinding"`
}

// BlindedPaths holds the configuration options for blinded path construction.
//
//nolint:lll
type BlindedPaths struct {
	PolicyIncreaseMultiplier float64 `long:"policy-increase-multiplier" description:"The amount by which to increase certain policy values of hops on a blinded path in order to add a probing buffer."`
	PolicyDecreaseMultiplier float64 `long:"policy-decrease-multiplier" description:"The amount by which to decrease certain policy values of hops on a blinded path in order to add a probing buffer."`
}

// Validate checks that the various routing config options are sane.
//
// NOTE: this is part of the Validator interface.
func (r *Routing) Validate() error {
	if r.BlindedPaths.PolicyIncreaseMultiplier < 1 {
		return fmt.Errorf("the blinded route policy increase " +
			"multiplier must be greater than or equal to 1")
	}

	if r.BlindedPaths.PolicyDecreaseMultiplier > 1 ||
		r.BlindedPaths.PolicyDecreaseMultiplier < 0 {

		return fmt.Errorf("the blinded route policy decrease " +
			"multiplier must be in the range (0,1]")
	}

	return nil
}
