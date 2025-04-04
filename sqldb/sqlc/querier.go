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
	AddChannelPolicyExtraType(ctx context.Context, arg AddChannelPolicyExtraTypeParams) error
	AddSourceNode(ctx context.Context, nodeID int64) error
	ClearKVInvoiceHashIndex(ctx context.Context) error
	CountZombieChannels(ctx context.Context, version int16) (int64, error)
	CreateChannel(ctx context.Context, arg CreateChannelParams) (int64, error)
	CreateChannelPolicy(ctx context.Context, arg CreateChannelPolicyParams) (int64, error)
	CreateChannelPolicyV1Data(ctx context.Context, arg CreateChannelPolicyV1DataParams) error
	CreateChannelsV1Data(ctx context.Context, arg CreateChannelsV1DataParams) error
	CreateFeature(ctx context.Context, bit int32) (int64, error)
	CreateNode(ctx context.Context, arg CreateNodeParams) (int64, error)
	CreateV1ChannelProof(ctx context.Context, arg CreateV1ChannelProofParams) error
	DeleteAllChannelPolicyExtraTypes(ctx context.Context, channelPolicyID int64) error
	DeleteCanceledInvoices(ctx context.Context) (sql.Result, error)
	DeleteChannel(ctx context.Context, id int64) error
	DeleteChannelPolicyExtraType(ctx context.Context, arg DeleteChannelPolicyExtraTypeParams) error
	DeleteExtraChannelType(ctx context.Context, arg DeleteExtraChannelTypeParams) error
	DeleteExtraNodeType(ctx context.Context, arg DeleteExtraNodeTypeParams) error
	DeleteInvoice(ctx context.Context, arg DeleteInvoiceParams) (sql.Result, error)
	DeleteNode(ctx context.Context, id int64) error
	DeleteNodeAddresses(ctx context.Context, nodeID int64) error
	DeleteNodeFeature(ctx context.Context, arg DeleteNodeFeatureParams) error
	DeletePruneLogEntriesInRange(ctx context.Context, arg DeletePruneLogEntriesInRangeParams) error
	DeletePruneLogEntry(ctx context.Context, blockHeight int64) error
	DeleteZombieChannel(ctx context.Context, arg DeleteZombieChannelParams) error
	FetchAMPSubInvoiceHTLCs(ctx context.Context, arg FetchAMPSubInvoiceHTLCsParams) ([]FetchAMPSubInvoiceHTLCsRow, error)
	FetchAMPSubInvoices(ctx context.Context, arg FetchAMPSubInvoicesParams) ([]AmpSubInvoice, error)
	FetchSettledAMPSubInvoices(ctx context.Context, arg FetchSettledAMPSubInvoicesParams) ([]FetchSettledAMPSubInvoicesRow, error)
	FilterInvoices(ctx context.Context, arg FilterInvoicesParams) ([]Invoice, error)
	GetAMPInvoiceID(ctx context.Context, setID []byte) (int64, error)
	GetChannelByID(ctx context.Context, id int64) (Channel, error)
	GetChannelByOutpoint(ctx context.Context, outpoint string) (Channel, error)
	GetChannelByOutpointAndVersion(ctx context.Context, arg GetChannelByOutpointAndVersionParams) (Channel, error)
	GetChannelBySCIDAndVersion(ctx context.Context, arg GetChannelBySCIDAndVersionParams) (Channel, error)
	GetChannelFeatures(ctx context.Context, channelID int64) ([]GetChannelFeaturesRow, error)
	GetChannelPolicyByChannelAndNode(ctx context.Context, arg GetChannelPolicyByChannelAndNodeParams) (ChannelPolicy, error)
	GetChannelPolicyExtraTypes(ctx context.Context, channelPolicyID int64) ([]ChannelPolicyExtraType, error)
	GetChannelPolicyV1Data(ctx context.Context, channelPolicyID int64) (ChannelPolicyV1Datum, error)
	GetChannelsBySCIDRange(ctx context.Context, arg GetChannelsBySCIDRangeParams) ([]Channel, error)
	GetChannelsV1Data(ctx context.Context, channelID int64) (ChannelsV1Datum, error)
	GetDatabaseVersion(ctx context.Context) (int32, error)
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
	GetNodeAddresses(ctx context.Context, nodeID int64) ([]GetNodeAddressesRow, error)
	GetNodeAliasByPubKeyAndVersion(ctx context.Context, arg GetNodeAliasByPubKeyAndVersionParams) (sql.NullString, error)
	GetNodeByID(ctx context.Context, id int64) (Node, error)
	GetNodeByPubKeyAndVersion(ctx context.Context, arg GetNodeByPubKeyAndVersionParams) (Node, error)
	GetNodeFeatures(ctx context.Context, nodeID int64) ([]GetNodeFeaturesRow, error)
	GetPruneTip(ctx context.Context) (PruneLog, error)
	GetPublicV1ChannelsBySCID(ctx context.Context, arg GetPublicV1ChannelsBySCIDParams) ([]Channel, error)
	GetSCIDByOutpointAndVersion(ctx context.Context, arg GetSCIDByOutpointAndVersionParams) ([]byte, error)
	GetSourceNodesByVersion(ctx context.Context, version int16) ([]int64, error)
	GetUnconnectedNodes(ctx context.Context, version int16) ([]GetUnconnectedNodesRow, error)
	GetV1ChannelPolicyByChannelAndNode(ctx context.Context, arg GetV1ChannelPolicyByChannelAndNodeParams) (GetV1ChannelPolicyByChannelAndNodeRow, error)
	GetV1ChannelProof(ctx context.Context, channelID int64) (V1ChannelProof, error)
	GetV1ChannelsByPolicyLastUpdateRange(ctx context.Context, arg GetV1ChannelsByPolicyLastUpdateRangeParams) ([]Channel, error)
	GetV1DisabledSCIDs(ctx context.Context) ([][]byte, error)
	GetV1NodeData(ctx context.Context, nodeID int64) (NodesV1Datum, error)
	GetV1NodesByLastUpdateRange(ctx context.Context, arg GetV1NodesByLastUpdateRangeParams) ([]Node, error)
	GetZombieChannel(ctx context.Context, arg GetZombieChannelParams) (ZombieChannel, error)
	HighestSCID(ctx context.Context, version int16) ([]byte, error)
	InsertAMPSubInvoice(ctx context.Context, arg InsertAMPSubInvoiceParams) error
	InsertAMPSubInvoiceHTLC(ctx context.Context, arg InsertAMPSubInvoiceHTLCParams) error
	InsertChannelFeature(ctx context.Context, arg InsertChannelFeatureParams) error
	InsertClosedChannel(ctx context.Context, scid []byte) error
	InsertInvoice(ctx context.Context, arg InsertInvoiceParams) (int64, error)
	InsertInvoiceFeature(ctx context.Context, arg InsertInvoiceFeatureParams) error
	InsertInvoiceHTLC(ctx context.Context, arg InsertInvoiceHTLCParams) (int64, error)
	InsertInvoiceHTLCCustomRecord(ctx context.Context, arg InsertInvoiceHTLCCustomRecordParams) error
	InsertKVInvoiceKeyAndAddIndex(ctx context.Context, arg InsertKVInvoiceKeyAndAddIndexParams) error
	InsertMigratedInvoice(ctx context.Context, arg InsertMigratedInvoiceParams) (int64, error)
	InsertNodeAddress(ctx context.Context, arg InsertNodeAddressParams) error
	InsertNodeFeature(ctx context.Context, arg InsertNodeFeatureParams) error
	IsClosedChannel(ctx context.Context, scid []byte) (bool, error)
	IsV1ChannelPublic(ctx context.Context, channelID int64) (bool, error)
	IsZombieChannel(ctx context.Context, arg IsZombieChannelParams) (bool, error)
	ListAllChannelsByVersion(ctx context.Context, version int16) ([]Channel, error)
	ListChannelsByNodeIDAndVersion(ctx context.Context, arg ListChannelsByNodeIDAndVersionParams) ([]Channel, error)
	ListNodeIDsAndPubKeysV1(ctx context.Context) ([]ListNodeIDsAndPubKeysV1Row, error)
	ListNodesByVersion(ctx context.Context, version int16) ([]ListNodesByVersionRow, error)
	NextInvoiceSettleIndex(ctx context.Context) (int64, error)
	NodeHasV1ProofChannel(ctx context.Context, nodeID1 int64) (bool, error)
	OnAMPSubInvoiceCanceled(ctx context.Context, arg OnAMPSubInvoiceCanceledParams) error
	OnAMPSubInvoiceCreated(ctx context.Context, arg OnAMPSubInvoiceCreatedParams) error
	OnAMPSubInvoiceSettled(ctx context.Context, arg OnAMPSubInvoiceSettledParams) error
	OnInvoiceCanceled(ctx context.Context, arg OnInvoiceCanceledParams) error
	OnInvoiceCreated(ctx context.Context, arg OnInvoiceCreatedParams) error
	OnInvoiceSettled(ctx context.Context, arg OnInvoiceSettledParams) error
	SetKVInvoicePaymentHash(ctx context.Context, arg SetKVInvoicePaymentHashParams) error
	SetMigration(ctx context.Context, arg SetMigrationParams) error
	UpdateAMPSubInvoiceHTLCPreimage(ctx context.Context, arg UpdateAMPSubInvoiceHTLCPreimageParams) (sql.Result, error)
	UpdateAMPSubInvoiceState(ctx context.Context, arg UpdateAMPSubInvoiceStateParams) error
	UpdateChannelPolicy(ctx context.Context, arg UpdateChannelPolicyParams) error
	UpdateChannelPolicyV1Data(ctx context.Context, arg UpdateChannelPolicyV1DataParams) error
	UpdateChannelsV1Data(ctx context.Context, arg UpdateChannelsV1DataParams) error
	UpdateInvoiceAmountPaid(ctx context.Context, arg UpdateInvoiceAmountPaidParams) (sql.Result, error)
	UpdateInvoiceHTLC(ctx context.Context, arg UpdateInvoiceHTLCParams) error
	UpdateInvoiceHTLCs(ctx context.Context, arg UpdateInvoiceHTLCsParams) error
	UpdateInvoiceState(ctx context.Context, arg UpdateInvoiceStateParams) (sql.Result, error)
	UpdateNode(ctx context.Context, arg UpdateNodeParams) error
	UpsertAMPSubInvoice(ctx context.Context, arg UpsertAMPSubInvoiceParams) (sql.Result, error)
	UpsertChannelExtraType(ctx context.Context, arg UpsertChannelExtraTypeParams) error
	UpsertNodeExtraType(ctx context.Context, arg UpsertNodeExtraTypeParams) error
	UpsertPruneLogEntry(ctx context.Context, arg UpsertPruneLogEntryParams) error
	UpsertV1NodeData(ctx context.Context, arg UpsertV1NodeDataParams) error
	UpsertZombieChannel(ctx context.Context, arg UpsertZombieChannelParams) error
}

var _ Querier = (*Queries)(nil)
