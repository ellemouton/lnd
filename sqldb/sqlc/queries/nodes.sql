-- name: InsertNode :one
INSERT INTO nodes (
    pub_key, alias, block_height, signature, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: GetNode :one
SELECT *
FROM nodes
WHERE id = $1;

-- name: GetNodeIDByPubKey :one
SELECT id
FROM nodes
WHERE pub_key = $1;

-- name: GetNodeByPubKey :one
SELECT *
FROM nodes
WHERE pub_key = $1;

-- name: GetNodeAliasByPubKey :one
SELECT alias
FROM nodes
WHERE pub_key = $1;

-- name: UpdateNode :exec
UPDATE nodes
SET block_height = $2,
    alias = $3,
    signature = $4,
    updated_at = $5
WHERE id = $1;

-- name: DeleteNode :exec
DELETE FROM nodes
WHERE id = $1;

-- name: InsertNodeFeature :exec
INSERT INTO node_features (
    node_id, feature
) VALUES (
    $1, $2
);

-- name: GetNodeFeatures :many
SELECT *
FROM node_features
WHERE node_id = $1;

-- name: DeleteNodeFeature :exec
DELETE FROM node_features
WHERE node_id = $1
  AND feature = $2;

-- name: InsertNodeAddress :exec
INSERT INTO node_addresses (
    node_id, type, address
) VALUES (
    $1, $2, $3
);

-- name: GetNodeAddresses :many
SELECT *
FROM node_addresses
WHERE node_id = $1;

-- name: DeleteNodeAddress :exec
DELETE FROM node_addresses
WHERE node_id = $1
  AND address = $2;

-- name: UpsertNodeExtraType :exec
INSERT INTO node_extra_types (node_id, type, value)
VALUES ($1, $2, $3)
ON CONFLICT (type, node_id)
    DO UPDATE SET value = EXCLUDED.value;

-- name: GetExtraNodeTypes :many
SELECT *
FROM node_extra_types
WHERE node_id = $1;

-- name: DeleteExtraNodeType :exec
DELETE FROM node_extra_types
WHERE node_id = $1
  AND type = $2;

-- name: SetSourceNode :exec
INSERT INTO source_node (node_id)
VALUES ($1)
ON CONFLICT (node_id) DO NOTHING;

-- name: GetSourceNode :one
SELECT node_id
FROM source_node;
