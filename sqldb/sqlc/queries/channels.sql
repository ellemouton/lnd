-- name: InsertChannel :one
INSERT INTO channels (
    channel_id, outpoint, node_id_1, node_id_2, capacity, signature, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id;

-- name: AddChannelSignature :exec
UPDATE channels
SET signature = $2
WHERE id = $1;

-- name: GetChannel :one
SELECT *
FROM channels
WHERE id = $1;

-- name: GetChannelByChanID :one
SELECT *
FROM channels
WHERE channel_id = $1;

-- name: GetChannelByOutpoint :one
SELECT *
FROM channels
WHERE outpoint = $1;

-- name: ListNodeChannels :many
SELECT *
FROM channels
WHERE node_id_1 = $1
   OR node_id_2 = $1;

-- name: DeleteChannel :exec
DELETE FROM channels
WHERE channel_id = $1;

-- name: InsertChannelFeature :exec
INSERT INTO channel_features (
    channel_id, feature
) VALUES (
    $1, $2
);

-- name: GetChannelFeatures :many
SELECT *
FROM channel_features
WHERE channel_id = $1;

-- name: DeleteChannelFeature :exec
DELETE FROM channel_features
WHERE channel_id = $1
  AND feature = $2;

-- name: UpsertChannelExtraType :exec
INSERT INTO channel_extra_types (channel_id, type, value)
VALUES ($1, $2, $3)
ON CONFLICT (type, channel_id)
    DO UPDATE SET value = EXCLUDED.value;

-- name: GetExtraChannelTypes :many
SELECT *
FROM channel_extra_types
WHERE channel_id = $1;

-- name: DeleteExtraChannelType :exec
DELETE FROM channel_extra_types
WHERE channel_id = $1
  AND type = $2;

-- name: IsPublicNode :one
SELECT EXISTS (
    SELECT 1
    FROM channels
    WHERE (node_id_1 = $1 OR node_id_2 = $1)
      AND signature IS NOT NULL
) AS is_public;