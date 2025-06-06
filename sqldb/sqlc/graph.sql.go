// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0
// source: graph.sql

package sqlc

import (
	"context"
	"database/sql"
)

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

const createChannel = `-- name: CreateChannel :one
/* ─────────────────────────────────────────────
   channels table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channels (
    version, scid, node_id_1, node_id_2,
    outpoint, capacity, bitcoin_key_1, bitcoin_key_2,
    node_1_signature, node_2_signature, bitcoin_1_signature,
    bitcoin_2_signature
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING id
`

type CreateChannelParams struct {
	Version           int16
	Scid              []byte
	NodeID1           int64
	NodeID2           int64
	Outpoint          string
	Capacity          sql.NullInt64
	BitcoinKey1       []byte
	BitcoinKey2       []byte
	Node1Signature    []byte
	Node2Signature    []byte
	Bitcoin1Signature []byte
	Bitcoin2Signature []byte
}

func (q *Queries) CreateChannel(ctx context.Context, arg CreateChannelParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, createChannel,
		arg.Version,
		arg.Scid,
		arg.NodeID1,
		arg.NodeID2,
		arg.Outpoint,
		arg.Capacity,
		arg.BitcoinKey1,
		arg.BitcoinKey2,
		arg.Node1Signature,
		arg.Node2Signature,
		arg.Bitcoin1Signature,
		arg.Bitcoin2Signature,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const createChannelExtraType = `-- name: CreateChannelExtraType :exec
/* ─────────────────────────────────────────────
   channel_extra_types table queries
   ─────────────────────────────────────────────
*/

INSERT INTO channel_extra_types (
    channel_id, type, value
)
VALUES ($1, $2, $3)
`

type CreateChannelExtraTypeParams struct {
	ChannelID int64
	Type      int64
	Value     []byte
}

func (q *Queries) CreateChannelExtraType(ctx context.Context, arg CreateChannelExtraTypeParams) error {
	_, err := q.db.ExecContext(ctx, createChannelExtraType, arg.ChannelID, arg.Type, arg.Value)
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

const deleteNodeAddresses = `-- name: DeleteNodeAddresses :exec
DELETE FROM node_addresses
WHERE node_id = $1
`

func (q *Queries) DeleteNodeAddresses(ctx context.Context, nodeID int64) error {
	_, err := q.db.ExecContext(ctx, deleteNodeAddresses, nodeID)
	return err
}

const deleteNodeByPubKey = `-- name: DeleteNodeByPubKey :execresult
DELETE FROM nodes
WHERE pub_key = $1
  AND version = $2
`

type DeleteNodeByPubKeyParams struct {
	PubKey  []byte
	Version int16
}

func (q *Queries) DeleteNodeByPubKey(ctx context.Context, arg DeleteNodeByPubKeyParams) (sql.Result, error) {
	return q.db.ExecContext(ctx, deleteNodeByPubKey, arg.PubKey, arg.Version)
}

const deleteNodeFeature = `-- name: DeleteNodeFeature :exec
DELETE FROM node_features
WHERE node_id = $1
  AND feature_bit = $2
`

type DeleteNodeFeatureParams struct {
	NodeID     int64
	FeatureBit int32
}

func (q *Queries) DeleteNodeFeature(ctx context.Context, arg DeleteNodeFeatureParams) error {
	_, err := q.db.ExecContext(ctx, deleteNodeFeature, arg.NodeID, arg.FeatureBit)
	return err
}

const getChannelBySCID = `-- name: GetChannelBySCID :one
SELECT id, version, scid, node_id_1, node_id_2, outpoint, capacity, bitcoin_key_1, bitcoin_key_2, node_1_signature, node_2_signature, bitcoin_1_signature, bitcoin_2_signature FROM channels
WHERE scid = $1 AND version = $2
`

type GetChannelBySCIDParams struct {
	Scid    []byte
	Version int16
}

func (q *Queries) GetChannelBySCID(ctx context.Context, arg GetChannelBySCIDParams) (Channel, error) {
	row := q.db.QueryRowContext(ctx, getChannelBySCID, arg.Scid, arg.Version)
	var i Channel
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.Scid,
		&i.NodeID1,
		&i.NodeID2,
		&i.Outpoint,
		&i.Capacity,
		&i.BitcoinKey1,
		&i.BitcoinKey2,
		&i.Node1Signature,
		&i.Node2Signature,
		&i.Bitcoin1Signature,
		&i.Bitcoin2Signature,
	)
	return i, err
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

const getNodeAddressesByPubKey = `-- name: GetNodeAddressesByPubKey :many
SELECT a.type, a.address
FROM nodes n
LEFT JOIN node_addresses a ON a.node_id = n.id
WHERE n.pub_key = $1 AND n.version = $2
ORDER BY a.type ASC, a.position ASC
`

type GetNodeAddressesByPubKeyParams struct {
	PubKey  []byte
	Version int16
}

type GetNodeAddressesByPubKeyRow struct {
	Type    sql.NullInt16
	Address sql.NullString
}

func (q *Queries) GetNodeAddressesByPubKey(ctx context.Context, arg GetNodeAddressesByPubKeyParams) ([]GetNodeAddressesByPubKeyRow, error) {
	rows, err := q.db.QueryContext(ctx, getNodeAddressesByPubKey, arg.PubKey, arg.Version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetNodeAddressesByPubKeyRow
	for rows.Next() {
		var i GetNodeAddressesByPubKeyRow
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

const getNodeByPubKey = `-- name: GetNodeByPubKey :one
SELECT id, version, pub_key, alias, last_update, color, signature
FROM nodes
WHERE pub_key = $1
  AND version = $2
`

type GetNodeByPubKeyParams struct {
	PubKey  []byte
	Version int16
}

func (q *Queries) GetNodeByPubKey(ctx context.Context, arg GetNodeByPubKeyParams) (Node, error) {
	row := q.db.QueryRowContext(ctx, getNodeByPubKey, arg.PubKey, arg.Version)
	var i Node
	err := row.Scan(
		&i.ID,
		&i.Version,
		&i.PubKey,
		&i.Alias,
		&i.LastUpdate,
		&i.Color,
		&i.Signature,
	)
	return i, err
}

const getNodeFeatures = `-- name: GetNodeFeatures :many
SELECT node_id, feature_bit
FROM node_features
WHERE node_id = $1
`

func (q *Queries) GetNodeFeatures(ctx context.Context, nodeID int64) ([]NodeFeature, error) {
	rows, err := q.db.QueryContext(ctx, getNodeFeatures, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []NodeFeature
	for rows.Next() {
		var i NodeFeature
		if err := rows.Scan(&i.NodeID, &i.FeatureBit); err != nil {
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

const getNodeFeaturesByPubKey = `-- name: GetNodeFeaturesByPubKey :many
SELECT f.feature_bit
FROM nodes n
    JOIN node_features f ON f.node_id = n.id
WHERE n.pub_key = $1
  AND n.version = $2
`

type GetNodeFeaturesByPubKeyParams struct {
	PubKey  []byte
	Version int16
}

func (q *Queries) GetNodeFeaturesByPubKey(ctx context.Context, arg GetNodeFeaturesByPubKeyParams) ([]int32, error) {
	rows, err := q.db.QueryContext(ctx, getNodeFeaturesByPubKey, arg.PubKey, arg.Version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []int32
	for rows.Next() {
		var feature_bit int32
		if err := rows.Scan(&feature_bit); err != nil {
			return nil, err
		}
		items = append(items, feature_bit)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getNodesByLastUpdateRange = `-- name: GetNodesByLastUpdateRange :many
SELECT id, version, pub_key, alias, last_update, color, signature
FROM nodes
WHERE last_update >= $1
  AND last_update < $2
`

type GetNodesByLastUpdateRangeParams struct {
	StartTime sql.NullInt64
	EndTime   sql.NullInt64
}

func (q *Queries) GetNodesByLastUpdateRange(ctx context.Context, arg GetNodesByLastUpdateRangeParams) ([]Node, error) {
	rows, err := q.db.QueryContext(ctx, getNodesByLastUpdateRange, arg.StartTime, arg.EndTime)
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
			&i.LastUpdate,
			&i.Color,
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
    channel_id, feature_bit
) VALUES (
    $1, $2
)
`

type InsertChannelFeatureParams struct {
	ChannelID  int64
	FeatureBit int32
}

func (q *Queries) InsertChannelFeature(ctx context.Context, arg InsertChannelFeatureParams) error {
	_, err := q.db.ExecContext(ctx, insertChannelFeature, arg.ChannelID, arg.FeatureBit)
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
    node_id, feature_bit
) VALUES (
    $1, $2
)
`

type InsertNodeFeatureParams struct {
	NodeID     int64
	FeatureBit int32
}

func (q *Queries) InsertNodeFeature(ctx context.Context, arg InsertNodeFeatureParams) error {
	_, err := q.db.ExecContext(ctx, insertNodeFeature, arg.NodeID, arg.FeatureBit)
	return err
}

const upsertNode = `-- name: UpsertNode :one
/* ─────────────────────────────────────────────
   nodes table queries
   ─────────────────────────────────────────────
*/

INSERT INTO nodes (
    version, pub_key, alias, last_update, color, signature
) VALUES (
    $1, $2, $3, $4, $5, $6
)
ON CONFLICT (pub_key, version)
    -- Update the following fields if a conflict occurs on pub_key
    -- and version.
    DO UPDATE SET
        alias = EXCLUDED.alias,
        last_update = EXCLUDED.last_update,
        color = EXCLUDED.color,
        signature = EXCLUDED.signature
WHERE nodes.last_update IS NULL
    OR EXCLUDED.last_update > nodes.last_update
RETURNING id
`

type UpsertNodeParams struct {
	Version    int16
	PubKey     []byte
	Alias      sql.NullString
	LastUpdate sql.NullInt64
	Color      sql.NullString
	Signature  []byte
}

func (q *Queries) UpsertNode(ctx context.Context, arg UpsertNodeParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, upsertNode,
		arg.Version,
		arg.PubKey,
		arg.Alias,
		arg.LastUpdate,
		arg.Color,
		arg.Signature,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
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
    -- Update the value if a conflict occurs on type
    -- and node_id.
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
