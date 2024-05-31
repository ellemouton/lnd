package models

import (
	"encoding/binary"
	"io"

	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/lightningnetwork/lnd/routing/route"
)

var byteOrder = binary.BigEndian

type MCRoute struct {
	SourcePubKey route.Vertex
	TotalAmount  lnwire.MilliSatoshi
	Hops         []*MCHop
}

type MCHop struct {
	PubKeyBytes      route.Vertex
	AmtToForward     lnwire.MilliSatoshi
	ChannelID        uint64
	HasBlindingPoint bool
}

func (r *MCRoute) Serialize(w io.Writer) error {
	err := binary.Write(w, byteOrder, uint64(r.TotalAmount))
	if err != nil {
		return err
	}

	if err := wire.WriteVarBytes(w, 0, r.SourcePubKey[:]); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, uint32(len(r.Hops))); err != nil {
		return err
	}

	for _, h := range r.Hops {
		if err := h.serialize(w); err != nil {
			return err
		}
	}

	return nil
}

func DeserializeRoute(r io.Reader) (*MCRoute, error) {
	var (
		rt      MCRoute
		numHops uint32
	)

	if err := binary.Read(r, byteOrder, &rt.TotalAmount); err != nil {
		return nil, err
	}

	pub, err := wire.ReadVarBytes(r, 0, 66000, "[]byte")
	if err != nil {
		return nil, err
	}
	copy(rt.SourcePubKey[:], pub)

	if err := binary.Read(r, byteOrder, &numHops); err != nil {
		return nil, err
	}

	hops := make([]*MCHop, numHops)
	for i := uint32(0); i < numHops; i++ {
		hop, err := deserializeHop(r)
		if err != nil {
			return nil, err
		}

		hops[i] = hop
	}
	rt.Hops = hops

	return &rt, nil
}

func (h *MCHop) serialize(w io.Writer) error {
	if err := wire.WriteVarBytes(w, 0, h.PubKeyBytes[:]); err != nil {
		return err
	}

	err := binary.Write(w, byteOrder, uint64(h.AmtToForward))
	if err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, h.ChannelID); err != nil {
		return err
	}

	if err := binary.Write(w, byteOrder, h.HasBlindingPoint); err != nil {
		return err
	}

	return nil
}

func deserializeHop(r io.Reader) (*MCHop, error) {
	var (
		h MCHop
	)
	pub, err := wire.ReadVarBytes(r, 0, 66000, "[]byte")
	if err != nil {
		return nil, err
	}
	copy(h.PubKeyBytes[:], pub)

	var a uint64
	if err := binary.Read(r, byteOrder, &a); err != nil {
		return nil, err
	}
	h.AmtToForward = lnwire.MilliSatoshi(a)

	if err := binary.Read(r, byteOrder, &h.ChannelID); err != nil {
		return nil, err
	}

	if err := binary.Read(r, byteOrder, &h.HasBlindingPoint); err != nil {
		return nil, err
	}

	return &h, nil
}

func ToMCRoute(route *route.Route) *MCRoute {
	hops := make([]*MCHop, len(route.Hops))

	for i, hop := range route.Hops {
		hops[i] = &MCHop{
			PubKeyBytes:      hop.PubKeyBytes,
			AmtToForward:     hop.AmtToForward,
			ChannelID:        hop.ChannelID,
			HasBlindingPoint: hop.BlindingPoint != nil,
		}
	}

	return &MCRoute{
		SourcePubKey: route.SourcePubKey,
		TotalAmount:  route.TotalAmount,
		Hops:         hops,
	}
}
