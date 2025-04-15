// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0
// source: nodes.sql

package sqlc

import (
	"context"
	"database/sql"
)

const addSourceNode = `-- name: AddSourceNode :exec
INSERT INTO source_nodes (node_id)
VALUES ($1)
ON CONFLICT (node_id) DO NOTHING
`

func (q *Queries) AddSourceNode(ctx context.Context, nodeID int64) error {
	_, err := q.db.ExecContext(ctx, addSourceNode, nodeID)
	return err
}

const createFeature = `-- name: CreateFeature :one
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

const getSourceNodes = `-- name: GetSourceNodes :many
SELECT sn.node_id, n.pub_key, n.version
FROM source_nodes sn
JOIN nodes n ON sn.node_id = n.id
`

type GetSourceNodesRow struct {
	NodeID  int64
	PubKey  []byte
	Version int16
}

func (q *Queries) GetSourceNodes(ctx context.Context) ([]GetSourceNodesRow, error) {
	rows, err := q.db.QueryContext(ctx, getSourceNodes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetSourceNodesRow
	for rows.Next() {
		var i GetSourceNodesRow
		if err := rows.Scan(&i.NodeID, &i.PubKey, &i.Version); err != nil {
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

const insertNodeAddress = `-- name: InsertNodeAddress :exec
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

const listNodeIDsAndPubKeysV1 = `-- name: ListNodeIDsAndPubKeysV1 :many
SELECT id, pub_key
FROM nodes
WHERE version = 1
`

type ListNodeIDsAndPubKeysV1Row struct {
	ID     int64
	PubKey []byte
}

func (q *Queries) ListNodeIDsAndPubKeysV1(ctx context.Context) ([]ListNodeIDsAndPubKeysV1Row, error) {
	rows, err := q.db.QueryContext(ctx, listNodeIDsAndPubKeysV1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListNodeIDsAndPubKeysV1Row
	for rows.Next() {
		var i ListNodeIDsAndPubKeysV1Row
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

const upsertNodeExtraType = `-- name: UpsertNodeExtraType :exec
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
