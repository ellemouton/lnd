package wtdb

import (
	"io"

	"github.com/lightningnetwork/lnd/fn"
	"github.com/lightningnetwork/lnd/lnwire"
)

// ChannelInfos is a map for a given channel id to it's ChannelInfo.
type ChannelInfos map[lnwire.ChannelID]*ChannelInfo

// ChannelInfo contains various useful things about a registered channel.
type ChannelInfo struct {
	ClientChanSummary

	// MaxHeight is the highest commitment height that the tower has been
	// handed for this channel. An Option type is used to store this since
	// a commitment height of zero is valid, and we need a way of knowing if
	// we have seen a new height yet or not.
	MaxHeight fn.Option[uint64]
}

// ChannelSummaries is a map for a given channel id to it's ClientChanSummary.
type ChannelSummaries map[lnwire.ChannelID]ClientChanSummary

// ClientChanSummary tracks channel-specific information. A new
// ClientChanSummary is inserted in the database the first time the client
// encounters a particular channel.
type ClientChanSummary struct {
	// SweepPkScript is the pkscript to which all justice transactions will
	// deposit recovered funds for this particular channel.
	SweepPkScript []byte

	// TODO(conner): later extend with info about initial commit height,
	// ineligible states, etc.
}

// Encode writes the ClientChanSummary to the passed io.Writer.
func (s *ClientChanSummary) Encode(w io.Writer) error {
	return WriteElement(w, s.SweepPkScript)
}

// Decode reads a ClientChanSummary form the passed io.Reader.
func (s *ClientChanSummary) Decode(r io.Reader) error {
	return ReadElement(r, &s.SweepPkScript)
}
