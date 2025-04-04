-- name: CreateNode :one
INSERT INTO nodes (
    version, pub_key, alias, signature
)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: GetNodeByID :one
SELECT *
FROM nodes
WHERE id = $1;

-- name: ListNodesByVersion :many
SELECT id, pub_key
FROM nodes
WHERE version = $1;

-- name: GetNodeByPubKeyAndVersion :one
SELECT *
FROM nodes
WHERE pub_key = $1
AND version = $2;

-- name: UpdateNode :exec
UPDATE nodes
SET alias = $2,
    signature = $3
WHERE id = $1;

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = $1;

-- name: GetV1NodesByLastUpdateRange :many
SELECT n.*
FROM nodes n
JOIN nodes_v1_data v1 ON n.id = v1.node_id
WHERE n.version = 1
  AND v1.last_update >= sqlc.arg(start_time)
  AND v1.last_update < sqlc.arg(end_time);

-- name: UpsertV1NodeData :exec
INSERT INTO nodes_v1_data (
    node_id, last_update, color
)
VALUES ($1, $2, $3)
ON CONFLICT (node_id) DO UPDATE
    SET last_update = EXCLUDED.last_update,
        color = EXCLUDED.color;

-- name: GetV1NodeData :one
SELECT *
FROM nodes_v1_data
WHERE node_id = $1;

-- name: UpsertNodeExtraType :exec
INSERT INTO node_extra_types (
    node_id, type, value
)
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

-- name: CreateFeature :one
INSERT INTO features (bit)
VALUES ($1)
ON CONFLICT (bit) DO UPDATE SET bit = EXCLUDED.bit
RETURNING id;

-- name: InsertNodeFeature :exec
INSERT INTO node_features (
    node_id, feature_id
) VALUES (
    $1, $2
);

-- name: GetNodeFeatures :many
SELECT
    nf.node_id,
    nf.feature_id,
    f.bit
FROM node_features nf
         JOIN features f ON nf.feature_id = f.id
WHERE nf.node_id = $1;

-- name: DeleteNodeFeature :exec
DELETE FROM node_features
WHERE node_id = $1
  AND feature_id = $2;

-- name: InsertNodeAddress :exec
INSERT INTO node_addresses (
    node_id,
    type,
    address,
    position
) VALUES (
    $1, $2, $3, $4
);

-- name: GetNodeAddresses :many
SELECT type, address
FROM node_addresses
WHERE node_id = $1
ORDER BY type ASC, position ASC;

-- name: DeleteNodeAddresses :exec
DELETE FROM node_addresses
WHERE node_id = $1;

-- name: AddSourceNode :exec
INSERT INTO source_nodes (node_id)
VALUES ($1)
ON CONFLICT (node_id) DO NOTHING;

-- name: GetSourceNodesByVersion :many
SELECT sn.node_id
FROM source_nodes sn
JOIN nodes n ON sn.node_id = n.id
WHERE n.version = $1;

-- name: GetNodeAliasByPubKeyAndVersion :one
SELECT alias
FROM nodes
WHERE pub_key = $1 AND version = $2;

-- name: ListNodeIDsAndPubKeysV1 :many
SELECT id, pub_key
FROM nodes
WHERE version = 1;
