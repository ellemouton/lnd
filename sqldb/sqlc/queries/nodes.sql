-- name: InsertNode :one
INSERT INTO nodes (
    pub_key, block_height, alias,  serialised_announcement
) VALUES (
    $1, $2, $3, $4
) RETURNING id;

-- name: UpdateNode :exec
UPDATE nodes
SET block_height = $2,
    alias = $3,
    serialised_announcement = $4
WHERE id = $1;

-- name: DeleteNode :exec
DELETE FROM nodes
WHERE id = $1;

-- name: GetNode :one
SELECT *
FROM nodes
WHERE id = $1;

-- name: GetNodeByPubKey :one
SELECT *
FROM nodes
WHERE pub_key = $1;

-- name: GetNodePubKeyByDBID :one
SELECT pub_key
FROM nodes
WHERE id = $1;

-- name: GetNodeIDByPubKey :one
SELECT id
FROM nodes
WHERE pub_key = $1;

-- name: GetNodeAliasByPubKey :one
SELECT alias
FROM nodes
WHERE pub_key = $1;

-- name: InsertNodeFeature :exec
INSERT INTO node_features (
    node_id, feature
) VALUES (
    $1, $2
);

-- name: InsertIPV4NodeAddress :exec
INSERT INTO node_addresses (
    node_id, address_type, address
) VALUES (
    $1, 0, $2
);

-- name: InsertIPV6NodeAddress :exec
INSERT INTO node_addresses (
    node_id, address_type, address
) VALUES (
   $1, 1, $2
);

-- name: InsertTorV3NodeAddress :exec
INSERT INTO node_addresses (
    node_id, address_type, address
) VALUES (
    $1, 2, $2
);

-- name: GetNodeFeatures :many
SELECT *
FROM node_features
WHERE node_id = $1;

-- name: DeleteNodeFeature :exec
DELETE FROM node_features
WHERE node_id = $1
AND feature = $2;

-- name: GetNodeAddresses :many
SELECT *
FROM node_addresses
WHERE node_id = $1;

-- name: DeleteNodeAddress :exec
DELETE FROM node_addresses
WHERE node_id = $1
  AND address = $2;

-- name: SetSourceNode :exec
INSERT INTO source_node (node_id)
VALUES ($1)
ON CONFLICT (node_id) DO NOTHING;

-- name: GetSourceNode :one
SELECT node_id
FROM source_node;