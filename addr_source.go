package lnd

import (
	"net"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/graph"
	"github.com/lightningnetwork/lnd/graph/graphdb"
	"github.com/lightningnetwork/lnd/routing/route"
)

type addrSource struct {
	graph         graph.DB
	linkNodeAddrs func(nodePub *btcec.PublicKey) ([]net.Addr, error)
}

// AddrsForNode consults the graph and channel database for all addresses known
// to the passed node public key.
func (d *addrSource) AddrsForNode(nodePub *btcec.PublicKey) ([]net.Addr,
	error) {

	linkNodeAddrs, err := d.linkNodeAddrs(nodePub)
	if err != nil {
		return nil, err
	}

	// We'll also query the graph for this peer to see if they have any
	// addresses that we don't currently have stored within the link node
	// database.
	pubKey, err := route.NewVertexFromBytes(nodePub.SerializeCompressed())
	if err != nil {
		return nil, err
	}
	graphNode, err := d.graph.FetchLightningNode(pubKey)
	if err != nil && err != graphdb.ErrGraphNodeNotFound {
		return nil, err
	} else if err == graphdb.ErrGraphNodeNotFound {
		// If the node isn't found, then that's OK, as we still have the
		// link node data. But any other error needs to be returned.
		graphNode = &graphdb.LightningNode{}
	}

	// Now that we have both sources of addrs for this node, we'll use a
	// map to de-duplicate any addresses between the two sources, and
	// produce a final list of the combined addrs.
	addrs := make(map[string]net.Addr)
	for _, addr := range linkNodeAddrs {
		addrs[addr.String()] = addr
	}
	for _, addr := range graphNode.Addresses {
		addrs[addr.String()] = addr
	}
	dedupedAddrs := make([]net.Addr, 0, len(addrs))
	for _, addr := range addrs {
		dedupedAddrs = append(dedupedAddrs, addr)
	}

	return dedupedAddrs, nil
}
