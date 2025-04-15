// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0
// source: channels.sql

package sqlc

import (
	"context"
)

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

const createChannelsV1Data = `-- name: CreateChannelsV1Data :exec
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

const createV1ChannelProof = `-- name: CreateV1ChannelProof :exec
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

const deletePruneLogEntriesInRange = `-- name: DeletePruneLogEntriesInRange :exec
DELETE FROM prune_log
WHERE block_height >= $1
  AND block_height <= $2
`

type DeletePruneLogEntriesInRangeParams struct {
	StartHeight int64
	EndHeight   int64
}

func (q *Queries) DeletePruneLogEntriesInRange(ctx context.Context, arg DeletePruneLogEntriesInRangeParams) error {
	_, err := q.db.ExecContext(ctx, deletePruneLogEntriesInRange, arg.StartHeight, arg.EndHeight)
	return err
}

const deletePruneLogEntry = `-- name: DeletePruneLogEntry :exec
DELETE FROM prune_log
WHERE block_height = $1
`

func (q *Queries) DeletePruneLogEntry(ctx context.Context, blockHeight int64) error {
	_, err := q.db.ExecContext(ctx, deletePruneLogEntry, blockHeight)
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

const getChannelByID = `-- name: GetChannelByID :one
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity FROM channels
WHERE id = $1
`

func (q *Queries) GetChannelByID(ctx context.Context, id int64) (Channel, error) {
	row := q.db.QueryRowContext(ctx, getChannelByID, id)
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

const getChannelByOutpoint = `-- name: GetChannelByOutpoint :one
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity FROM channels
WHERE outpoint = $1
`

func (q *Queries) GetChannelByOutpoint(ctx context.Context, outpoint string) (Channel, error) {
	row := q.db.QueryRowContext(ctx, getChannelByOutpoint, outpoint)
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

const getChannelsBySCIDRange = `-- name: GetChannelsBySCIDRange :many
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity
FROM channels
WHERE scid >= $1
  AND scid < $2
`

type GetChannelsBySCIDRangeParams struct {
	StartScid []byte
	EndScid   []byte
}

func (q *Queries) GetChannelsBySCIDRange(ctx context.Context, arg GetChannelsBySCIDRangeParams) ([]Channel, error) {
	rows, err := q.db.QueryContext(ctx, getChannelsBySCIDRange, arg.StartScid, arg.EndScid)
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

const getPruneTip = `-- name: GetPruneTip :one
SELECT block_height, block_hash
FROM prune_log
ORDER BY block_height DESC
LIMIT 1
`

func (q *Queries) GetPruneTip(ctx context.Context) (PruneLog, error) {
	row := q.db.QueryRowContext(ctx, getPruneTip)
	var i PruneLog
	err := row.Scan(&i.BlockHeight, &i.BlockHash)
	return i, err
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

const getUnconnectedNodes = `-- name: GetUnconnectedNodes :many
SELECT n.id, n.pub_key
FROM nodes n
WHERE NOT EXISTS (
    SELECT 1
    FROM channels c
    WHERE c.node_id_1 = n.id OR c.node_id_2 = n.id
)
`

type GetUnconnectedNodesRow struct {
	ID     int64
	PubKey []byte
}

func (q *Queries) GetUnconnectedNodes(ctx context.Context) ([]GetUnconnectedNodesRow, error) {
	rows, err := q.db.QueryContext(ctx, getUnconnectedNodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetUnconnectedNodesRow
	for rows.Next() {
		var i GetUnconnectedNodesRow
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

const insertClosedChannel = `-- name: InsertClosedChannel :exec
INSERT INTO closed_scids (
    scid
) VALUES (
    $1
)
ON CONFLICT (scid) DO NOTHING
`

func (q *Queries) InsertClosedChannel(ctx context.Context, scid []byte) error {
	_, err := q.db.ExecContext(ctx, insertClosedChannel, scid)
	return err
}

const isClosedChannel = `-- name: IsClosedChannel :one
SELECT EXISTS (
    SELECT 1
    FROM closed_scids
    WHERE scid = $1
)
`

func (q *Queries) IsClosedChannel(ctx context.Context, scid []byte) (bool, error) {
	row := q.db.QueryRowContext(ctx, isClosedChannel, scid)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

const isV1ChannelPublic = `-- name: IsV1ChannelPublic :one
SELECT EXISTS (
    SELECT 1
    FROM v1_channel_proofs
    WHERE channel_id = $1
)
`

func (q *Queries) IsV1ChannelPublic(ctx context.Context, channelID int64) (bool, error) {
	row := q.db.QueryRowContext(ctx, isV1ChannelPublic, channelID)
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

const updateChannelsV1Data = `-- name: UpdateChannelsV1Data :exec
UPDATE channels_v1_data
SET bitcoin_key_1 = $2, bitcoin_key_2 = $3
WHERE channel_id = $1
`

type UpdateChannelsV1DataParams struct {
	ChannelID   int64
	BitcoinKey1 []byte
	BitcoinKey2 []byte
}

func (q *Queries) UpdateChannelsV1Data(ctx context.Context, arg UpdateChannelsV1DataParams) error {
	_, err := q.db.ExecContext(ctx, updateChannelsV1Data, arg.ChannelID, arg.BitcoinKey1, arg.BitcoinKey2)
	return err
}

const upsertChannelExtraType = `-- name: UpsertChannelExtraType :exec
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

const upsertPruneLogEntry = `-- name: UpsertPruneLogEntry :exec
INSERT INTO prune_log (
    block_height, block_hash
) VALUES (
    $1, $2
)
ON CONFLICT(block_height) DO UPDATE SET
    block_hash = EXCLUDED.block_hash
`

type UpsertPruneLogEntryParams struct {
	BlockHeight int64
	BlockHash   []byte
}

func (q *Queries) UpsertPruneLogEntry(ctx context.Context, arg UpsertPruneLogEntryParams) error {
	_, err := q.db.ExecContext(ctx, upsertPruneLogEntry, arg.BlockHeight, arg.BlockHash)
	return err
}

const upsertZombieChannel = `-- name: UpsertZombieChannel :exec
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
