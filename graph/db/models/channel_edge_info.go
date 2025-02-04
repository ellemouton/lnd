package models

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcec/v2/schnorr/musig2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/fn/v2"
	"github.com/lightningnetwork/lnd/input"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

// ChannelEdgeInfo represents a fully authenticated channel along with all its
// unique attributes. Once an authenticated channel announcement has been
// processed on the network, then an instance of ChannelEdgeInfo encapsulating
// the channels attributes is stored. The other portions relevant to routing
// policy of a channel are stored within a ChannelEdgePolicy for each direction
// of the channel.
type ChannelEdgeInfo struct {
	// ChannelID is the unique channel ID for the channel. The first 3
	// bytes are the block height, the next 3 the index within the block,
	// and the last 2 bytes are the output index for the channel.
	ChannelID uint64

	// ChainHash is the hash that uniquely identifies the chain that this
	// channel was opened within.
	//
	// TODO(roasbeef): need to modify db keying for multi-chain
	//  * must add chain hash to prefix as well
	ChainHash chainhash.Hash

	// NodeKey1Bytes is the raw public key of the first node.
	NodeKey1Bytes [33]byte
	nodeKey1      *btcec.PublicKey

	// NodeKey2Bytes is the raw public key of the first node.
	NodeKey2Bytes [33]byte
	nodeKey2      *btcec.PublicKey

	// BitcoinKey1Bytes is the raw public key of the first node.
	BitcoinKey1Bytes [33]byte
	bitcoinKey1      *btcec.PublicKey

	// BitcoinKey2Bytes is the raw public key of the first node.
	BitcoinKey2Bytes [33]byte
	bitcoinKey2      *btcec.PublicKey

	// Features is an opaque byte slice that encodes the set of channel
	// specific features that this channel edge supports.
	Features []byte

	// AuthProof is the authentication proof for this channel. This proof
	// contains a set of signatures binding four identities, which attests
	// to the legitimacy of the advertised channel.
	AuthProof *ChannelAuthProof

	// ChannelPoint is the funding outpoint of the channel. This can be
	// used to uniquely identify the channel within the channel graph.
	ChannelPoint wire.OutPoint

	// Capacity is the total capacity of the channel, this is determined by
	// the value output in the outpoint that created this channel.
	Capacity btcutil.Amount

	// ExtraOpaqueData is the set of data that was appended to this
	// message, some of which we may not actually know how to iterate or
	// parse. By holding onto this data, we ensure that we're able to
	// properly validate the set of signatures that cover these new fields,
	// and ensure we're able to make upgrades to the network in a forwards
	// compatible manner.
	ExtraOpaqueData []byte
}

func (c *ChannelEdgeInfo) GetOutpoint() wire.OutPoint {
	return c.ChannelPoint
}

func (c *ChannelEdgeInfo) GetChainHash() (chainhash.Hash, error) {
	return c.ChainHash, nil
}

func (c *ChannelEdgeInfo) HaveSig() bool {
	return c.AuthProof != nil
}

func (c *ChannelEdgeInfo) FundingScript() ([]byte, error) {
	witnessScript, err := input.GenMultiSigScript(
		c.BitcoinKey1Bytes[:], c.BitcoinKey2Bytes[:],
	)
	if err != nil {
		return nil, err
	}

	return input.WitnessScriptHash(witnessScript)
}

func (c *ChannelEdgeInfo) Protocol() lnwire.Protocol {
	return lnwire.V1Protocol
}

func (c *ChannelEdgeInfo) Node1() route.Vertex {
	return c.NodeKey1Bytes
}

func (c *ChannelEdgeInfo) Node2() route.Vertex {
	return c.NodeKey2Bytes
}

func (c *ChannelEdgeInfo) ChanID() uint64 {
	return c.ChannelID
}

func (c *ChannelEdgeInfo) Cap() btcutil.Amount {
	return c.Capacity
}

// AddNodeKeys is a setter-like method that can be used to replace the set of
// keys for the target ChannelEdgeInfo.
func (c *ChannelEdgeInfo) AddNodeKeys(nodeKey1, nodeKey2, bitcoinKey1,
	bitcoinKey2 *btcec.PublicKey) {

	c.nodeKey1 = nodeKey1
	copy(c.NodeKey1Bytes[:], c.nodeKey1.SerializeCompressed())

	c.nodeKey2 = nodeKey2
	copy(c.NodeKey2Bytes[:], nodeKey2.SerializeCompressed())

	c.bitcoinKey1 = bitcoinKey1
	copy(c.BitcoinKey1Bytes[:], c.bitcoinKey1.SerializeCompressed())

	c.bitcoinKey2 = bitcoinKey2
	copy(c.BitcoinKey2Bytes[:], bitcoinKey2.SerializeCompressed())
}

// NodeKey1 is the identity public key of the "first" node that was involved in
// the creation of this channel. A node is considered "first" if the
// lexicographical ordering the its serialized public key is "smaller" than
// that of the other node involved in channel creation.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) NodeKey1() (*btcec.PublicKey, error) {
	if c.nodeKey1 != nil {
		return c.nodeKey1, nil
	}

	key, err := btcec.ParsePubKey(c.NodeKey1Bytes[:])
	if err != nil {
		return nil, err
	}
	c.nodeKey1 = key

	return key, nil
}

// NodeKey2 is the identity public key of the "second" node that was involved in
// the creation of this channel. A node is considered "second" if the
// lexicographical ordering the its serialized public key is "larger" than that
// of the other node involved in channel creation.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) NodeKey2() (*btcec.PublicKey, error) {
	if c.nodeKey2 != nil {
		return c.nodeKey2, nil
	}

	key, err := btcec.ParsePubKey(c.NodeKey2Bytes[:])
	if err != nil {
		return nil, err
	}
	c.nodeKey2 = key

	return key, nil
}

// BitcoinKey1 is the Bitcoin multi-sig key belonging to the first node, that
// was involved in the funding transaction that originally created the channel
// that this struct represents.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) BitcoinKey1() (*btcec.PublicKey, error) {
	if c.bitcoinKey1 != nil {
		return c.bitcoinKey1, nil
	}

	key, err := btcec.ParsePubKey(c.BitcoinKey1Bytes[:])
	if err != nil {
		return nil, err
	}
	c.bitcoinKey1 = key

	return key, nil
}

// BitcoinKey2 is the Bitcoin multi-sig key belonging to the second node, that
// was involved in the funding transaction that originally created the channel
// that this struct represents.
//
// NOTE: By having this method to access an attribute, we ensure we only need
// to fully deserialize the pubkey if absolutely necessary.
func (c *ChannelEdgeInfo) BitcoinKey2() (*btcec.PublicKey, error) {
	if c.bitcoinKey2 != nil {
		return c.bitcoinKey2, nil
	}

	key, err := btcec.ParsePubKey(c.BitcoinKey2Bytes[:])
	if err != nil {
		return nil, err
	}
	c.bitcoinKey2 = key

	return key, nil
}

// OtherNodeKeyBytes returns the node key bytes of the other end of the channel.
func (c *ChannelEdgeInfo) OtherNodeKeyBytes(thisNodeKey []byte) (
	[33]byte, error) {

	switch {
	case bytes.Equal(c.NodeKey1Bytes[:], thisNodeKey):
		return c.NodeKey2Bytes, nil
	case bytes.Equal(c.NodeKey2Bytes[:], thisNodeKey):
		return c.NodeKey1Bytes, nil
	default:
		return [33]byte{}, fmt.Errorf("node not participating in " +
			"this channel")
	}
}

var _ Channel = (*ChannelEdgeInfo)(nil)

type Channel2 struct {
	ChannelID uint64
	Outpoint  wire.OutPoint
	Node1Key  route.Vertex
	Node2Key  route.Vertex

	Bitcoin1Key    fn.Option[route.Vertex]
	Bitcoin2Key    fn.Option[route.Vertex]
	MerkleRootHash fn.Option[chainhash.Hash]

	Capacity btcutil.Amount
	Features *lnwire.FeatureVector

	AuthProof fn.Option[*ChannelAuthProof2]

	// FundingPkScript is the funding transaction's pk script. We persist
	// this since there are some cases in which this will not be derivable
	// using the contents of the announcement. In that case, we still want
	// quick access to the funding script so that we can register for spend
	// notifications.
	FundingPkScript fn.Option[[]byte]

	// TODO: rename to just be AllFields or something like that.
	AllSignedFields map[uint64][]byte
}

func (c *Channel2) GetOutpoint() wire.OutPoint {
	return c.Outpoint
}

func (c *Channel2) GetChainHash() (chainhash.Hash, error) {
	// TODO(elle): remove hack. inefficient. Maybe cache the announcement so
	// we have easy access to any field?
	var chanAnn lnwire.ChannelAnnouncement2
	err := lnwire.DecodePureTLVFromRecords(&chanAnn, c.AllSignedFields)
	if err != nil {
		return chainhash.Hash{}, err
	}

	return chanAnn.ChainHash.Val, nil
}

func (c *Channel2) HaveSig() bool {
	return c.AuthProof.IsSome()
}

func (c *Channel2) FundingScript() ([]byte, error) {
	var (
		pubKey1 *btcec.PublicKey
		pubKey2 *btcec.PublicKey
		err     error
	)
	c.Bitcoin1Key.WhenSome(func(key route.Vertex) {
		pubKey1, err = btcec.ParsePubKey(key[:])
	})
	if err != nil {
		return nil, err
	}

	c.Bitcoin2Key.WhenSome(func(key route.Vertex) {
		pubKey2, err = btcec.ParsePubKey(key[:])
	})
	if err != nil {
		return nil, err
	}

	// If both bitcoin keys are not present in the announcement, then we
	// should previously have stored the funding script found on-chain.
	if pubKey1 == nil || pubKey2 == nil {
		return c.FundingPkScript.UnwrapOrErr(fmt.Errorf(
			"expected a funding pk script since no bitcoin keys " +
				"were provided",
		))
	}

	// Initially we set the tweak to an empty byte array. If a merkle root
	// hash is provided in the announcement then we use that to set the
	// tweak but otherwise, the empty tweak will have the same effect as a
	// BIP86 tweak.
	var tweak []byte
	c.MerkleRootHash.WhenSome(func(hash chainhash.Hash) {
		tweak = hash[:]
	})

	// Calculate the internal key by computing the MuSig2 combination of the
	// two public keys.
	internalKey, _, _, err := musig2.AggregateKeys(
		[]*btcec.PublicKey{pubKey1, pubKey2}, true,
	)
	if err != nil {
		return nil, err
	}

	// Now, determine the tweak to be added to the internal key. If the
	// tweak is empty, then this will effectively be a BIP86 tweak.
	tapTweakHash := chainhash.TaggedHash(
		chainhash.TagTapTweak, schnorr.SerializePubKey(
			internalKey.FinalKey,
		), tweak,
	)

	// Compute the final output key.
	combinedKey, _, _, err := musig2.AggregateKeys(
		[]*btcec.PublicKey{pubKey1, pubKey2}, true,
		musig2.WithKeyTweaks(musig2.KeyTweakDesc{
			Tweak:   *tapTweakHash,
			IsXOnly: true,
		}),
	)
	if err != nil {
		return nil, err
	}

	// Now that we have the combined key, we can create a taproot pkScript
	// from this, and then make the txout given the amount.
	fundingScript, err := input.PayToTaprootScript(combinedKey.FinalKey)
	if err != nil {
		return nil, fmt.Errorf("unable to make taproot pkscript: %w",
			err)
	}

	return fundingScript, nil
}

func (c *Channel2) Protocol() lnwire.Protocol {
	return lnwire.V2Protocol
}

func (c *Channel2) Node1() route.Vertex {
	return c.Node1Key
}

func (c *Channel2) Node2() route.Vertex {
	return c.Node2Key
}

func (c *Channel2) ChanID() uint64 {
	return c.ChannelID
}

func (c *Channel2) Cap() btcutil.Amount {
	return c.Capacity
}

var _ Channel = (*Channel2)(nil)

type Channel interface {
	Node1() route.Vertex
	Node2() route.Vertex
	ChanID() uint64
	Cap() btcutil.Amount
	Protocol() lnwire.Protocol
	FundingScript() ([]byte, error)
	GetOutpoint() wire.OutPoint
	HaveSig() bool
	GetChainHash() (chainhash.Hash, error)
}
