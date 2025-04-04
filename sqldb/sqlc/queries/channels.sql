-- name: CreateChannel :one
INSERT INTO channels (
    version, scid, node_id_1, node_id_2,
    outpoint, capacity
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id;

-- name: GetChannelByID :one
SELECT * FROM channels
WHERE id = $1;

-- name: GetChannelBySCIDAndVersion :one
SELECT * FROM channels
WHERE scid = $1 AND version = $2;

-- name: GetSCIDByOutpointAndVersion :one
SELECT scid from channels
WHERE outpoint = $1 AND version = $2;

-- name: GetChannelByOutpointAndVersion :one
SELECT * FROM channels
WHERE outpoint = $1 AND version = $2;

-- name: GetChannelByOutpoint :one
SELECT * FROM channels
WHERE outpoint = $1;

-- name: ListAllChannelsByVersion :many
SELECT * FROM channels
WHERE version = $1;

-- name: ListChannelsByNodeIDAndVersion :many
SELECT * FROM channels
WHERE version = $1
  AND (node_id_1 = $2 OR node_id_2 = $2);

-- name: DeleteChannel :exec
DELETE FROM channels WHERE id = $1;

-- name: CreateChannelsV1Data :exec
INSERT INTO channels_v1_data (
    channel_id, bitcoin_key_1, bitcoin_key_2
) VALUES (
    $1, $2, $3
);

-- name: GetChannelsV1Data :one
SELECT * FROM channels_v1_data WHERE channel_id = $1;

-- name: UpdateChannelsV1Data :exec
UPDATE channels_v1_data
SET bitcoin_key_1 = $2, bitcoin_key_2 = $3
WHERE channel_id = $1;

-- name: CreateV1ChannelProof :exec
INSERT INTO v1_channel_proofs (
    channel_id, node_1_signature, node_2_signature,
    bitcoin_1_signature, bitcoin_2_signature
) VALUES (
    $1, $2, $3, $4, $5
);

-- name: GetV1ChannelProof :one
SELECT * FROM v1_channel_proofs WHERE channel_id = $1;

-- name: InsertChannelFeature :exec
INSERT INTO channel_features (
    channel_id, feature_id
) VALUES (
    $1, $2
);

-- name: GetChannelFeatures :many
SELECT
    nf.channel_id,
    nf.feature_id,
    f.bit
FROM channel_features nf
JOIN features f ON nf.feature_id = f.id
WHERE nf.channel_id = $1;

-- name: UpsertChannelExtraType :exec
INSERT INTO channel_extra_types (
    channel_id, type, value
)
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

-- name: HighestSCID :one
SELECT scid
FROM channels
WHERE version = $1
ORDER BY scid DESC
LIMIT 1;

-- name: UpsertZombieChannel :exec
INSERT INTO zombie_channels (scid, version, node_key_1, node_key_2)
VALUES ($1, $2, $3, $4)
ON CONFLICT (scid, version)
    DO UPDATE SET
    node_key_1 = COALESCE(EXCLUDED.node_key_1, zombie_channels.node_key_1),
    node_key_2 = COALESCE(EXCLUDED.node_key_2, zombie_channels.node_key_2);

-- name: DeleteZombieChannel :exec
DELETE FROM zombie_channels
WHERE scid = $1
AND version = $2;

-- name: CountZombieChannels :one
SELECT COUNT(*)
FROM zombie_channels
WHERE version = $1;

-- name: IsZombieChannel :one
SELECT EXISTS (
    SELECT 1
    FROM zombie_channels
    WHERE scid = $1
    AND version = $2
) AS is_zombie;

-- name: GetZombieChannel :one
SELECT *
FROM zombie_channels
WHERE scid = $1
AND version = $2;

-- name: GetUnconnectedNodes :many
SELECT n.id, n.pub_key
FROM nodes n
WHERE NOT EXISTS (
    SELECT 1
    FROM channels c
    WHERE c.node_id_1 = n.id OR c.node_id_2 = n.id AND c.version = $1
);

-- name: NodeHasV1ProofChannel :one
SELECT EXISTS (
    SELECT 1
    FROM channels c
             JOIN v1_channel_proofs v1p ON c.id = v1p.channel_id
    WHERE c.node_id_1 = $1 OR c.node_id_2 = $1
);

-- name: GetV1DisabledSCIDs :many
SELECT c.scid
FROM channels c
         JOIN channel_policies cp ON cp.channel_id = c.id
         JOIN channel_policy_v1_data v1 ON v1.channel_policy_id = cp.id
WHERE v1.disabled = true
GROUP BY c.scid
HAVING COUNT(*) > 1;

-- name: GetV1ChannelsByPolicyLastUpdateRange :many
SELECT DISTINCT c.*
FROM channels c
         JOIN channel_policies cp ON cp.channel_id = c.id
         JOIN channel_policy_v1_data v1 ON v1.channel_policy_id = cp.id
WHERE v1.last_update >= sqlc.arg(start_time)
  AND v1.last_update < sqlc.arg(end_time);

-- name: GetPublicV1ChannelsBySCID :many
SELECT c.*
FROM channels c
         JOIN v1_channel_proofs p ON p.channel_id = c.id
WHERE c.scid >= sqlc.arg(start_scid)
  AND c.scid < sqlc.arg(end_scid);

-- name: GetChannelsBySCIDRange :many
SELECT *
FROM channels
WHERE scid >= sqlc.arg(start_scid)
  AND scid < sqlc.arg(end_scid);

-- name: IsV1ChannelPublic :one
SELECT EXISTS (
    SELECT 1
    FROM v1_channel_proofs
    WHERE channel_id = $1
);

-- name: UpsertPruneLogEntry :exec
INSERT INTO prune_log (
    block_height, block_hash
) VALUES (
    $1, $2
)
ON CONFLICT(block_height) DO UPDATE SET
    block_hash = EXCLUDED.block_hash;

-- name: DeletePruneLogEntry :exec
DELETE FROM prune_log
WHERE block_height = $1;

-- name: GetPruneTip :one
SELECT block_height, block_hash
FROM prune_log
ORDER BY block_height DESC
LIMIT 1;

-- name: DeletePruneLogEntriesInRange :exec
DELETE FROM prune_log
WHERE block_height >= sqlc.arg(start_height)
  AND block_height <= sqlc.arg(end_height);

-- name: InsertClosedChannel :exec
INSERT INTO closed_scids (
    scid
) VALUES (
    $1
)
ON CONFLICT (scid) DO NOTHING;

-- name: IsClosedChannel :one
SELECT EXISTS (
    SELECT 1
    FROM closed_scids
    WHERE scid = $1
);
