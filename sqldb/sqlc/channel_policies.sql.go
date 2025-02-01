// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0
// source: channel_policies.sql

package sqlc

import (
	"context"
	"time"
)

const deleteExtraChanPolicyType = `-- name: DeleteExtraChanPolicyType :exec
DELETE FROM channel_policy_extra_types
WHERE channel_policy_id = $1
  AND type = $2
`

type DeleteExtraChanPolicyTypeParams struct {
	ChannelPolicyID int64
	Type            int64
}

func (q *Queries) DeleteExtraChanPolicyType(ctx context.Context, arg DeleteExtraChanPolicyTypeParams) error {
	_, err := q.db.ExecContext(ctx, deleteExtraChanPolicyType, arg.ChannelPolicyID, arg.Type)
	return err
}

const getChannelPolicy = `-- name: GetChannelPolicy :one
SELECT id, channel_id, second_peer, block_height, disable_flags, timelock, fee_ppm, base_fee_msat, max_htlc_msat, min_htlc_msat, signature, created_at, updated_at
FROM channel_policies
WHERE channel_id = $1 AND second_peer = $2
`

type GetChannelPolicyParams struct {
	ChannelID  int64
	SecondPeer bool
}

func (q *Queries) GetChannelPolicy(ctx context.Context, arg GetChannelPolicyParams) (ChannelPolicy, error) {
	row := q.db.QueryRowContext(ctx, getChannelPolicy, arg.ChannelID, arg.SecondPeer)
	var i ChannelPolicy
	err := row.Scan(
		&i.ID,
		&i.ChannelID,
		&i.SecondPeer,
		&i.BlockHeight,
		&i.DisableFlags,
		&i.Timelock,
		&i.FeePpm,
		&i.BaseFeeMsat,
		&i.MaxHtlcMsat,
		&i.MinHtlcMsat,
		&i.Signature,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const getExtraChanPolicyTypes = `-- name: GetExtraChanPolicyTypes :many
SELECT channel_policy_id, type, value
FROM channel_policy_extra_types
WHERE channel_policy_id = $1
`

func (q *Queries) GetExtraChanPolicyTypes(ctx context.Context, channelPolicyID int64) ([]ChannelPolicyExtraType, error) {
	rows, err := q.db.QueryContext(ctx, getExtraChanPolicyTypes, channelPolicyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ChannelPolicyExtraType
	for rows.Next() {
		var i ChannelPolicyExtraType
		if err := rows.Scan(&i.ChannelPolicyID, &i.Type, &i.Value); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const insertChannelPolicy = `-- name: InsertChannelPolicy :one
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
) RETURNING id
`

type InsertChannelPolicyParams struct {
	ChannelID    int64
	SecondPeer   bool
	BlockHeight  int32
	DisableFlags int32
	Timelock     int32
	FeePpm       int64
	BaseFeeMsat  int64
	MaxHtlcMsat  int64
	MinHtlcMsat  int64
	Signature    []byte
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (q *Queries) InsertChannelPolicy(ctx context.Context, arg InsertChannelPolicyParams) (int64, error) {
	row := q.db.QueryRowContext(ctx, insertChannelPolicy,
		arg.ChannelID,
		arg.SecondPeer,
		arg.BlockHeight,
		arg.DisableFlags,
		arg.Timelock,
		arg.FeePpm,
		arg.BaseFeeMsat,
		arg.MaxHtlcMsat,
		arg.MinHtlcMsat,
		arg.Signature,
		arg.CreatedAt,
		arg.UpdatedAt,
	)
	var id int64
	err := row.Scan(&id)
	return id, err
}

const updateChannelPolicy = `-- name: UpdateChannelPolicy :exec
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
WHERE id = $1 AND second_peer = $2
`

type UpdateChannelPolicyParams struct {
	ID           int64
	SecondPeer   bool
	BlockHeight  int32
	DisableFlags int32
	Timelock     int32
	FeePpm       int64
	BaseFeeMsat  int64
	MaxHtlcMsat  int64
	MinHtlcMsat  int64
	Signature    []byte
	UpdatedAt    time.Time
}

func (q *Queries) UpdateChannelPolicy(ctx context.Context, arg UpdateChannelPolicyParams) error {
	_, err := q.db.ExecContext(ctx, updateChannelPolicy,
		arg.ID,
		arg.SecondPeer,
		arg.BlockHeight,
		arg.DisableFlags,
		arg.Timelock,
		arg.FeePpm,
		arg.BaseFeeMsat,
		arg.MaxHtlcMsat,
		arg.MinHtlcMsat,
		arg.Signature,
		arg.UpdatedAt,
	)
	return err
}

const upsertChanPolicyExtraType = `-- name: UpsertChanPolicyExtraType :exec
INSERT INTO channel_policy_extra_types (channel_policy_id, type, value)
VALUES ($1, $2, $3)
ON CONFLICT (type, channel_policy_id)
    DO UPDATE SET value = EXCLUDED.value
`

type UpsertChanPolicyExtraTypeParams struct {
	ChannelPolicyID int64
	Type            int64
	Value           []byte
}

func (q *Queries) UpsertChanPolicyExtraType(ctx context.Context, arg UpsertChanPolicyExtraTypeParams) error {
	_, err := q.db.ExecContext(ctx, upsertChanPolicyExtraType, arg.ChannelPolicyID, arg.Type, arg.Value)
	return err
}
