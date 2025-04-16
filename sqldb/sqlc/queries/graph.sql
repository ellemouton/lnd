/* ─────────────────────────────────────────────
   nodes table queries
   ─────────────────────────────────────────────
*/

-- name: CreateNode :one
INSERT INTO nodes (
    version, pub_key, alias, signature
)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: UpdateNode :exec
UPDATE nodes
SET alias = $2,
    signature = $3
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

/* ─────────────────────────────────────────────
   nodes_v1_data table queries
   ─────────────────────────────────────────────
*/

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
    outpoint, capacity
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING id;

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

/* ─────────────────────────────────────────────
   channels_v1_data table queries
   ─────────────────────────────────────────────
*/

-- name: CreateChannelsV1Data :exec
INSERT INTO channels_v1_data (
    channel_id, bitcoin_key_1, bitcoin_key_2
) VALUES (
    $1, $2, $3
);

-- name: GetChannelsV1Data :one
SELECT * FROM channels_v1_data WHERE channel_id = $1;

/* ─────────────────────────────────────────────
   channels_v1_channel_proofs table queries
   ─────────────────────────────────────────────
*/

-- name: CreateV1ChannelProof :exec
INSERT INTO v1_channel_proofs (
    channel_id, node_1_signature, node_2_signature,
    bitcoin_1_signature, bitcoin_2_signature
) VALUES (
     $1, $2, $3, $4, $5
);

-- name: GetV1ChannelProof :one
SELECT * FROM v1_channel_proofs WHERE channel_id = $1;

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
    channel_id, node_id, timelock, fee_ppm, base_fee_msat, min_htlc_msat, signature
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING id;

-- name: GetChannelPolicyByChannelAndNode :one
SELECT * FROM channel_policies
WHERE channel_id = $1 AND node_id = $2;

-- name: UpdateChannelPolicy :exec
UPDATE channel_policies
SET timelock = $2,
    fee_ppm = $3,
    base_fee_msat = $4,
    min_htlc_msat = $5,
    signature = $6
WHERE id = $1;

/* ─────────────────────────────────────────────
   channel_policy_v1_data table queries
   ─────────────────────────────────────────────
*/

-- name: CreateChannelPolicyV1Data :exec
INSERT INTO channel_policy_v1_data (
    channel_policy_id, last_update, disabled, max_htlc_msat
) VALUES (
    $1, $2, $3, $4
);

-- name: GetV1ChannelPolicyByChannelAndNode :one
SELECT
    cp.*,
    v1.*
FROM channel_policies cp
JOIN channel_policy_v1_data v1 ON cp.id = v1.channel_policy_id
WHERE channel_id = $1 AND node_id = $2;

-- name: GetChannelPolicyV1Data :one
SELECT * FROM channel_policy_v1_data WHERE channel_policy_id = $1;

-- name: UpdateChannelPolicyV1Data :exec
UPDATE channel_policy_v1_data
SET last_update = $2,
    disabled = $3,
    max_htlc_msat = $4
WHERE channel_policy_id = $1;

-- name: AddChannelPolicyExtraType :exec
INSERT INTO channel_policy_extra_types (
    channel_policy_id, type, value
) VALUES (
   $1, $2, $3
)
ON CONFLICT (type, channel_policy_id)
    DO UPDATE SET value = EXCLUDED.value;

/* ─────────────────────────────────────────────
   channel_policy_extra_types table queries
   ─────────────────────────────────────────────
*/

-- name: GetChannelPolicyExtraTypes :many
SELECT * FROM channel_policy_extra_types WHERE channel_policy_id = $1;

-- name: DeleteChannelPolicyExtraType :exec
DELETE FROM channel_policy_extra_types
WHERE channel_policy_id = $1 AND type = $2;

