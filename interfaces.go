package lnd

import (
	graphsources "github.com/lightningnetwork/lnd/graph/sources"
)

// Providers is an interface that LND itself can satisfy.
type Providers interface {
	// GraphSource can be used to obtain the graph source that this LND node
	// provides.
	GraphSource() (graphsources.GraphSource, error)
}

// A compile-time check to ensure that LND's main server struct satisfies the
// Provider interface.
var _ Providers = (*server)(nil)

// GraphSource returns this LND nodes graph DB as a GraphSource.
//
// NOTE: this method is part of the Providers interface.
func (s *server) GraphSource() (graphsources.GraphSource, error) {
	return s.graphSource, nil
}
