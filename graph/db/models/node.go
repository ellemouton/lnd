package models

import (
	"fmt"
	"image/color"
	"net"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tlv"
)

// Node represents an individual vertex/node within the channel graph.
// A node is connected to other nodes by one or more channel edges emanating
// from it. As the graph is directed, a node will also have an incoming edge
// attached to it for each outgoing edge.
type Node struct {
	Version lnwire.GossipVersion

	// PubKeyBytes is the raw bytes of the public key of the target node.
	PubKeyBytes [33]byte
	pubKey      *btcec.PublicKey

	// LastUpdate is the last time the vertex information for this node has
	// been updated.
	LastUpdate time.Time

	// LastBlockHeight is the block height that timestamps the last update
	// we received for this node. This is only used if this is a V2 node
	// announcement.
	LastBlockHeight uint32

	// Address is the TCP address this node is reachable over.
	Addresses []net.Addr

	// Color is the selected color for the node.
	Color fn.Option[color.RGBA]

	// Alias is a nick-name for the node. The alias can be used to confirm
	// a node's identity or to serve as a short ID for an address book.
	Alias fn.Option[string]

	// AuthSigBytes is the raw signature under the advertised public key
	// which serves to authenticate the attributes announced by this node.
	AuthSigBytes []byte

	// Features is the list of protocol features supported by this node.
	Features *lnwire.FeatureVector

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData []byte

	ExtraSignedFields map[uint64][]byte
}

type NodeV1Fields struct {
	// Address is the TCP address this node is reachable over.
	Addresses []net.Addr

	// AuthSigBytes is the raw signature under the advertised public key
	// which serves to authenticate the attributes announced by this node.
	AuthSigBytes []byte

	// Features is the list of protocol features supported by this node.
	Features *lnwire.RawFeatureVector

	// Color is the selected color for the node.
	Color color.RGBA

	// Alias is a nick-name for the node. The alias can be used to confirm
	// a node's identity or to serve as a short ID for an address book.
	Alias string

	// LastUpdate is the last time the vertex information for this node has
	// been updated.
	LastUpdate time.Time

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData []byte
}

func NewV1Node(pub route.Vertex, n *NodeV1Fields) *Node {
	return &Node{
		Version:      lnwire.GossipVersion1,
		PubKeyBytes:  pub,
		Addresses:    n.Addresses,
		AuthSigBytes: n.AuthSigBytes,
		Features: lnwire.NewFeatureVector(
			n.Features, lnwire.Features,
		),
		Color:           fn.Some(n.Color),
		Alias:           fn.Some(n.Alias),
		LastUpdate:      n.LastUpdate,
		ExtraOpaqueData: n.ExtraOpaqueData,
	}
}

func NewV1ShellNode(pubKey route.Vertex) *Node {
	return NewShellNode(lnwire.GossipVersion1, pubKey)
}

func NewShellNode(v lnwire.GossipVersion, pubKey route.Vertex) *Node {
	return &Node{
		Version:     v,
		PubKeyBytes: pubKey,
		Features:    lnwire.NewFeatureVector(nil, lnwire.Features),
		LastUpdate:  time.Unix(0, 0),
	}
}

type NodeV2Fields struct {
	LastBlockHeight uint32

	// Address is the TCP address this node is reachable over.
	Addresses []net.Addr

	Color fn.Option[color.RGBA]

	// Alias is a nick-name for the node. The alias can be used to confirm
	// a node's identity or to serve as a short ID for an address book.
	Alias fn.Option[string]

	Signature []byte

	// Features is the list of protocol features supported by this node.
	Features *lnwire.RawFeatureVector

	ExtraSignedFields map[uint64][]byte
}

func NewV2Node(pub route.Vertex, n *NodeV2Fields) *Node {
	return &Node{
		Version:      lnwire.GossipVersion2,
		PubKeyBytes:  pub,
		Addresses:    n.Addresses,
		AuthSigBytes: n.Signature,
		Features: lnwire.NewFeatureVector(
			n.Features, lnwire.Features,
		),
		LastBlockHeight:   n.LastBlockHeight,
		Color:             n.Color,
		Alias:             n.Alias,
		LastUpdate:        time.Unix(0, 0),
		ExtraSignedFields: n.ExtraSignedFields,
	}
}

func (n *Node) HaveAnnouncement() bool {
	return len(n.AuthSigBytes) > 0
}

// PubKey is the node's long-term identity public key. This key will be used to
// authenticated any advertisements/updates sent by the node.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (n *Node) PubKey() (*btcec.PublicKey, error) {
	if n.pubKey != nil {
		return n.pubKey, nil
	}

	key, err := btcec.ParsePubKey(n.PubKeyBytes[:])
	if err != nil {
		return nil, err
	}
	n.pubKey = key

	return key, nil
}

func (n *Node) nodeAnn1(signed bool) (*lnwire.NodeAnnouncement1, error) {
	if n.Version != lnwire.GossipVersion1 {
		return nil, fmt.Errorf("node with version %s cannot be cast "+
			"into a V1 node announcement", n.Version)
	}

	alias, err := lnwire.NewNodeAlias(n.Alias.UnwrapOr(""))
	if err != nil {
		return nil, err
	}

	nodeAnn := &lnwire.NodeAnnouncement1{
		Features:        n.Features.RawFeatureVector,
		NodeID:          n.PubKeyBytes,
		RGBColor:        n.Color.UnwrapOr(color.RGBA{}),
		Alias:           alias,
		Addresses:       n.Addresses,
		Timestamp:       uint32(n.LastUpdate.Unix()),
		ExtraOpaqueData: n.ExtraOpaqueData,
	}

	if !signed {
		return nodeAnn, nil
	}

	sig, err := lnwire.NewSigFromECDSARawSignature(n.AuthSigBytes)
	if err != nil {
		return nil, err
	}

	nodeAnn.Signature = sig

	return nodeAnn, nil
}

func (n *Node) nodeAnn2(signed bool) (*lnwire.NodeAnnouncement2, error) {
	nodeAnn := &lnwire.NodeAnnouncement2{
		Features: tlv.NewRecordT[tlv.TlvType0](
			*n.Features.RawFeatureVector,
		),
		BlockHeight: tlv.NewPrimitiveRecord[tlv.TlvType2](
			n.LastBlockHeight,
		),
		NodeID: tlv.NewPrimitiveRecord[tlv.TlvType6, [33]byte](
			n.PubKeyBytes,
		),
		ExtraSignedFields: n.ExtraSignedFields,
	}

	err := nodeAnn.SetAddrs(n.Addresses)
	if err != nil {
		return nil, err
	}

	n.Alias.WhenSome(func(s string) {
		var a lnwire.NodeAlias
		a, err = lnwire.NewNodeAlias(s)
		if err != nil {
			return
		}
		aliasRecord := tlv.ZeroRecordT[tlv.TlvType3, []byte]()
		aliasRecord.Val = a[:]
		nodeAnn.Alias = tlv.SomeRecordT(aliasRecord)
	})
	if err != nil {
		return nil, err
	}

	n.Color.WhenSome(func(rgba color.RGBA) {
		colorRecord := tlv.ZeroRecordT[tlv.TlvType1, lnwire.Color]()
		colorRecord.Val = lnwire.Color(rgba)
		nodeAnn.Color = tlv.SomeRecordT(colorRecord)
	})

	if !signed {
		return nodeAnn, nil
	}

	sig, err := input.ParseSignature(n.AuthSigBytes)
	if err != nil {
		return nil, err
	}

	nodeAnn.Signature.Val, err = lnwire.NewSigFromSignature(sig)
	if err != nil {
		return nil, err
	}

	return nodeAnn, nil
}

// NodeAnnouncement retrieves the latest node announcement of the node.
func (n *Node) NodeAnnouncement(signed bool) (lnwire.NodeAnnouncement, error) {
	if !n.HaveAnnouncement() {
		return nil, fmt.Errorf("node does not have node announcement")
	}

	switch n.Version {
	case lnwire.GossipVersion1:
		return n.nodeAnn1(signed)
	case lnwire.GossipVersion2:
		return n.nodeAnn2(signed)

	default:
		return nil, fmt.Errorf("unknown gossip version: %s", n.Version)
	}
}

// NodeFromWireAnnouncement creates a Node instance from an
// lnwire.NodeAnnouncement1 message.
func NodeFromWireAnnouncement(ann lnwire.NodeAnnouncement) (*Node, error) {
	switch msg := ann.(type) {
	case *lnwire.NodeAnnouncement1:
		timestamp := time.Unix(int64(msg.Timestamp), 0)

		return NewV1Node(
			msg.NodeID,
			&NodeV1Fields{
				LastUpdate:      timestamp,
				Addresses:       msg.Addresses,
				Alias:           msg.Alias.String(),
				AuthSigBytes:    msg.Signature.ToSignatureBytes(),
				Features:        msg.Features,
				Color:           msg.RGBColor,
				ExtraOpaqueData: msg.ExtraOpaqueData,
			},
		), nil

	case *lnwire.NodeAnnouncement2:
		f := &NodeV2Fields{
			LastBlockHeight:   msg.BlockHeight.Val,
			Signature:         msg.Signature.Val.ToSignatureBytes(),
			Features:          &msg.Features.Val,
			ExtraSignedFields: msg.ExtraSignedFields,
		}
		msg.Color.WhenSome(func(r tlv.RecordT[tlv.TlvType1, lnwire.Color]) {
			f.Color = fn.Some(color.RGBA(r.Val))
		})
		msg.Alias.WhenSome(func(r tlv.RecordT[tlv.TlvType3, []byte]) {
			f.Alias = fn.Some(string(r.Val))
		})

		msg.IPV4Addrs.WhenSome(func(r tlv.RecordT[tlv.TlvType5, lnwire.IPV4Addrs]) {
			for _, addr := range r.Val {
				f.Addresses = append(f.Addresses, addr)
			}
		})
		msg.IPV6Addrs.WhenSome(func(r tlv.RecordT[tlv.TlvType7, lnwire.IPV6Addrs]) {
			for _, addr := range r.Val {
				f.Addresses = append(f.Addresses, addr)
			}
		})
		msg.TorV3Addrs.WhenSome(func(r tlv.RecordT[tlv.TlvType9, lnwire.TorV3Addrs]) {
			for _, addr := range r.Val {
				f.Addresses = append(f.Addresses, addr)
			}
		})
		// TODO(elle): DNS addrs

		return NewV2Node(msg.NodeID.Val, f), nil

	default:
		return nil, fmt.Errorf("unknown node type: %T", msg)
	}
}
