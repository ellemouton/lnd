package netann

import (
	"bytes"
	"fmt"

	"github.com/lightningnetwork/lnd/channeldb"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/lnwire"
)

// CreateChanAnnouncement is a helper function which creates all channel
// announcements given the necessary channel related database items. This
// function is used to transform out database structs into the corresponding wire
// structs for announcing new channels to other peers, or simply syncing up a
// peer's initial routing table upon connect.
func CreateChanAnnouncement(chanProof models.ChannelAuthProof,
	chanInfo models.ChannelEdgeInfo,
	e1, e2 *channeldb.ChannelEdgePolicy1) (lnwire.ChannelAnnouncement,
	*lnwire.ChannelUpdate1, *lnwire.ChannelUpdate1, error) {

	switch proof := chanProof.(type) {
	case *channeldb.ChannelAuthProof1:
		info, ok := chanInfo.(*channeldb.ChannelEdgeInfo1)
		if !ok {
			return nil, nil, nil, fmt.Errorf("expected type "+
				"ChannelEdgeInfo1 to be paired with "+
				"ChannelAuthProof1, got: %T", chanInfo)
		}

		return createChanAnnouncement1(proof, info, e1, e2)

	default:
		return nil, nil, nil, fmt.Errorf("unhandled "+
			"channeldb.ChannelAuthProof type: %T", chanProof)
	}
}

func createChanAnnouncement1(chanProof *channeldb.ChannelAuthProof1,
	chanInfo *channeldb.ChannelEdgeInfo1,
	e1, e2 *channeldb.ChannelEdgePolicy1) (lnwire.ChannelAnnouncement,
	*lnwire.ChannelUpdate1, *lnwire.ChannelUpdate1, error) {

	// First, using the parameters of the channel, along with the channel
	// authentication chanProof, we'll create re-create the original
	// authenticated channel announcement.
	chanID := lnwire.NewShortChanIDFromInt(chanInfo.ChannelID)
	chanAnn := &lnwire.ChannelAnnouncement1{
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
	var edge1Ann, edge2Ann *lnwire.ChannelUpdate1
	if e1 != nil {
		edge1Ann, err = ChannelUpdateFromEdge(chanInfo, e1)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if e2 != nil {
		edge2Ann, err = ChannelUpdateFromEdge(chanInfo, e2)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	return chanAnn, edge1Ann, edge2Ann, nil
}
