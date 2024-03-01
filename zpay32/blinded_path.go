package zpay32

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/lightningnetwork/lnd/lnwire"
)

type BlindedHop struct {
	// NodeID is the public key that a sender should use to derive the
	// shared secret to use for encrypting the onion for this node. It is
	// real pub key of the node for the blinded path introduction node (ie,
	// the first hop), otherwise, it is a blinded public key.
	NodeID *btcec.PublicKey

	EncryptedRecipientData []byte
}

func (h *BlindedHop) Size() uint64 {
	return 33 + 2 + uint64(len(h.EncryptedRecipientData))
}

func DecodeBlindedHop(r io.Reader) (*BlindedHop, error) {
	var nodeIDBytes [33]byte
	_, err := r.Read(nodeIDBytes[:])
	if err != nil {
		return nil, err
	}

	nodeID, err := btcec.ParsePubKey(nodeIDBytes[:])
	if err != nil {
		return nil, err
	}

	var dataLenBytes [2]byte
	_, err = r.Read(dataLenBytes[:])
	if err != nil {
		return nil, err
	}

	dataLen := binary.BigEndian.Uint16(dataLenBytes[:])

	encryptedData := make([]byte, dataLen)
	_, err = r.Read(encryptedData)
	if err != nil {
		return nil, err
	}

	return &BlindedHop{
		NodeID:                 nodeID,
		EncryptedRecipientData: encryptedData,
	}, nil
}

func (h *BlindedHop) Encode(w io.Writer) error {
	_, err := w.Write(h.NodeID.SerializeCompressed())
	if err != nil {
		return err
	}

	if len(h.EncryptedRecipientData) > math.MaxUint16 {
		return fmt.Errorf("encrypted recipient data can not exceed a "+
			"length of %d bytes", math.MaxUint16)
	}

	var dataLen [2]byte
	binary.BigEndian.PutUint16(
		dataLen[:], uint16(len(h.EncryptedRecipientData)),
	)
	_, err = w.Write(dataLen[:])
	if err != nil {
		return err
	}

	_, err = w.Write(h.EncryptedRecipientData)

	return err
}

type BlindedPath struct {
	FeeBaseMsat  uint32
	FeePropMsat  uint32
	CltvExpDelta uint16
	HTLCMinMsat  uint64
	HTLCMaxMsat  uint64
	Features     *lnwire.FeatureVector

	FirstEphemeralBlindingPoint *btcec.PublicKey
	Hops                        []*BlindedHop
}

func DecodeBlindedPath(r io.Reader) (*BlindedPath, error) {
	var relayInfo [26]byte
	n, err := r.Read(relayInfo[:])
	if err != nil {
		return nil, err
	}
	if n != 26 {
		return nil, fmt.Errorf("bad")
	}

	var path BlindedPath

	path.FeeBaseMsat = binary.BigEndian.Uint32(relayInfo[:4])
	path.FeePropMsat = binary.BigEndian.Uint32(relayInfo[4:8])
	path.CltvExpDelta = binary.BigEndian.Uint16(relayInfo[8:10])
	path.HTLCMinMsat = binary.BigEndian.Uint64(relayInfo[10:18])
	path.HTLCMaxMsat = binary.BigEndian.Uint64(relayInfo[18:])

	f := lnwire.EmptyFeatureVector()
	err = f.Decode(r)
	if err != nil {
		return nil, err
	}
	path.Features = f

	var blindingPointBytes [33]byte
	_, err = r.Read(blindingPointBytes[:])
	if err != nil {
		return nil, err
	}

	blinding, err := btcec.ParsePubKey(blindingPointBytes[:])
	if err != nil {
		return nil, err
	}
	path.FirstEphemeralBlindingPoint = blinding

	var numHops [1]byte
	_, err = r.Read(numHops[:])
	if err != nil {
		return nil, err
	}

	for i := 0; i < int(numHops[0]); i++ {
		hop, err := DecodeBlindedHop(r)
		if err != nil {
			return nil, err
		}

		path.Hops = append(path.Hops, hop)
	}

	return &path, nil
}

func (p *BlindedPath) Encode(w io.Writer) error {
	var relayInfo [26]byte
	binary.BigEndian.PutUint32(relayInfo[:4], p.FeeBaseMsat)
	binary.BigEndian.PutUint32(relayInfo[4:8], p.FeePropMsat)
	binary.BigEndian.PutUint16(relayInfo[8:10], p.CltvExpDelta)
	binary.BigEndian.PutUint64(relayInfo[10:18], p.HTLCMinMsat)
	binary.BigEndian.PutUint64(relayInfo[18:], p.HTLCMaxMsat)

	_, err := w.Write(relayInfo[:])
	if err != nil {
		return err
	}

	err = p.Features.Encode(w)
	if err != nil {
		return err
	}

	_, err = w.Write(p.FirstEphemeralBlindingPoint.SerializeCompressed())
	if err != nil {
		return err
	}

	_, err = w.Write([]byte{byte(len(p.Hops))})
	if err != nil {
		return err
	}

	for _, hop := range p.Hops {
		err = hop.Encode(w)
		if err != nil {
			return err
		}
	}

	return nil
}
