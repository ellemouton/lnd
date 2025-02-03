// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0

package sqlc

import (
	"context"
	"database/sql"
	"time"
)

type Querier interface {
	AddChannelSignature(ctx context.Context, arg AddChannelSignatureParams) error
	AddClosedSCID(ctx context.Context, arg AddClosedSCIDParams) error
	ClearKVInvoiceHashIndex(ctx context.Context) error
	CountZombieChannels(ctx context.Context) (int64, error)
	DeleteCanceledInvoices(ctx context.Context) (sql.Result, error)
	DeleteChannel(ctx context.Context, channelID int64) error
	DeleteChannelFeature(ctx context.Context, arg DeleteChannelFeatureParams) error
	DeleteExtraChanPolicyType(ctx context.Context, arg DeleteExtraChanPolicyTypeParams) error
	DeleteExtraChannelType(ctx context.Context, arg DeleteExtraChannelTypeParams) error
	DeleteExtraNodeType(ctx context.Context, arg DeleteExtraNodeTypeParams) error
	DeleteInvoice(ctx context.Context, arg DeleteInvoiceParams) (sql.Result, error)
	DeleteNode(ctx context.Context, id int64) error
	DeleteNodeAddress(ctx context.Context, arg DeleteNodeAddressParams) error
	DeleteNodeFeature(ctx context.Context, arg DeleteNodeFeatureParams) error
	DeleteZombieChannel(ctx context.Context, channelID int64) error
	FetchAMPSubInvoiceHTLCs(ctx context.Context, arg FetchAMPSubInvoiceHTLCsParams) ([]FetchAMPSubInvoiceHTLCsRow, error)
	FetchAMPSubInvoices(ctx context.Context, arg FetchAMPSubInvoicesParams) ([]AmpSubInvoice, error)
	FetchSettledAMPSubInvoices(ctx context.Context, arg FetchSettledAMPSubInvoicesParams) ([]FetchSettledAMPSubInvoicesRow, error)
	FilterInvoices(ctx context.Context, arg FilterInvoicesParams) ([]Invoice, error)
	GetAMPInvoiceID(ctx context.Context, setID []byte) (int64, error)
	GetChannel(ctx context.Context, id int64) (Channel, error)
	GetChannelByChanID(ctx context.Context, channelID int64) (Channel, error)
	GetChannelByOutpoint(ctx context.Context, outpoint string) (Channel, error)
	GetChannelFeatures(ctx context.Context, channelID int64) ([]ChannelFeature, error)
	GetChannelPolicy(ctx context.Context, arg GetChannelPolicyParams) (ChannelPolicy, error)
	GetDatabaseVersion(ctx context.Context) (int32, error)
	GetExtraChanPolicyTypes(ctx context.Context, channelPolicyID int64) ([]ChannelPolicyExtraType, error)
	GetExtraChannelTypes(ctx context.Context, channelID int64) ([]ChannelExtraType, error)
	GetExtraNodeTypes(ctx context.Context, nodeID int64) ([]NodeExtraType, error)
	// This method may return more than one invoice if filter using multiple fields
	// from different invoices. It is the caller's responsibility to ensure that
	// we bubble up an error in those cases.
	GetInvoice(ctx context.Context, arg GetInvoiceParams) ([]Invoice, error)
	GetInvoiceByHash(ctx context.Context, hash []byte) (Invoice, error)
	GetInvoiceBySetID(ctx context.Context, setID []byte) ([]Invoice, error)
	GetInvoiceFeatures(ctx context.Context, invoiceID int64) ([]InvoiceFeature, error)
	GetInvoiceHTLCCustomRecords(ctx context.Context, invoiceID int64) ([]GetInvoiceHTLCCustomRecordsRow, error)
	GetInvoiceHTLCs(ctx context.Context, invoiceID int64) ([]InvoiceHtlc, error)
	GetKVInvoicePaymentHashByAddIndex(ctx context.Context, addIndex int64) ([]byte, error)
	GetMigration(ctx context.Context, version int32) (time.Time, error)
	GetNode(ctx context.Context, id int64) (Node, error)
	GetNodeAddresses(ctx context.Context, nodeID int64) ([]NodeAddress, error)
	GetNodeAliasByPubKey(ctx context.Context, pubKey []byte) (sql.NullString, error)
	GetNodeByPubKey(ctx context.Context, pubKey []byte) (Node, error)
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]NodeFeature, error)
	GetNodeIDByPubKey(ctx context.Context, pubKey []byte) (int64, error)
	GetSourceNode(ctx context.Context) (int64, error)
	InsertAMPSubInvoice(ctx context.Context, arg InsertAMPSubInvoiceParams) error
	InsertAMPSubInvoiceHTLC(ctx context.Context, arg InsertAMPSubInvoiceHTLCParams) error
	InsertChannel(ctx context.Context, arg InsertChannelParams) (int64, error)
	InsertChannelFeature(ctx context.Context, arg InsertChannelFeatureParams) error
	InsertChannelPolicy(ctx context.Context, arg InsertChannelPolicyParams) (int64, error)
	InsertInvoice(ctx context.Context, arg InsertInvoiceParams) (int64, error)
	InsertInvoiceFeature(ctx context.Context, arg InsertInvoiceFeatureParams) error
	InsertInvoiceHTLC(ctx context.Context, arg InsertInvoiceHTLCParams) (int64, error)
	InsertInvoiceHTLCCustomRecord(ctx context.Context, arg InsertInvoiceHTLCCustomRecordParams) error
	InsertKVInvoiceKeyAndAddIndex(ctx context.Context, arg InsertKVInvoiceKeyAndAddIndexParams) error
	InsertMigratedInvoice(ctx context.Context, arg InsertMigratedInvoiceParams) (int64, error)
	InsertNode(ctx context.Context, arg InsertNodeParams) (int64, error)
	InsertNodeAddress(ctx context.Context, arg InsertNodeAddressParams) error
	InsertNodeFeature(ctx context.Context, arg InsertNodeFeatureParams) error
	IsClosedSCID(ctx context.Context, channelID int64) (bool, error)
	IsPublicNode(ctx context.Context, nodeID1 int64) (bool, error)
	IsZombieChannel(ctx context.Context, channelID int64) (bool, error)
	ListAllChannels(ctx context.Context) ([]Channel, error)
	ListNodeChannels(ctx context.Context, nodeID1 int64) ([]Channel, error)
	NextInvoiceSettleIndex(ctx context.Context) (int64, error)
	OnAMPSubInvoiceCanceled(ctx context.Context, arg OnAMPSubInvoiceCanceledParams) error
	OnAMPSubInvoiceCreated(ctx context.Context, arg OnAMPSubInvoiceCreatedParams) error
	OnAMPSubInvoiceSettled(ctx context.Context, arg OnAMPSubInvoiceSettledParams) error
	OnInvoiceCanceled(ctx context.Context, arg OnInvoiceCanceledParams) error
	OnInvoiceCreated(ctx context.Context, arg OnInvoiceCreatedParams) error
	OnInvoiceSettled(ctx context.Context, arg OnInvoiceSettledParams) error
	SetKVInvoicePaymentHash(ctx context.Context, arg SetKVInvoicePaymentHashParams) error
	SetMigration(ctx context.Context, arg SetMigrationParams) error
	SetSourceNode(ctx context.Context, nodeID int64) error
	UpdateAMPSubInvoiceHTLCPreimage(ctx context.Context, arg UpdateAMPSubInvoiceHTLCPreimageParams) (sql.Result, error)
	UpdateAMPSubInvoiceState(ctx context.Context, arg UpdateAMPSubInvoiceStateParams) error
	UpdateChannelPolicy(ctx context.Context, arg UpdateChannelPolicyParams) error
	UpdateInvoiceAmountPaid(ctx context.Context, arg UpdateInvoiceAmountPaidParams) (sql.Result, error)
	UpdateInvoiceHTLC(ctx context.Context, arg UpdateInvoiceHTLCParams) error
	UpdateInvoiceHTLCs(ctx context.Context, arg UpdateInvoiceHTLCsParams) error
	UpdateInvoiceState(ctx context.Context, arg UpdateInvoiceStateParams) (sql.Result, error)
	UpdateNode(ctx context.Context, arg UpdateNodeParams) error
	UpsertAMPSubInvoice(ctx context.Context, arg UpsertAMPSubInvoiceParams) (sql.Result, error)
	UpsertChanPolicyExtraType(ctx context.Context, arg UpsertChanPolicyExtraTypeParams) error
	UpsertChannelExtraType(ctx context.Context, arg UpsertChannelExtraTypeParams) error
	UpsertNodeExtraType(ctx context.Context, arg UpsertNodeExtraTypeParams) error
	UpsertZombieChannel(ctx context.Context, arg UpsertZombieChannelParams) error
}

var _ Querier = (*Queries)(nil)
