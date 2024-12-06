-- name: InsertChannelUpdate :one
INSERT INTO channel_policies (
    channel_id,
    block_height,
    disable_flags,
    second_peer,
    timelock,
    fee_ppm,
    base_fee_msat,
    max_htlc_msat,
    min_htlc_msat,
    serialised_announcement
) VALUES (
    $1,
 $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10
) RETURNING id;

-- name: UpdateChannelPolicy :exec
UPDATE channel_policies
SET
    block_height = $2,
    disable_flags = $3,
    timelock = $4,
    fee_ppm = $5,
    base_fee_msat = $6,
    max_htlc_msat = $7,
    min_htlc_msat = $8,
    serialised_announcement = $9
WHERE id = $1;

-- name: GetChannelPolicy :one
SELECT cp.*
FROM channel_policies cp
JOIN channels c ON cp.channel_id = c.id
WHERE c.channel_id = $1 AND cp.second_peer = $2;

