-- name: CreateChannelPolicy :one
INSERT INTO channel_policies (
    channel_id, node_id, timelock, fee_ppm, base_fee_msat, min_htlc_msat, signature
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING id;

-- name: GetV1ChannelPolicyByChannelAndNode :one
SELECT
    cp.*,
    v1.*
FROM channel_policies cp
         JOIN channel_policy_v1_data v1 ON cp.id = v1.channel_policy_id
WHERE channel_id = $1 AND node_id = $2;

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

-- name: DeleteAllChannelPolicyExtraTypes :exec
DELETE FROM channel_policy_extra_types WHERE channel_policy_id = $1;

-- name: CreateChannelPolicyV1Data :exec
INSERT INTO channel_policy_v1_data (
    channel_policy_id, last_update, disabled, max_htlc_msat
) VALUES (
             $1, $2, $3, $4
         );

-- name: GetChannelPolicyV1Data :one
SELECT * FROM channel_policy_v1_data WHERE channel_policy_id = $1;

-- name: UpdateChannelPolicyV1Data :exec
UPDATE channel_policy_v1_data
SET last_update = $2,
    disabled = $3,
    max_htlc_msat = $4
WHERE channel_policy_id = $1;
