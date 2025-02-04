CREATE TABLE IF NOT EXISTS channel_policies (
    id INTEGER PRIMARY KEY NOT NULL,

    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    second_peer BOOLEAN NOT NULL,

    block_height INTEGER NOT NULL,
    disable_flags INTEGER NOT NULL,
    timelock INTEGER NOT NULL,
    fee_ppm BIGINT NOT NULL,
    base_fee_msat BIGINT NOT NULL,
    max_htlc_msat BIGINT NOT NULL,
    min_htlc_msat BIGINT NOT NULL,

    -- The signature of the channel update announcement.
    signature BLOB,

    -- The timestamp that this channel update record was created. This is
    -- metadata about the record and is not present in the protocol message.
    created_at TIMESTAMP NOT NULL,

    -- The last time this channel policy was updated. This is metadata about
    -- the record and is not present in the protocol message.
    updated_at TIMESTAMP NOT NULL,

    UNIQUE (channel_id, second_peer)
);

CREATE INDEX IF NOT EXISTS channel_policies_channel_id_idx ON channel_policies(channel_id);
CREATE INDEX IF NOT EXISTS channel_policies_block_height_idx ON channel_policies(block_height);
CREATE INDEX IF NOT EXISTS channel_policies_disable_flags_idx ON channel_policies(disable_flags);

-- channel_policy_extra_types stores any extra TLV fields covered by a channel
-- update that we do not have an explicit column for in the channel_policies
-- table.
CREATE TABLE IF NOT EXISTS channel_policy_extra_types (
    -- The channel_policy id this TLV field belongs to.
    channel_policy_id BIGINT NOT NULL REFERENCES channel_policies(id) ON DELETE CASCADE,

    -- The Type field.
    type BIGINT NOT NULL,

    -- The value field.
    value BLOB,

    -- Each channel update announcement can only have one entry per type.
    UNIQUE (type, channel_policy_id)
);
CREATE INDEX IF NOT EXISTS channel_extra_types_channel_id_idx ON channel_policy_extra_types(channel_policy_id);
