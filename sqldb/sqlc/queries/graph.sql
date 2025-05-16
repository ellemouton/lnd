/* ─────────────────────────────────────────────
   nodes table queries
   ─────────────────────────────────────────────
*/

-- name: CreateNode :one
INSERT INTO nodes (
    version, pub_key, alias,
    last_update, color, signature
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: UpdateNode :exec
UPDATE nodes
SET alias = $2,
    signature = $3,
    last_update = $4,
    color = $5
WHERE id = $1;

-- name: GetNodeByID :one
SELECT *
FROM nodes
WHERE id = $1;

-- name: GetNodeByPubKeyAndVersion :one
SELECT *
FROM nodes
WHERE pub_key = $1
  AND version = $2;

-- name: GetV1NodesByLastUpdateRange :many
SELECT *
FROM nodes
WHERE version = 1
  AND last_update >= sqlc.arg(start_time)
  AND last_update < sqlc.arg(end_time);

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = $1;

-- name: GetNodeIDByPubKeyAndVersion :one
SELECT id
FROM nodes
WHERE pub_key = $1
  AND version = $2;

-- name: GetNodeAliasByPubKeyAndVersion :one
SELECT alias
FROM nodes
WHERE pub_key = $1 AND version = $2;

-- name: ListNodeIDsAndPubKeysByVersion :many
SELECT id, pub_key
FROM nodes
WHERE version = $1;

-- name: ListNodesByVersion :many
SELECT id, pub_key
FROM nodes
WHERE version = $1;

-- name: IsPublicV1Node :one
SELECT EXISTS (
    SELECT 1
    FROM channels c
         JOIN nodes n ON n.id = c.node_id_1 OR n.id = c.node_id_2
    WHERE c.version = 1
      AND c.bitcoin_1_signature IS NOT NULL
      AND n.pub_key = $1
);

-- name: GetUnconnectedNodes :many
SELECT n.id, n.pub_key
FROM nodes n
WHERE NOT EXISTS (
    SELECT 1
    FROM channels c
    WHERE c.node_id_1 = n.id OR c.node_id_2 = n.id
);

/* ─────────────────────────────────────────────
   node_features table queries
   ─────────────────────────────────────────────
*/

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

-- name: GetNodeFeaturesByPubKey :many
SELECT f.bit
FROM nodes n
    JOIN node_features nf ON nf.node_id = n.id
    JOIN features f ON nf.feature_id = f.id
WHERE n.pub_key = $1
  AND n.version = $2;

-- name: DeleteNodeFeature :exec
DELETE FROM node_features
WHERE node_id = $1
  AND feature_id = $2;

/* ─────────────────────────────────────────────
   node_addresses table queries
   ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
   node_extra_types table queries
   ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
   source_nodes table queries
   ─────────────────────────────────────────────
*/

-- name: AddSourceNode :exec
INSERT INTO source_nodes (node_id)
VALUES ($1)
ON CONFLICT (node_id) DO NOTHING;

-- name: GetSourceNodesByVersion :many
SELECT sn.node_id, n.pub_key
FROM source_nodes sn
   JOIN nodes n ON sn.node_id = n.id
WHERE n.version = $1;

-- name: GetSourceNodes :many
SELECT sn.node_id, n.pub_key, n.version
FROM source_nodes sn
         JOIN nodes n ON sn.node_id = n.id;

/* ─────────────────────────────────────────────
   features table queries
   ─────────────────────────────────────────────
 */

-- name: CreateFeature :one
INSERT INTO features (bit)
VALUES ($1)
ON CONFLICT (bit) DO UPDATE SET bit = EXCLUDED.bit
RETURNING id;

/* ─────────────────────────────────────────────
   channels table queries
   ─────────────────────────────────────────────
*/

-- name: CreateChannel :one
INSERT INTO channels (
    version, scid, node_id_1, node_id_2,
    outpoint, capacity, bitcoin_key_1, bitcoin_key_2,
    node_1_signature, node_2_signature, bitcoin_1_signature,
    bitcoin_2_signature
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING id;

-- name: AddV1ChannelProof :exec
UPDATE channels
SET node_1_signature = $2,
    node_2_signature = $3,
    bitcoin_1_signature = $4,
    bitcoin_2_signature = $5
WHERE id = $1;

-- name: GetChannelBySCIDAndVersion :one
SELECT * FROM channels
WHERE scid = $1 AND version = $2;

-- name: ListChannelsByNodeIDAndVersion :many
SELECT * FROM channels
WHERE version = $1
  AND (node_id_1 = $2 OR node_id_2 = $2);

-- name: ListAllChannelsByVersion :many
SELECT * FROM channels
WHERE version = $1;

-- name: HighestSCID :one
SELECT scid
FROM channels
WHERE version = $1
ORDER BY scid DESC
LIMIT 1;

-- name: GetChannelByOutpoint :one
SELECT * FROM channels
WHERE outpoint = $1;

-- name: GetPublicV1ChannelsBySCID :many
SELECT *
FROM channels
WHERE node_1_signature IS NOT NULL
  AND scid >= sqlc.arg(start_scid)
  AND scid < sqlc.arg(end_scid);

-- name: GetSCIDByOutpointAndVersion :one
SELECT scid from channels
WHERE outpoint = $1 AND version = $2;

-- name: GetChannelByOutpointAndVersion :one
SELECT * FROM channels
WHERE outpoint = $1 AND version = $2;

-- name: GetV1DisabledSCIDs :many
SELECT c.scid
FROM channels c
         JOIN channel_policies cp ON cp.channel_id = c.id
WHERE cp.disabled = true
GROUP BY c.scid
HAVING COUNT(*) > 1;

-- name: GetChannelsBySCIDRange :many
SELECT *
FROM channels
WHERE scid >= sqlc.arg(start_scid)
  AND scid < sqlc.arg(end_scid);

-- name: DeleteChannel :exec
DELETE FROM channels WHERE id = $1;

/* ─────────────────────────────────────────────
   channel_features table queries
   ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
   channel_extra_types table queries
   ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
   channel_policies table queries
   ─────────────────────────────────────────────
*/

-- name: CreateChannelPolicy :one
INSERT INTO channel_policies (
    version, channel_id, node_id, timelock, fee_ppm,
    base_fee_msat, min_htlc_msat, last_update, disabled,
    max_htlc_msat, signature
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING id;

-- name: UpdateChannelPolicy :exec
UPDATE channel_policies
SET timelock = $2,
    fee_ppm = $3,
    base_fee_msat = $4,
    min_htlc_msat = $5,
    last_update = $6,
    disabled = $7,
    max_htlc_msat = $8,
    signature = $9
WHERE id = $1;

-- name: GetChannelPolicyByChannelAndNode :one
SELECT *
FROM channel_policies
WHERE channel_id = $1
    AND node_id = $2
    AND version = $3;

-- name: GetChannelsByPolicyLastUpdateRange :many
SELECT DISTINCT c.*
FROM channels c
    JOIN channel_policies cp ON cp.channel_id = c.id
WHERE c.version=sqlc.arg(version)
    AND cp.last_update >= sqlc.arg(start_time)
    AND cp.last_update < sqlc.arg(end_time);


/* ─────────────────────────────────────────────
   channel_policy_extra_types table queries
   ─────────────────────────────────────────────
*/

-- name: AddChannelPolicyExtraType :exec
INSERT INTO channel_policy_extra_types (
    channel_policy_id, type, value
) VALUES (
    $1, $2, $3
)
ON CONFLICT (type, channel_policy_id)
    DO UPDATE SET value = EXCLUDED.value;

-- name: GetChannelPolicyExtraTypes :many
SELECT * FROM channel_policy_extra_types WHERE channel_policy_id = $1;

-- name: DeleteChannelPolicyExtraType :exec
DELETE FROM channel_policy_extra_types
WHERE channel_policy_id = $1 AND type = $2;

/* ─────────────────────────────────────────────
   zombie_channels table queries
   ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
    prune_log table queries
    ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
   closed_scid table queries
   ────────────────────────────────────────────-
*/

-- name: InsertClosedChannel :exec
INSERT INTO closed_scids (scid)
VALUES ($1)
ON CONFLICT (scid) DO NOTHING;

-- name: IsClosedChannel :one
SELECT EXISTS (
    SELECT 1
    FROM closed_scids
    WHERE scid = $1
);