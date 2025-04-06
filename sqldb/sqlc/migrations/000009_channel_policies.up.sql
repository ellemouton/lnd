CREATE TABLE IF NOT EXISTS channel_policies (
    id INTEGER PRIMARY KEY,

    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    timelock INTEGER NOT NULL,
    fee_ppm BIGINT NOT NULL,
    base_fee_msat BIGINT NOT NULL,
    min_htlc_msat BIGINT NOT NULL,

    -- The signature of the channel update announcement.
    signature BLOB
);
CREATE INDEX IF NOT EXISTS channel_policies_channel_id_idx ON channel_policies(channel_id);
CREATE UNIQUE INDEX IF NOT EXISTS channel_policies_unique ON channel_policies (
    channel_id, node_id
);

CREATE TABLE IF NOT EXISTS channel_policy_v1_data (
    channel_policy_id BIGINT PRIMARY KEY REFERENCES channel_policies(id) ON DELETE CASCADE,

    -- The unix timestamp of the last time the policy was updated.
    last_update BIGINT NOT NULL,

    disabled bool NOT NULL,

    max_htlc_msat BIGINT
);
CREATE INDEX IF NOT EXISTS channel_policies_v1_data_channel_policy_id_last_update_idx
    ON channel_policy_v1_data(channel_policy_id, last_update);
CREATE INDEX IF NOT EXISTS channel_policy_v1_data_channel_policy_id_disabled_idx
    ON channel_policy_v1_data(channel_policy_id, disabled);

-- channel_policy_extra_types stores any extra TLV fields covered by a channel
-- update that we do not have an explicit column for in the channel_policies
-- table.
CREATE TABLE IF NOT EXISTS channel_policy_extra_types (
    -- The channel_policy id this TLV field belongs to.
    channel_policy_id BIGINT NOT NULL REFERENCES channel_policies(id) ON DELETE CASCADE,

    -- The Type field.
    type BIGINT NOT NULL,

    -- The value field.
    value BLOB
);
CREATE INDEX IF NOT EXISTS channel_extra_types_channel_id_idx ON channel_policy_extra_types(channel_policy_id);
CREATE UNIQUE INDEX IF NOT EXISTS channel_policy_extra_types_unique ON channel_policy_extra_types (
    type, channel_policy_id
);

