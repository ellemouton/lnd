-- name: InsertChannel :one
INSERT INTO channels (
    channel_id, outpoint, node_id_1, node_id_2, capacity, serialised_announcement
) VALUES (
     $1, $2, $3, $4, $5, $6
) RETURNING id;

-- name: UpdateChannel :exec
UPDATE channels
SET serialised_announcement = $2
WHERE id = $1;

-- name: GetChanDBIDBYChanID :one
SELECT id
FROM channels
WHERE channel_id = $1;

-- name: InsertSourceChannel :exec
INSERT INTO source_channels (
    channel_id, announced
) VALUES (
    $1, $2
) ON CONFLICT (channel_id)
DO UPDATE SET announced=$2;

-- name: SetSourceChannelAnnounced :execresult
UPDATE source_channels
SET announced = $2
WHERE channel_id = $1;

-- name: GetChannel :one
SELECT
    c.id,
    c.channel_id,
    c.outpoint,
    c.node_id_1,
    c.node_id_2,
    c.capacity,
    c.serialised_announcement,
    COALESCE(sc.announced, true) AS announced
FROM
    channels c
        LEFT JOIN
    source_channels sc ON c.id = sc.channel_id
WHERE
    c.id = $1;

-- name: GetChanDBIDByChanID :one
SELECT id
FROM channels
WHERE channel_id = $1;

-- name: GetChanDBIDByOutpoint :one
SELECT id
FROM channels
WHERE outpoint = $1;

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

-- name: DeleteChannelByChanID :exec
DELETE FROM channels
WHERE channel_id = $1;
