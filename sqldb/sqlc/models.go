// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0

package sqlc

import (
	"database/sql"
	"time"
)

type AmpSubInvoice struct {
	SetID       []byte
	State       int16
	CreatedAt   time.Time
	SettledAt   sql.NullTime
	SettleIndex sql.NullInt64
	InvoiceID   int64
}

type AmpSubInvoiceHtlc struct {
	InvoiceID  int64
	SetID      []byte
	HtlcID     int64
	RootShare  []byte
	ChildIndex int64
	Hash       []byte
	Preimage   []byte
}

type Channel struct {
	ID       int64
	Version  int16
	Scid     []byte
	NodeID1  int64
	NodeID2  int64
	Outpoint string
	Capacity int64
}

type ChannelExtraType struct {
	ChannelID int64
	Type      int64
	Value     []byte
}

type ChannelFeature struct {
	ChannelID int64
	FeatureID int64
}

type ChannelPolicy struct {
	ID          int64
	ChannelID   int64
	NodeID      int64
	Timelock    int32
	FeePpm      int64
	BaseFeeMsat int64
	MinHtlcMsat int64
	Signature   []byte
}

type ChannelPolicyExtraType struct {
	ChannelPolicyID int64
	Type            int64
	Value           []byte
}

type ChannelPolicyV1Datum struct {
	ChannelPolicyID int64
	LastUpdate      int64
	Disabled        bool
	MaxHtlcMsat     sql.NullInt64
}

type ChannelsV1Datum struct {
	ChannelID   int64
	BitcoinKey1 []byte
	BitcoinKey2 []byte
}

type Feature struct {
	ID  int64
	Bit int32
}

type Invoice struct {
	ID                 int64
	Hash               []byte
	Preimage           []byte
	SettleIndex        sql.NullInt64
	SettledAt          sql.NullTime
	Memo               sql.NullString
	AmountMsat         int64
	CltvDelta          sql.NullInt32
	Expiry             int32
	PaymentAddr        []byte
	PaymentRequest     sql.NullString
	PaymentRequestHash []byte
	State              int16
	AmountPaidMsat     int64
	IsAmp              bool
	IsHodl             bool
	IsKeysend          bool
	CreatedAt          time.Time
}

type InvoiceEvent struct {
	ID        int64
	AddedAt   time.Time
	EventType int32
	InvoiceID int64
	SetID     []byte
}

type InvoiceEventType struct {
	ID          int64
	Description string
}

type InvoiceFeature struct {
	Feature   int32
	InvoiceID int64
}

type InvoiceHtlc struct {
	ID           int64
	ChanID       string
	HtlcID       int64
	AmountMsat   int64
	TotalMppMsat sql.NullInt64
	AcceptHeight int32
	AcceptTime   time.Time
	ExpiryHeight int32
	State        int16
	ResolveTime  sql.NullTime
	InvoiceID    int64
}

type InvoiceHtlcCustomRecord struct {
	Key    int64
	Value  []byte
	HtlcID int64
}

type InvoicePaymentHash struct {
	ID       int64
	AddIndex int64
	Hash     []byte
}

type InvoiceSequence struct {
	Name         string
	CurrentValue int64
}

type MigrationTracker struct {
	Version       int32
	MigrationTime time.Time
}

type Node struct {
	ID        int64
	Version   int16
	PubKey    []byte
	Alias     sql.NullString
	Signature []byte
}

type NodeAddress struct {
	NodeID   int64
	Type     int16
	Position int32
	Address  string
}

type NodeExtraType struct {
	NodeID int64
	Type   int64
	Value  []byte
}

type NodeFeature struct {
	NodeID    int64
	FeatureID int64
}

type NodesV1Datum struct {
	NodeID     int64
	LastUpdate int64
	Color      string
}

type SourceNode struct {
	NodeID int64
}

type V1ChannelProof struct {
	ChannelID         int64
	Node1Signature    []byte
	Node2Signature    []byte
	Bitcoin1Signature []byte
	Bitcoin2Signature []byte
}

type ZombieChannel struct {
	Scid     int64
	Version  int16
	NodeKey1 []byte
	NodeKey2 []byte
}
