package sources

import "github.com/lightningnetwork/lnd/routing"

// GraphSource defines the read-only graph interface required by LND for graph
// related queries.
type GraphSource interface {
	routing.GraphSessionFactory
}
