package channeldb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/channeldb/models"
	"github.com/lightningnetwork/lnd/kvdb"
)

func putChanEdgeInfo(edgeIndex kvdb.RwBucket, edgeInfo models.ChannelEdgeInfo,
	chanID [8]byte) error {

	switch info := edgeInfo.(type) {
	case *models.ChannelEdgeInfo1:
		var b bytes.Buffer
		err := serializeChanEdgeInfo1(&b, info, chanID)
		if err != nil {
			return err
		}

		return edgeIndex.Put(chanID[:], b.Bytes())
	default:
		return fmt.Errorf("unhandled implementation of "+
			"ChannelEdgeInfo: %T", edgeInfo)
	}
}

func serializeChanEdgeInfo1(w io.Writer, edgeInfo *models.ChannelEdgeInfo1,
	chanID [8]byte) error {

	if _, err := w.Write(edgeInfo.NodeKey1Bytes[:]); err != nil {
		return err
	}
	if _, err := w.Write(edgeInfo.NodeKey2Bytes[:]); err != nil {
		return err
	}
	if _, err := w.Write(edgeInfo.BitcoinKey1Bytes[:]); err != nil {
		return err
	}
	if _, err := w.Write(edgeInfo.BitcoinKey2Bytes[:]); err != nil {
		return err
	}

	if err := wire.WriteVarBytes(w, 0, edgeInfo.Features); err != nil {
		return err
	}

	authProof := edgeInfo.AuthProof
	var nodeSig1, nodeSig2, bitcoinSig1, bitcoinSig2 []byte
	if authProof != nil {
		nodeSig1 = authProof.NodeSig1Bytes
		nodeSig2 = authProof.NodeSig2Bytes
		bitcoinSig1 = authProof.BitcoinSig1Bytes
		bitcoinSig2 = authProof.BitcoinSig2Bytes
	}

	if err := wire.WriteVarBytes(w, 0, nodeSig1); err != nil {
		return err
	}
	if err := wire.WriteVarBytes(w, 0, nodeSig2); err != nil {
		return err
	}
	if err := wire.WriteVarBytes(w, 0, bitcoinSig1); err != nil {
		return err
	}
	if err := wire.WriteVarBytes(w, 0, bitcoinSig2); err != nil {
		return err
	}

	if err := writeOutpoint(w, &edgeInfo.ChannelPoint); err != nil {
		return err
	}
	err := binary.Write(w, byteOrder, uint64(edgeInfo.Capacity))
	if err != nil {
		return err
	}
	if _, err := w.Write(chanID[:]); err != nil {
		return err
	}
	if _, err := w.Write(edgeInfo.ChainHash[:]); err != nil {
		return err
	}

	if len(edgeInfo.ExtraOpaqueData) > MaxAllowedExtraOpaqueBytes {
		return ErrTooManyExtraOpaqueBytes(len(edgeInfo.ExtraOpaqueData))
	}

	return wire.WriteVarBytes(w, 0, edgeInfo.ExtraOpaqueData)
}

func fetchChanEdgeInfo(edgeIndex kvdb.RBucket,
	chanID []byte) (models.ChannelEdgeInfo, error) {

	edgeInfoBytes := edgeIndex.Get(chanID)
	if edgeInfoBytes == nil {
		return nil, ErrEdgeNotFound
	}

	edgeInfoReader := bytes.NewReader(edgeInfoBytes)

	return deserializeChanEdgeInfo(edgeInfoReader)
}

func deserializeChanEdgeInfo(r io.Reader) (*models.ChannelEdgeInfo1, error) {
	var (
		err      error
		edgeInfo models.ChannelEdgeInfo1
	)

	if _, err := io.ReadFull(r, edgeInfo.NodeKey1Bytes[:]); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, edgeInfo.NodeKey2Bytes[:]); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, edgeInfo.BitcoinKey1Bytes[:]); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(r, edgeInfo.BitcoinKey2Bytes[:]); err != nil {
		return nil, err
	}

	edgeInfo.Features, err = wire.ReadVarBytes(r, 0, 900, "features")
	if err != nil {
		return nil, err
	}

	var proof models.ChannelAuthProof1

	proof.NodeSig1Bytes, err = wire.ReadVarBytes(r, 0, 80, "sigs")
	if err != nil {
		return nil, err
	}
	proof.NodeSig2Bytes, err = wire.ReadVarBytes(r, 0, 80, "sigs")
	if err != nil {
		return nil, err
	}
	proof.BitcoinSig1Bytes, err = wire.ReadVarBytes(r, 0, 80, "sigs")
	if err != nil {
		return nil, err
	}
	proof.BitcoinSig2Bytes, err = wire.ReadVarBytes(r, 0, 80, "sigs")
	if err != nil {
		return nil, err
	}

	if !proof.IsEmpty() {
		edgeInfo.AuthProof = &proof
	}

	edgeInfo.ChannelPoint = wire.OutPoint{}
	if err := readOutpoint(r, &edgeInfo.ChannelPoint); err != nil {
		return nil, err
	}
	if err := binary.Read(r, byteOrder, &edgeInfo.Capacity); err != nil {
		return nil, err
	}
	if err := binary.Read(r, byteOrder, &edgeInfo.ChannelID); err != nil {
		return nil, err
	}

	if _, err := io.ReadFull(r, edgeInfo.ChainHash[:]); err != nil {
		return nil, err
	}

	// We'll try and see if there are any opaque bytes left, if not, then
	// we'll ignore the EOF error and return the edge as is.
	edgeInfo.ExtraOpaqueData, err = wire.ReadVarBytes(
		r, 0, MaxAllowedExtraOpaqueBytes, "blob",
	)
	switch {
	case err == io.ErrUnexpectedEOF:
	case err == io.EOF:
	case err != nil:
		return nil, err
	}

	return &edgeInfo, nil
}
