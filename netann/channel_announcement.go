package netann

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/lnwire"
)

// ChanAnn is an interface representing the info that the handleChanAnnouncement
// method needs to know about a channel announcement. This abstraction allows
// the handleChanAnnouncement method to work for multiple types of channel
// announcements.
type ChanAnn interface {
	// SCID returns the short channel ID of the channel being announced.
	SCID() lnwire.ShortChannelID

	// ChainHash returns the hash identifying the chain that the channel
	// was opened on.
	ChainHash() chainhash.Hash

	// Name returns the underlying lnwire message name.
	Name() string

	// Msg returns the underlying lnwire.Message.
	Msg() lnwire.Message

	// BuildProof converts the lnwire.Message into a
	// channeldb.ChannelAuthProof.
	BuildProof() *channeldb.ChannelAuthProof

	// BuildEdgeInfo converts the lnwire.Message into a
	// channeldb.ChannelEdgeInfo.
	BuildEdgeInfo() (*channeldb.ChannelEdgeInfo, error)

	lnwire.Message
}

// ChanAnn1 is an implementation of the ChanAnn interface which represents the
// lnwire.ChannelAnnouncement message used for P2WSH channels.
type ChanAnn1 struct {
	*lnwire.ChannelAnnouncement
}

// SCID returns the short channel ID of the channel being announced.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn1) SCID() lnwire.ShortChannelID {
	return c.ShortChannelID
}

// ChainHash returns the hash identifying the chain that the channel was opened
// on.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn1) ChainHash() chainhash.Hash {
	return c.ChannelAnnouncement.ChainHash
}

// Name returns the underlying lnwire message name.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn1) Name() string {
	return "ChannelAnnouncement"
}

// BuildProof converts the lnwire.Message into a channeldb.ChannelAuthProof.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn1) BuildProof() *channeldb.ChannelAuthProof {
	return &channeldb.ChannelAuthProof{
		NodeSig1Bytes:    c.NodeSig1.ToSignatureBytes(),
		NodeSig2Bytes:    c.NodeSig2.ToSignatureBytes(),
		BitcoinSig1Bytes: c.BitcoinSig1.ToSignatureBytes(),
		BitcoinSig2Bytes: c.BitcoinSig2.ToSignatureBytes(),
	}
}

// BuildEdgeInfo converts the lnwire.Message into a channeldb.ChannelEdgeInfo.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn1) BuildEdgeInfo() (*channeldb.ChannelEdgeInfo, error) {
	var featureBuf bytes.Buffer
	if err := c.Features.Encode(&featureBuf); err != nil {
		return nil, fmt.Errorf("unable to encode features: %v", err)
	}

	return &channeldb.ChannelEdgeInfo{
		ChannelID:        c.ShortChannelID.ToUint64(),
		ChainHash:        c.ChainHash(),
		NodeKey1Bytes:    c.NodeID1,
		NodeKey2Bytes:    c.NodeID2,
		BitcoinKey1Bytes: c.BitcoinKey1,
		BitcoinKey2Bytes: c.BitcoinKey2,
		Features:         featureBuf.Bytes(),
		ExtraOpaqueData:  c.ExtraOpaqueData,
	}, nil
}

// Msg returns the underlying lnwire.Message.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn1) Msg() lnwire.Message {
	return c.ChannelAnnouncement
}

var _ ChanAnn = (*ChanAnn1)(nil)

// ChanAnn2 is an implementation of the ChanAnn interface which represents the
// lnwire.ChannelAnnouncement2 message used for taproot channels.
type ChanAnn2 struct {
	*lnwire.ChannelAnnouncement2
}

// SCID returns the short channel ID of the channel being announced.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn2) SCID() lnwire.ShortChannelID {
	return c.ShortChannelID
}

// ChainHash returns the hash identifying the chain that the channel was opened
// on.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn2) ChainHash() chainhash.Hash {
	return c.ChannelAnnouncement2.ChainHash
}

// Name returns the underlying lnwire message name.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn2) Name() string {
	return "ChannelAnnouncement2"
}

// BuildProof converts the lnwire.Message into a channeldb.ChannelAuthProof.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn2) BuildProof() *channeldb.ChannelAuthProof {
	return &channeldb.ChannelAuthProof{
		SchnorrSigBytes: c.Signature.ToSignatureBytes(),
	}
}

// BuildEdgeInfo converts the lnwire.Message into a channeldb.ChannelEdgeInfo.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn2) BuildEdgeInfo() (*channeldb.ChannelEdgeInfo, error) {
	var featureBuf bytes.Buffer
	if err := c.Features.Encode(&featureBuf); err != nil {
		return nil, fmt.Errorf("unable to encode features: %v", err)
	}

	edge := &channeldb.ChannelEdgeInfo{
		ChannelID:       c.ShortChannelID.ToUint64(),
		ChainHash:       c.ChainHash(),
		NodeKey1Bytes:   c.NodeID1,
		NodeKey2Bytes:   c.NodeID2,
		Features:        featureBuf.Bytes(),
		IsTaproot:       true,
		ExtraOpaqueData: c.ExtraOpaqueData,
	}

	if c.BitcoinKey1 != nil && c.BitcoinKey2 != nil {
		edge.BitcoinKey1Bytes = *c.BitcoinKey1
		edge.BitcoinKey2Bytes = *c.BitcoinKey2
	}

	return edge, nil
}

// Msg returns the underlying lnwire.Message.
//
// NOTE: this is part of the ChanAnn interface.
func (c *ChanAnn2) Msg() lnwire.Message {
	return c.ChannelAnnouncement2
}

var _ ChanAnn = (*ChanAnn2)(nil)

func CreateChanAnnouncement(chanProof *channeldb.ChannelAuthProof,
	chanInfo *channeldb.ChannelEdgeInfo,
	e1, e2 channeldb.ChanEdgePolicy) (ChanAnn, lnwire.ChanUpdate,
	lnwire.ChanUpdate, error) {

	if chanInfo.IsTaproot {
		edge1, ok := e1.(*channeldb.ChanEdgePolicy2)
		if !ok {
			return nil, nil, nil, fmt.Errorf("wrong edge policy " +
				"type for taproot chan")
		}

		edge2, ok := e2.(*channeldb.ChanEdgePolicy2)
		if !ok {
			return nil, nil, nil, fmt.Errorf("wrong edge policy " +
				"type for taproot chan")
		}

		ann, upd1, upd2, err := createChanAnnouncement2(
			chanProof, chanInfo, edge1, edge2,
		)
		if err != nil {
			return nil, nil, nil, err
		}

		return &ChanAnn2{ann}, upd1, upd2, nil
	}

	edge1, ok := e1.(*channeldb.ChannelEdgePolicy1)
	if !ok {
		return nil, nil, nil, fmt.Errorf("wrong edge policy " +
			"type for legacy chan")
	}

	edge2, ok := e2.(*channeldb.ChannelEdgePolicy1)
	if !ok {
		return nil, nil, nil, fmt.Errorf("wrong edge policy " +
			"type for legacy chan")
	}

	ann, upd1, upd2, err := createChanAnnouncement1(
		chanProof, chanInfo, edge1, edge2,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	return &ChanAnn1{ann}, upd1, upd2, nil
}

// createChanAnnouncement1 is a helper function which creates all channel
// announcements given the necessary channel related database items. This
// function is used to transform out database structs into the corresponding wire
// structs for announcing new channels to other peers, or simply syncing up a
// peer's initial routing table upon connect.
func createChanAnnouncement1(chanProof *channeldb.ChannelAuthProof,
	chanInfo *channeldb.ChannelEdgeInfo,
	e1, e2 *channeldb.ChannelEdgePolicy1) (*lnwire.ChannelAnnouncement,
	*lnwire.ChannelUpdate, *lnwire.ChannelUpdate, error) {

	// First, using the parameters of the channel, along with the channel
	// authentication chanProof, we'll create re-create the original
	// authenticated channel announcement.
	chanID := lnwire.NewShortChanIDFromInt(chanInfo.ChannelID)
	chanAnn := &lnwire.ChannelAnnouncement{
		ShortChannelID:  chanID,
		NodeID1:         chanInfo.NodeKey1Bytes,
		NodeID2:         chanInfo.NodeKey2Bytes,
		ChainHash:       chanInfo.ChainHash,
		BitcoinKey1:     chanInfo.BitcoinKey1Bytes,
		BitcoinKey2:     chanInfo.BitcoinKey2Bytes,
		Features:        lnwire.NewRawFeatureVector(),
		ExtraOpaqueData: chanInfo.ExtraOpaqueData,
	}

	err := chanAnn.Features.Decode(bytes.NewReader(chanInfo.Features))
	if err != nil {
		return nil, nil, nil, err
	}
	chanAnn.BitcoinSig1, err = lnwire.NewSigFromECDSARawSignature(
		chanProof.BitcoinSig1Bytes,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chanAnn.BitcoinSig2, err = lnwire.NewSigFromECDSARawSignature(
		chanProof.BitcoinSig2Bytes,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chanAnn.NodeSig1, err = lnwire.NewSigFromECDSARawSignature(
		chanProof.NodeSig1Bytes,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	chanAnn.NodeSig2, err = lnwire.NewSigFromECDSARawSignature(
		chanProof.NodeSig2Bytes,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// We'll unconditionally queue the channel's existence chanProof as it
	// will need to be processed before either of the channel update
	// networkMsgs.

	// Since it's up to a node's policy as to whether they advertise the
	// edge in a direction, we don't create an advertisement if the edge is
	// nil.
	var edge1Ann, edge2Ann *lnwire.ChannelUpdate
	if e1 != nil {
		edge1Ann, err = ChannelUpdate1FromEdge(chanInfo, e1)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if e2 != nil {
		edge2Ann, err = ChannelUpdate1FromEdge(chanInfo, e2)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return chanAnn, edge1Ann, edge2Ann, nil
}

func createChanAnnouncement2(chanProof *channeldb.ChannelAuthProof,
	chanInfo *channeldb.ChannelEdgeInfo,
	e1, e2 *channeldb.ChanEdgePolicy2) (*lnwire.ChannelAnnouncement2,
	*lnwire.ChannelUpdate2, *lnwire.ChannelUpdate2, error) {

	// First, using the parameters of the channel, along with the channel
	// authentication chanProof, we'll create re-create the original
	// authenticated channel announcement.
	chanID := lnwire.NewShortChanIDFromInt(chanInfo.ChannelID)
	chanAnn := &lnwire.ChannelAnnouncement2{
		ShortChannelID:  chanID,
		NodeID1:         chanInfo.NodeKey1Bytes,
		NodeID2:         chanInfo.NodeKey2Bytes,
		ChainHash:       chanInfo.ChainHash,
		BitcoinKey1:     &chanInfo.BitcoinKey1Bytes,
		BitcoinKey2:     &chanInfo.BitcoinKey2Bytes,
		Features:        *lnwire.NewRawFeatureVector(),
		ExtraOpaqueData: chanInfo.ExtraOpaqueData,
	}

	err := chanAnn.Features.Decode(bytes.NewReader(chanInfo.Features))
	if err != nil {
		return nil, nil, nil, err
	}

	chanAnn.Signature, err = lnwire.NewSigFromSchnorrRawSignature(
		chanProof.SchnorrSigBytes,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	// We'll unconditionally queue the channel's existence chanProof as it
	// will need to be processed before either of the channel update
	// networkMsgs.

	// Since it's up to a node's policy as to whether they advertise the
	// edge in a direction, we don't create an advertisement if the edge is
	// nil.
	var edge1Ann, edge2Ann *lnwire.ChannelUpdate2
	if e1 != nil {
		edge1Ann = ChannelUpdate2FromEdge(chanInfo, e1)
	}
	if e2 != nil {
		edge2Ann = ChannelUpdate2FromEdge(chanInfo, e2)
	}

	return chanAnn, edge1Ann, edge2Ann, nil
}
