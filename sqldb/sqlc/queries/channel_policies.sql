-- name: InsertChannelPolicy :one
INSERT INTO channel_policies (
    channel_id,
    second_peer,
    block_height,
    disable_flags,
    timelock,
    fee_ppm,
    base_fee_msat,
    max_htlc_msat,
    min_htlc_msat,
    signature,
    created_at,
    updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING id;

-- name: UpdateChannelPolicy :exec
UPDATE channel_policies
SET
    block_height = $3,
    disable_flags = $4,
    timelock = $5,
    fee_ppm = $6,
    base_fee_msat = $7,
    max_htlc_msat = $8,
    min_htlc_msat = $9,
    signature = $10,
    updated_at = $11
WHERE id = $1 AND second_peer = $2;

-- name: GetChannelPolicy :one
SELECT *
FROM channel_policies
WHERE channel_id = $1 AND second_peer = $2;

-- name: UpsertChanPolicyExtraType :exec
INSERT INTO channel_policy_extra_types (channel_policy_id, type, value)
VALUES ($1, $2, $3)
ON CONFLICT (type, channel_policy_id)
    DO UPDATE SET value = EXCLUDED.value;

-- name: GetExtraChanPolicyTypes :many
SELECT *
FROM channel_policy_extra_types
WHERE channel_policy_id = $1;

-- name: DeleteExtraChanPolicyType :exec
DELETE FROM channel_policy_extra_types
WHERE channel_policy_id = $1
  AND type = $2;
