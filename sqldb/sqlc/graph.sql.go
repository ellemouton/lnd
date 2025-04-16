// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0
// source: graph.sql

package sqlc

import (
	"context"
	"database/sql"
)

const addChannelPolicyExtraType = `-- name: AddChannelPolicyExtraType :exec
INSERT INTO channel_policy_extra_types (
    channel_policy_id, type, value
) VALUES (
   $1, $2, $3
)
ON CONFLICT (type, channel_policy_id)
    DO UPDATE SET value = EXCLUDED.value
`

type AddChannelPolicyExtraTypeParams struct {
	ChannelPolicyID int64
	Type            int64
	Value           []byte
}

func (q *Queries) AddChannelPolicyExtraType(ctx context.Context, arg AddChannelPolicyExtraTypeParams) error {
	_, err := q.db.ExecContext(ctx, addChannelPolicyExtraType, arg.ChannelPolicyID, arg.Type, arg.Value)
	return err
}

const addSourceNode = `-- name: AddSourceNode :exec
/* ─────────────────────────────────────────────
   source_nodes table queries
   ─────────────────────────────────────────────
*/

INSERT INTO source_nodes (node_id)
VALUES ($1)
ON CONFLICT (node_id) DO NOTHING
`

func (q *Queries) AddSourceNode(ctx context.Context, nodeID int64) error {
	_, err := q.db.ExecContext(ctx, addSourceNode, nodeID)
	return err
}

const countZombieChannels = `-- name: CountZombieChannels :one
SELECT COUNT(*)
FROM zombie_channels
WHERE version = $1
`

func (q *Queries) CountZombieChannels(ctx context.Context, version int16) (int64, error) {
	row := q.db.QueryRowContext(ctx, countZombieChannels, version)
	var count int64
	err := row.Scan(&count)
	return count, err
}

const createChannel = `-- name: CreateChannel :one
/* ─────────────────────────────────────────────
   channels table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channels (
    version, scid, node_id_1, node_id_2,
    outpoint, capacity
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id
`

type CreateChannelParams struct {
	Version  int16
	Scid     []byte
	NodeID1  int64
	NodeID2  int64
	Outpoint string
	Capacity int64
}

func (q *Queries) CreateChannel(ctx context.Context, arg CreateChannelParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, createChannel,
		arg.Version,
		arg.Scid,
		arg.NodeID1,
		arg.NodeID2,
		arg.Outpoint,
		arg.Capacity,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const createChannelPolicy = `-- name: CreateChannelPolicy :one
/* ─────────────────────────────────────────────
   channel_policies table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channel_policies (
    channel_id, node_id, timelock, fee_ppm, base_fee_msat, min_htlc_msat, signature
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING id
`

type CreateChannelPolicyParams struct {
	ChannelID   int64
	NodeID      int64
	Timelock    int32
	FeePpm      int64
	BaseFeeMsat int64
	MinHtlcMsat int64
	Signature   []byte
}

func (q *Queries) CreateChannelPolicy(ctx context.Context, arg CreateChannelPolicyParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, createChannelPolicy,
		arg.ChannelID,
		arg.NodeID,
		arg.Timelock,
		arg.FeePpm,
		arg.BaseFeeMsat,
		arg.MinHtlcMsat,
		arg.Signature,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const createChannelPolicyV1Data = `-- name: CreateChannelPolicyV1Data :exec
/* ─────────────────────────────────────────────
   channel_policy_v1_data table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channel_policy_v1_data (
    channel_policy_id, last_update, disabled, max_htlc_msat
) VALUES (
    $1, $2, $3, $4
)
`

type CreateChannelPolicyV1DataParams struct {
	ChannelPolicyID int64
	LastUpdate      int64
	Disabled        bool
	MaxHtlcMsat     sql.NullInt64
}

func (q *Queries) CreateChannelPolicyV1Data(ctx context.Context, arg CreateChannelPolicyV1DataParams) error {
	_, err := q.db.ExecContext(ctx, createChannelPolicyV1Data,
		arg.ChannelPolicyID,
		arg.LastUpdate,
		arg.Disabled,
		arg.MaxHtlcMsat,
	)
	return err
}

const createChannelsV1Data = `-- name: CreateChannelsV1Data :exec
/* ─────────────────────────────────────────────
   channels_v1_data table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channels_v1_data (
    channel_id, bitcoin_key_1, bitcoin_key_2
) VALUES (
    $1, $2, $3
)
`

type CreateChannelsV1DataParams struct {
	ChannelID   int64
	BitcoinKey1 []byte
	BitcoinKey2 []byte
}

func (q *Queries) CreateChannelsV1Data(ctx context.Context, arg CreateChannelsV1DataParams) error {
	_, err := q.db.ExecContext(ctx, createChannelsV1Data, arg.ChannelID, arg.BitcoinKey1, arg.BitcoinKey2)
	return err
}

const createFeature = `-- name: CreateFeature :one
/* ─────────────────────────────────────────────
   features table queries
   ─────────────────────────────────────────────
 */

INSERT INTO features (bit)
VALUES ($1)
ON CONFLICT (bit) DO UPDATE SET bit = EXCLUDED.bit
RETURNING id
`

func (q *Queries) CreateFeature(ctx context.Context, bit int32) (int64, error) {
	row := q.db.QueryRowContext(ctx, createFeature, bit)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const createNode = `-- name: CreateNode :one
/* ─────────────────────────────────────────────
   nodes table queries
   ─────────────────────────────────────────────
*/

INSERT INTO nodes (
    version, pub_key, alias, signature
)
VALUES ($1, $2, $3, $4)
RETURNING id
`

type CreateNodeParams struct {
	Version   int16
	PubKey    []byte
	Alias     sql.NullString
	Signature []byte
}

func (q *Queries) CreateNode(ctx context.Context, arg CreateNodeParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, createNode,
		arg.Version,
		arg.PubKey,
		arg.Alias,
		arg.Signature,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const createV1ChannelProof = `-- name: CreateV1ChannelProof :exec
/* ─────────────────────────────────────────────
   channels_v1_channel_proofs table queries
   ─────────────────────────────────────────────
*/

INSERT INTO v1_channel_proofs (
    channel_id, node_1_signature, node_2_signature,
    bitcoin_1_signature, bitcoin_2_signature
) VALUES (
     $1, $2, $3, $4, $5
)
`

type CreateV1ChannelProofParams struct {
	ChannelID         int64
	Node1Signature    []byte
	Node2Signature    []byte
	Bitcoin1Signature []byte
	Bitcoin2Signature []byte
}

func (q *Queries) CreateV1ChannelProof(ctx context.Context, arg CreateV1ChannelProofParams) error {
	_, err := q.db.ExecContext(ctx, createV1ChannelProof,
		arg.ChannelID,
		arg.Node1Signature,
		arg.Node2Signature,
		arg.Bitcoin1Signature,
		arg.Bitcoin2Signature,
	)
	return err
}

const deleteChannel = `-- name: DeleteChannel :exec
DELETE FROM channels WHERE id = $1
`

func (q *Queries) DeleteChannel(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, deleteChannel, id)
	return err
}

const deleteChannelPolicyExtraType = `-- name: DeleteChannelPolicyExtraType :exec
DELETE FROM channel_policy_extra_types
WHERE channel_policy_id = $1 AND type = $2
`

type DeleteChannelPolicyExtraTypeParams struct {
	ChannelPolicyID int64
	Type            int64
}

func (q *Queries) DeleteChannelPolicyExtraType(ctx context.Context, arg DeleteChannelPolicyExtraTypeParams) error {
	_, err := q.db.ExecContext(ctx, deleteChannelPolicyExtraType, arg.ChannelPolicyID, arg.Type)
	return err
}

const deleteExtraChannelType = `-- name: DeleteExtraChannelType :exec
DELETE FROM channel_extra_types
WHERE channel_id = $1
  AND type = $2
`

type DeleteExtraChannelTypeParams struct {
	ChannelID int64
	Type      int64
}

func (q *Queries) DeleteExtraChannelType(ctx context.Context, arg DeleteExtraChannelTypeParams) error {
	_, err := q.db.ExecContext(ctx, deleteExtraChannelType, arg.ChannelID, arg.Type)
	return err
}

const deleteExtraNodeType = `-- name: DeleteExtraNodeType :exec
DELETE FROM node_extra_types
WHERE node_id = $1
  AND type = $2
`

type DeleteExtraNodeTypeParams struct {
	NodeID int64
	Type   int64
}

func (q *Queries) DeleteExtraNodeType(ctx context.Context, arg DeleteExtraNodeTypeParams) error {
	_, err := q.db.ExecContext(ctx, deleteExtraNodeType, arg.NodeID, arg.Type)
	return err
}

const deleteNode = `-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = $1
`

func (q *Queries) DeleteNode(ctx context.Context, id int64) error {
	_, err := q.db.ExecContext(ctx, deleteNode, id)
	return err
}

const deleteNodeAddresses = `-- name: DeleteNodeAddresses :exec
DELETE FROM node_addresses
WHERE node_id = $1
`

func (q *Queries) DeleteNodeAddresses(ctx context.Context, nodeID int64) error {
	_, err := q.db.ExecContext(ctx, deleteNodeAddresses, nodeID)
	return err
}

const deleteNodeFeature = `-- name: DeleteNodeFeature :exec
DELETE FROM node_features
WHERE node_id = $1
  AND feature_id = $2
`

type DeleteNodeFeatureParams struct {
	NodeID    int64
	FeatureID int64
}

func (q *Queries) DeleteNodeFeature(ctx context.Context, arg DeleteNodeFeatureParams) error {
	_, err := q.db.ExecContext(ctx, deleteNodeFeature, arg.NodeID, arg.FeatureID)
	return err
}

const deleteZombieChannel = `-- name: DeleteZombieChannel :exec
DELETE FROM zombie_channels
WHERE scid = $1
  AND version = $2
`

type DeleteZombieChannelParams struct {
	Scid    int64
	Version int16
}

func (q *Queries) DeleteZombieChannel(ctx context.Context, arg DeleteZombieChannelParams) error {
	_, err := q.db.ExecContext(ctx, deleteZombieChannel, arg.Scid, arg.Version)
	return err
}

const getChannelByOutpointAndVersion = `-- name: GetChannelByOutpointAndVersion :one
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity FROM channels
WHERE outpoint = $1 AND version = $2
`

type GetChannelByOutpointAndVersionParams struct {
	Outpoint string
	Version  int16
}

func (q *Queries) GetChannelByOutpointAndVersion(ctx context.Context, arg GetChannelByOutpointAndVersionParams) (Channel, error) {
	row := q.db.QueryRowContext(ctx, getChannelByOutpointAndVersion, arg.Outpoint, arg.Version)
	var i Channel
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.Scid,
		&i.NodeID1,
		&i.NodeID2,
		&i.Outpoint,
		&i.Capacity,
	)
	return i, err
}

const getChannelBySCIDAndVersion = `-- name: GetChannelBySCIDAndVersion :one
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity FROM channels
WHERE scid = $1 AND version = $2
`

type GetChannelBySCIDAndVersionParams struct {
	Scid    []byte
	Version int16
}

func (q *Queries) GetChannelBySCIDAndVersion(ctx context.Context, arg GetChannelBySCIDAndVersionParams) (Channel, error) {
	row := q.db.QueryRowContext(ctx, getChannelBySCIDAndVersion, arg.Scid, arg.Version)
	var i Channel
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.Scid,
		&i.NodeID1,
		&i.NodeID2,
		&i.Outpoint,
		&i.Capacity,
	)
	return i, err
}

const getChannelFeatures = `-- name: GetChannelFeatures :many
SELECT
    nf.channel_id,
    nf.feature_id,
    f.bit
FROM channel_features nf
         JOIN features f ON nf.feature_id = f.id
WHERE nf.channel_id = $1
`

type GetChannelFeaturesRow struct {
	ChannelID int64
	FeatureID int64
	Bit       int32
}

func (q *Queries) GetChannelFeatures(ctx context.Context, channelID int64) ([]GetChannelFeaturesRow, error) {
	rows, err := q.db.QueryContext(ctx, getChannelFeatures, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetChannelFeaturesRow
	for rows.Next() {
		var i GetChannelFeaturesRow
		if err := rows.Scan(&i.ChannelID, &i.FeatureID, &i.Bit); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getChannelPolicyByChannelAndNode = `-- name: GetChannelPolicyByChannelAndNode :one
SELECT id, channel_id, node_id, timelock, fee_ppm, base_fee_msat, min_htlc_msat, signature FROM channel_policies
WHERE channel_id = $1 AND node_id = $2
`

type GetChannelPolicyByChannelAndNodeParams struct {
	ChannelID int64
	NodeID    int64
}

func (q *Queries) GetChannelPolicyByChannelAndNode(ctx context.Context, arg GetChannelPolicyByChannelAndNodeParams) (ChannelPolicy, error) {
	row := q.db.QueryRowContext(ctx, getChannelPolicyByChannelAndNode, arg.ChannelID, arg.NodeID)
	var i ChannelPolicy
	err := row.Scan(
		&i.ID,
		&i.ChannelID,
		&i.NodeID,
		&i.Timelock,
		&i.FeePpm,
		&i.BaseFeeMsat,
		&i.MinHtlcMsat,
		&i.Signature,
	)
	return i, err
}

const getChannelPolicyExtraTypes = `-- name: GetChannelPolicyExtraTypes :many
/* ─────────────────────────────────────────────
   channel_policy_extra_types table queries
   ─────────────────────────────────────────────
*/

SELECT channel_policy_id, type, value FROM channel_policy_extra_types WHERE channel_policy_id = $1
`

func (q *Queries) GetChannelPolicyExtraTypes(ctx context.Context, channelPolicyID int64) ([]ChannelPolicyExtraType, error) {
	rows, err := q.db.QueryContext(ctx, getChannelPolicyExtraTypes, channelPolicyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ChannelPolicyExtraType
	for rows.Next() {
		var i ChannelPolicyExtraType
		if err := rows.Scan(&i.ChannelPolicyID, &i.Type, &i.Value); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getChannelPolicyV1Data = `-- name: GetChannelPolicyV1Data :one
SELECT channel_policy_id, last_update, disabled, max_htlc_msat FROM channel_policy_v1_data WHERE channel_policy_id = $1
`

func (q *Queries) GetChannelPolicyV1Data(ctx context.Context, channelPolicyID int64) (ChannelPolicyV1Datum, error) {
	row := q.db.QueryRowContext(ctx, getChannelPolicyV1Data, channelPolicyID)
	var i ChannelPolicyV1Datum
	err := row.Scan(
		&i.ChannelPolicyID,
		&i.LastUpdate,
		&i.Disabled,
		&i.MaxHtlcMsat,
	)
	return i, err
}

const getChannelsV1Data = `-- name: GetChannelsV1Data :one
SELECT channel_id, bitcoin_key_1, bitcoin_key_2 FROM channels_v1_data WHERE channel_id = $1
`

func (q *Queries) GetChannelsV1Data(ctx context.Context, channelID int64) (ChannelsV1Datum, error) {
	row := q.db.QueryRowContext(ctx, getChannelsV1Data, channelID)
	var i ChannelsV1Datum
	err := row.Scan(&i.ChannelID, &i.BitcoinKey1, &i.BitcoinKey2)
	return i, err
}

const getExtraChannelTypes = `-- name: GetExtraChannelTypes :many
SELECT channel_id, type, value
FROM channel_extra_types
WHERE channel_id = $1
`

func (q *Queries) GetExtraChannelTypes(ctx context.Context, channelID int64) ([]ChannelExtraType, error) {
	rows, err := q.db.QueryContext(ctx, getExtraChannelTypes, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ChannelExtraType
	for rows.Next() {
		var i ChannelExtraType
		if err := rows.Scan(&i.ChannelID, &i.Type, &i.Value); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getExtraNodeTypes = `-- name: GetExtraNodeTypes :many
SELECT node_id, type, value
FROM node_extra_types
WHERE node_id = $1
`

func (q *Queries) GetExtraNodeTypes(ctx context.Context, nodeID int64) ([]NodeExtraType, error) {
	rows, err := q.db.QueryContext(ctx, getExtraNodeTypes, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []NodeExtraType
	for rows.Next() {
		var i NodeExtraType
		if err := rows.Scan(&i.NodeID, &i.Type, &i.Value); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNodeAddresses = `-- name: GetNodeAddresses :many
SELECT type, address
FROM node_addresses
WHERE node_id = $1
ORDER BY type ASC, position ASC
`

type GetNodeAddressesRow struct {
	Type    int16
	Address string
}

func (q *Queries) GetNodeAddresses(ctx context.Context, nodeID int64) ([]GetNodeAddressesRow, error) {
	rows, err := q.db.QueryContext(ctx, getNodeAddresses, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNodeAddressesRow
	for rows.Next() {
		var i GetNodeAddressesRow
		if err := rows.Scan(&i.Type, &i.Address); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNodeAliasByPubKeyAndVersion = `-- name: GetNodeAliasByPubKeyAndVersion :one
SELECT alias
FROM nodes
WHERE pub_key = $1 AND version = $2
`

type GetNodeAliasByPubKeyAndVersionParams struct {
	PubKey  []byte
	Version int16
}

func (q *Queries) GetNodeAliasByPubKeyAndVersion(ctx context.Context, arg GetNodeAliasByPubKeyAndVersionParams) (sql.NullString, error) {
	row := q.db.QueryRowContext(ctx, getNodeAliasByPubKeyAndVersion, arg.PubKey, arg.Version)
	var alias sql.NullString
	err := row.Scan(&alias)
	return alias, err
}

const getNodeByID = `-- name: GetNodeByID :one
SELECT id, version, pub_key, alias, signature
FROM nodes
WHERE id = $1
`

func (q *Queries) GetNodeByID(ctx context.Context, id int64) (Node, error) {
	row := q.db.QueryRowContext(ctx, getNodeByID, id)
	var i Node
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.PubKey,
		&i.Alias,
		&i.Signature,
	)
	return i, err
}

const getNodeByPubKeyAndVersion = `-- name: GetNodeByPubKeyAndVersion :one
SELECT id, version, pub_key, alias, signature
FROM nodes
WHERE pub_key = $1
  AND version = $2
`

type GetNodeByPubKeyAndVersionParams struct {
	PubKey  []byte
	Version int16
}

func (q *Queries) GetNodeByPubKeyAndVersion(ctx context.Context, arg GetNodeByPubKeyAndVersionParams) (Node, error) {
	row := q.db.QueryRowContext(ctx, getNodeByPubKeyAndVersion, arg.PubKey, arg.Version)
	var i Node
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.PubKey,
		&i.Alias,
		&i.Signature,
	)
	return i, err
}

const getNodeFeatures = `-- name: GetNodeFeatures :many
SELECT
    nf.node_id,
    nf.feature_id,
    f.bit
FROM node_features nf
         JOIN features f ON nf.feature_id = f.id
WHERE nf.node_id = $1
`

type GetNodeFeaturesRow struct {
	NodeID    int64
	FeatureID int64
	Bit       int32
}

func (q *Queries) GetNodeFeatures(ctx context.Context, nodeID int64) ([]GetNodeFeaturesRow, error) {
	rows, err := q.db.QueryContext(ctx, getNodeFeatures, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNodeFeaturesRow
	for rows.Next() {
		var i GetNodeFeaturesRow
		if err := rows.Scan(&i.NodeID, &i.FeatureID, &i.Bit); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNodeIDByPubKeyAndVersion = `-- name: GetNodeIDByPubKeyAndVersion :one
SELECT id
FROM nodes
WHERE pub_key = $1
  AND version = $2
`

type GetNodeIDByPubKeyAndVersionParams struct {
	PubKey  []byte
	Version int16
}

func (q *Queries) GetNodeIDByPubKeyAndVersion(ctx context.Context, arg GetNodeIDByPubKeyAndVersionParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, getNodeIDByPubKeyAndVersion, arg.PubKey, arg.Version)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const getPublicV1ChannelsBySCID = `-- name: GetPublicV1ChannelsBySCID :many
SELECT c.id, c.version, c.scid, c.node_id_1, c.node_id_2, c.outpoint, c.capacity
FROM channels c
         JOIN v1_channel_proofs p ON p.channel_id = c.id
WHERE c.scid >= $1
  AND c.scid < $2
`

type GetPublicV1ChannelsBySCIDParams struct {
	StartScid []byte
	EndScid   []byte
}

func (q *Queries) GetPublicV1ChannelsBySCID(ctx context.Context, arg GetPublicV1ChannelsBySCIDParams) ([]Channel, error) {
	rows, err := q.db.QueryContext(ctx, getPublicV1ChannelsBySCID, arg.StartScid, arg.EndScid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Channel
	for rows.Next() {
		var i Channel
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.Scid,
			&i.NodeID1,
			&i.NodeID2,
			&i.Outpoint,
			&i.Capacity,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getSCIDByOutpointAndVersion = `-- name: GetSCIDByOutpointAndVersion :one
SELECT scid from channels
WHERE outpoint = $1 AND version = $2
`

type GetSCIDByOutpointAndVersionParams struct {
	Outpoint string
	Version  int16
}

func (q *Queries) GetSCIDByOutpointAndVersion(ctx context.Context, arg GetSCIDByOutpointAndVersionParams) ([]byte, error) {
	row := q.db.QueryRowContext(ctx, getSCIDByOutpointAndVersion, arg.Outpoint, arg.Version)
	var scid []byte
	err := row.Scan(&scid)
	return scid, err
}

const getSourceNodesByVersion = `-- name: GetSourceNodesByVersion :many
SELECT sn.node_id, n.pub_key
FROM source_nodes sn
   JOIN nodes n ON sn.node_id = n.id
WHERE n.version = $1
`

type GetSourceNodesByVersionRow struct {
	NodeID int64
	PubKey []byte
}

func (q *Queries) GetSourceNodesByVersion(ctx context.Context, version int16) ([]GetSourceNodesByVersionRow, error) {
	rows, err := q.db.QueryContext(ctx, getSourceNodesByVersion, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetSourceNodesByVersionRow
	for rows.Next() {
		var i GetSourceNodesByVersionRow
		if err := rows.Scan(&i.NodeID, &i.PubKey); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getV1ChannelPolicyByChannelAndNode = `-- name: GetV1ChannelPolicyByChannelAndNode :one
SELECT
    cp.id, cp.channel_id, cp.node_id, cp.timelock, cp.fee_ppm, cp.base_fee_msat, cp.min_htlc_msat, cp.signature,
    v1.channel_policy_id, v1.last_update, v1.disabled, v1.max_htlc_msat
FROM channel_policies cp
JOIN channel_policy_v1_data v1 ON cp.id = v1.channel_policy_id
WHERE channel_id = $1 AND node_id = $2
`

type GetV1ChannelPolicyByChannelAndNodeParams struct {
	ChannelID int64
	NodeID    int64
}

type GetV1ChannelPolicyByChannelAndNodeRow struct {
	ID              int64
	ChannelID       int64
	NodeID          int64
	Timelock        int32
	FeePpm          int64
	BaseFeeMsat     int64
	MinHtlcMsat     int64
	Signature       []byte
	ChannelPolicyID int64
	LastUpdate      int64
	Disabled        bool
	MaxHtlcMsat     sql.NullInt64
}

func (q *Queries) GetV1ChannelPolicyByChannelAndNode(ctx context.Context, arg GetV1ChannelPolicyByChannelAndNodeParams) (GetV1ChannelPolicyByChannelAndNodeRow, error) {
	row := q.db.QueryRowContext(ctx, getV1ChannelPolicyByChannelAndNode, arg.ChannelID, arg.NodeID)
	var i GetV1ChannelPolicyByChannelAndNodeRow
	err := row.Scan(
		&i.ID,
		&i.ChannelID,
		&i.NodeID,
		&i.Timelock,
		&i.FeePpm,
		&i.BaseFeeMsat,
		&i.MinHtlcMsat,
		&i.Signature,
		&i.ChannelPolicyID,
		&i.LastUpdate,
		&i.Disabled,
		&i.MaxHtlcMsat,
	)
	return i, err
}

const getV1ChannelProof = `-- name: GetV1ChannelProof :one
SELECT channel_id, node_1_signature, node_2_signature, bitcoin_1_signature, bitcoin_2_signature FROM v1_channel_proofs WHERE channel_id = $1
`

func (q *Queries) GetV1ChannelProof(ctx context.Context, channelID int64) (V1ChannelProof, error) {
	row := q.db.QueryRowContext(ctx, getV1ChannelProof, channelID)
	var i V1ChannelProof
	err := row.Scan(
		&i.ChannelID,
		&i.Node1Signature,
		&i.Node2Signature,
		&i.Bitcoin1Signature,
		&i.Bitcoin2Signature,
	)
	return i, err
}

const getV1ChannelsByPolicyLastUpdateRange = `-- name: GetV1ChannelsByPolicyLastUpdateRange :many
SELECT DISTINCT c.id, c.version, c.scid, c.node_id_1, c.node_id_2, c.outpoint, c.capacity
FROM channels c
         JOIN channel_policies cp ON cp.channel_id = c.id
         JOIN channel_policy_v1_data v1 ON v1.channel_policy_id = cp.id
WHERE v1.last_update >= $1
  AND v1.last_update < $2
`

type GetV1ChannelsByPolicyLastUpdateRangeParams struct {
	StartTime int64
	EndTime   int64
}

func (q *Queries) GetV1ChannelsByPolicyLastUpdateRange(ctx context.Context, arg GetV1ChannelsByPolicyLastUpdateRangeParams) ([]Channel, error) {
	rows, err := q.db.QueryContext(ctx, getV1ChannelsByPolicyLastUpdateRange, arg.StartTime, arg.EndTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Channel
	for rows.Next() {
		var i Channel
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.Scid,
			&i.NodeID1,
			&i.NodeID2,
			&i.Outpoint,
			&i.Capacity,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getV1DisabledSCIDs = `-- name: GetV1DisabledSCIDs :many
SELECT c.scid
FROM channels c
         JOIN channel_policies cp ON cp.channel_id = c.id
         JOIN channel_policy_v1_data v1 ON v1.channel_policy_id = cp.id
WHERE v1.disabled = true
GROUP BY c.scid
HAVING COUNT(*) > 1
`

func (q *Queries) GetV1DisabledSCIDs(ctx context.Context) ([][]byte, error) {
	rows, err := q.db.QueryContext(ctx, getV1DisabledSCIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items [][]byte
	for rows.Next() {
		var scid []byte
		if err := rows.Scan(&scid); err != nil {
			return nil, err
		}
		items = append(items, scid)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getV1NodeData = `-- name: GetV1NodeData :one
SELECT node_id, last_update, color
FROM nodes_v1_data
WHERE node_id = $1
`

func (q *Queries) GetV1NodeData(ctx context.Context, nodeID int64) (NodesV1Datum, error) {
	row := q.db.QueryRowContext(ctx, getV1NodeData, nodeID)
	var i NodesV1Datum
	err := row.Scan(&i.NodeID, &i.LastUpdate, &i.Color)
	return i, err
}

const getV1NodesByLastUpdateRange = `-- name: GetV1NodesByLastUpdateRange :many
SELECT n.id, n.version, n.pub_key, n.alias, n.signature
FROM nodes n
         JOIN nodes_v1_data v1 ON n.id = v1.node_id
WHERE n.version = 1
  AND v1.last_update >= $1
  AND v1.last_update < $2
`

type GetV1NodesByLastUpdateRangeParams struct {
	StartTime int64
	EndTime   int64
}

func (q *Queries) GetV1NodesByLastUpdateRange(ctx context.Context, arg GetV1NodesByLastUpdateRangeParams) ([]Node, error) {
	rows, err := q.db.QueryContext(ctx, getV1NodesByLastUpdateRange, arg.StartTime, arg.EndTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Node
	for rows.Next() {
		var i Node
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.PubKey,
			&i.Alias,
			&i.Signature,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getZombieChannel = `-- name: GetZombieChannel :one
SELECT scid, version, node_key_1, node_key_2
FROM zombie_channels
WHERE scid = $1
  AND version = $2
`

type GetZombieChannelParams struct {
	Scid    int64
	Version int16
}

func (q *Queries) GetZombieChannel(ctx context.Context, arg GetZombieChannelParams) (ZombieChannel, error) {
	row := q.db.QueryRowContext(ctx, getZombieChannel, arg.Scid, arg.Version)
	var i ZombieChannel
	err := row.Scan(
		&i.Scid,
		&i.Version,
		&i.NodeKey1,
		&i.NodeKey2,
	)
	return i, err
}

const highestSCID = `-- name: HighestSCID :one
SELECT scid
FROM channels
WHERE version = $1
ORDER BY scid DESC
LIMIT 1
`

func (q *Queries) HighestSCID(ctx context.Context, version int16) ([]byte, error) {
	row := q.db.QueryRowContext(ctx, highestSCID, version)
	var scid []byte
	err := row.Scan(&scid)
	return scid, err
}

const insertChannelFeature = `-- name: InsertChannelFeature :exec
/* ─────────────────────────────────────────────
   channel_features table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channel_features (
    channel_id, feature_id
) VALUES (
    $1, $2
)
`

type InsertChannelFeatureParams struct {
	ChannelID int64
	FeatureID int64
}

func (q *Queries) InsertChannelFeature(ctx context.Context, arg InsertChannelFeatureParams) error {
	_, err := q.db.ExecContext(ctx, insertChannelFeature, arg.ChannelID, arg.FeatureID)
	return err
}

const insertNodeAddress = `-- name: InsertNodeAddress :exec
/* ─────────────────────────────────────────────
   node_addresses table queries
   ─────────────────────────────────────────────
*/

INSERT INTO node_addresses (
    node_id,
    type,
    address,
    position
) VALUES (
    $1, $2, $3, $4
)
`

type InsertNodeAddressParams struct {
	NodeID   int64
	Type     int16
	Address  string
	Position int32
}

func (q *Queries) InsertNodeAddress(ctx context.Context, arg InsertNodeAddressParams) error {
	_, err := q.db.ExecContext(ctx, insertNodeAddress,
		arg.NodeID,
		arg.Type,
		arg.Address,
		arg.Position,
	)
	return err
}

const insertNodeFeature = `-- name: InsertNodeFeature :exec
/* ─────────────────────────────────────────────
   node_features table queries
   ─────────────────────────────────────────────
*/

INSERT INTO node_features (
    node_id, feature_id
) VALUES (
    $1, $2
)
`

type InsertNodeFeatureParams struct {
	NodeID    int64
	FeatureID int64
}

func (q *Queries) InsertNodeFeature(ctx context.Context, arg InsertNodeFeatureParams) error {
	_, err := q.db.ExecContext(ctx, insertNodeFeature, arg.NodeID, arg.FeatureID)
	return err
}

const isPublicV1Node = `-- name: IsPublicV1Node :one
SELECT EXISTS (
    SELECT 1
    FROM channels c
             JOIN v1_channel_proofs v1p ON c.id = v1p.channel_id
             JOIN nodes n ON n.id = c.node_id_1 OR n.id = c.node_id_2
    WHERE n.pub_key = $1
)
`

func (q *Queries) IsPublicV1Node(ctx context.Context, pubKey []byte) (bool, error) {
	row := q.db.QueryRowContext(ctx, isPublicV1Node, pubKey)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

const isZombieChannel = `-- name: IsZombieChannel :one
SELECT EXISTS (
    SELECT 1
    FROM zombie_channels
    WHERE scid = $1
    AND version = $2
) AS is_zombie
`

type IsZombieChannelParams struct {
	Scid    int64
	Version int16
}

func (q *Queries) IsZombieChannel(ctx context.Context, arg IsZombieChannelParams) (bool, error) {
	row := q.db.QueryRowContext(ctx, isZombieChannel, arg.Scid, arg.Version)
	var is_zombie bool
	err := row.Scan(&is_zombie)
	return is_zombie, err
}

const listAllChannelsByVersion = `-- name: ListAllChannelsByVersion :many
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity FROM channels
WHERE version = $1
`

func (q *Queries) ListAllChannelsByVersion(ctx context.Context, version int16) ([]Channel, error) {
	rows, err := q.db.QueryContext(ctx, listAllChannelsByVersion, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Channel
	for rows.Next() {
		var i Channel
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.Scid,
			&i.NodeID1,
			&i.NodeID2,
			&i.Outpoint,
			&i.Capacity,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listChannelsByNodeIDAndVersion = `-- name: ListChannelsByNodeIDAndVersion :many
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity FROM channels
WHERE version = $1
  AND (node_id_1 = $2 OR node_id_2 = $2)
`

type ListChannelsByNodeIDAndVersionParams struct {
	Version int16
	NodeID1 int64
}

func (q *Queries) ListChannelsByNodeIDAndVersion(ctx context.Context, arg ListChannelsByNodeIDAndVersionParams) ([]Channel, error) {
	rows, err := q.db.QueryContext(ctx, listChannelsByNodeIDAndVersion, arg.Version, arg.NodeID1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Channel
	for rows.Next() {
		var i Channel
		if err := rows.Scan(
			&i.ID,
			&i.Version,
			&i.Scid,
			&i.NodeID1,
			&i.NodeID2,
			&i.Outpoint,
			&i.Capacity,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listNodeIDsAndPubKeysByVersion = `-- name: ListNodeIDsAndPubKeysByVersion :many
SELECT id, pub_key
FROM nodes
WHERE version = $1
`

type ListNodeIDsAndPubKeysByVersionRow struct {
	ID     int64
	PubKey []byte
}

func (q *Queries) ListNodeIDsAndPubKeysByVersion(ctx context.Context, version int16) ([]ListNodeIDsAndPubKeysByVersionRow, error) {
	rows, err := q.db.QueryContext(ctx, listNodeIDsAndPubKeysByVersion, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListNodeIDsAndPubKeysByVersionRow
	for rows.Next() {
		var i ListNodeIDsAndPubKeysByVersionRow
		if err := rows.Scan(&i.ID, &i.PubKey); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const listNodesByVersion = `-- name: ListNodesByVersion :many
SELECT id, pub_key
FROM nodes
WHERE version = $1
`

type ListNodesByVersionRow struct {
	ID     int64
	PubKey []byte
}

func (q *Queries) ListNodesByVersion(ctx context.Context, version int16) ([]ListNodesByVersionRow, error) {
	rows, err := q.db.QueryContext(ctx, listNodesByVersion, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListNodesByVersionRow
	for rows.Next() {
		var i ListNodesByVersionRow
		if err := rows.Scan(&i.ID, &i.PubKey); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const updateChannelPolicy = `-- name: UpdateChannelPolicy :exec
UPDATE channel_policies
SET timelock = $2,
    fee_ppm = $3,
    base_fee_msat = $4,
    min_htlc_msat = $5,
    signature = $6
WHERE id = $1
`

type UpdateChannelPolicyParams struct {
	ID          int64
	Timelock    int32
	FeePpm      int64
	BaseFeeMsat int64
	MinHtlcMsat int64
	Signature   []byte
}

func (q *Queries) UpdateChannelPolicy(ctx context.Context, arg UpdateChannelPolicyParams) error {
	_, err := q.db.ExecContext(ctx, updateChannelPolicy,
		arg.ID,
		arg.Timelock,
		arg.FeePpm,
		arg.BaseFeeMsat,
		arg.MinHtlcMsat,
		arg.Signature,
	)
	return err
}

const updateChannelPolicyV1Data = `-- name: UpdateChannelPolicyV1Data :exec
UPDATE channel_policy_v1_data
SET last_update = $2,
    disabled = $3,
    max_htlc_msat = $4
WHERE channel_policy_id = $1
`

type UpdateChannelPolicyV1DataParams struct {
	ChannelPolicyID int64
	LastUpdate      int64
	Disabled        bool
	MaxHtlcMsat     sql.NullInt64
}

func (q *Queries) UpdateChannelPolicyV1Data(ctx context.Context, arg UpdateChannelPolicyV1DataParams) error {
	_, err := q.db.ExecContext(ctx, updateChannelPolicyV1Data,
		arg.ChannelPolicyID,
		arg.LastUpdate,
		arg.Disabled,
		arg.MaxHtlcMsat,
	)
	return err
}

const updateNode = `-- name: UpdateNode :exec
UPDATE nodes
SET alias = $2,
    signature = $3
WHERE id = $1
`

type UpdateNodeParams struct {
	ID        int64
	Alias     sql.NullString
	Signature []byte
}

func (q *Queries) UpdateNode(ctx context.Context, arg UpdateNodeParams) error {
	_, err := q.db.ExecContext(ctx, updateNode, arg.ID, arg.Alias, arg.Signature)
	return err
}

const upsertChannelExtraType = `-- name: UpsertChannelExtraType :exec
/* ─────────────────────────────────────────────
   channel_extra_types table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channel_extra_types (
    channel_id, type, value
)
VALUES ($1, $2, $3)
ON CONFLICT (type, channel_id)
    DO UPDATE SET value = EXCLUDED.value
`

type UpsertChannelExtraTypeParams struct {
	ChannelID int64
	Type      int64
	Value     []byte
}

func (q *Queries) UpsertChannelExtraType(ctx context.Context, arg UpsertChannelExtraTypeParams) error {
	_, err := q.db.ExecContext(ctx, upsertChannelExtraType, arg.ChannelID, arg.Type, arg.Value)
	return err
}

const upsertNodeExtraType = `-- name: UpsertNodeExtraType :exec
/* ─────────────────────────────────────────────
   node_extra_types table queries
   ─────────────────────────────────────────────
*/

INSERT INTO node_extra_types (
    node_id, type, value
)
VALUES ($1, $2, $3)
ON CONFLICT (type, node_id)
    DO UPDATE SET value = EXCLUDED.value
`

type UpsertNodeExtraTypeParams struct {
	NodeID int64
	Type   int64
	Value  []byte
}

func (q *Queries) UpsertNodeExtraType(ctx context.Context, arg UpsertNodeExtraTypeParams) error {
	_, err := q.db.ExecContext(ctx, upsertNodeExtraType, arg.NodeID, arg.Type, arg.Value)
	return err
}

const upsertV1NodeData = `-- name: UpsertV1NodeData :exec
/* ─────────────────────────────────────────────
   nodes_v1_data table queries
   ─────────────────────────────────────────────
*/

INSERT INTO nodes_v1_data (
    node_id, last_update, color
)
VALUES ($1, $2, $3)
ON CONFLICT (node_id) DO UPDATE
    SET last_update = EXCLUDED.last_update,
        color = EXCLUDED.color
`

type UpsertV1NodeDataParams struct {
	NodeID     int64
	LastUpdate int64
	Color      string
}

func (q *Queries) UpsertV1NodeData(ctx context.Context, arg UpsertV1NodeDataParams) error {
	_, err := q.db.ExecContext(ctx, upsertV1NodeData, arg.NodeID, arg.LastUpdate, arg.Color)
	return err
}

const upsertZombieChannel = `-- name: UpsertZombieChannel :exec
/* ─────────────────────────────────────────────
   zombie_channels table queries
   ─────────────────────────────────────────────
*/

INSERT INTO zombie_channels (scid, version, node_key_1, node_key_2)
VALUES ($1, $2, $3, $4)
ON CONFLICT (scid, version)
    DO UPDATE SET
                  node_key_1 = COALESCE(EXCLUDED.node_key_1, zombie_channels.node_key_1),
                  node_key_2 = COALESCE(EXCLUDED.node_key_2, zombie_channels.node_key_2)
`

type UpsertZombieChannelParams struct {
	Scid     int64
	Version  int16
	NodeKey1 []byte
	NodeKey2 []byte
}

func (q *Queries) UpsertZombieChannel(ctx context.Context, arg UpsertZombieChannelParams) error {
	_, err := q.db.ExecContext(ctx, upsertZombieChannel,
		arg.Scid,
		arg.Version,
		arg.NodeKey1,
		arg.NodeKey2,
	)
	return err
}
