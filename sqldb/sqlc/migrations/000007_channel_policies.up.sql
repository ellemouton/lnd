CREATE TABLE IF NOT EXISTS channel_policies (
    id BIGINT PRIMARY KEY,

    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    block_height INTEGER NOT NULL,

    disable_flags INTEGER NOT NULL,

    second_peer BOOLEAN NOT NULL,

    timelock INTEGER NOT NULL,

    fee_ppm BIGINT NOT NULL,

    base_fee_msat BIGINT NOT NULL,

    max_htlc_msat BIGINT NOT NULL,

    min_htlc_msat BIGINT NOT NULL,

    serialised_announcement BYTEA,

    UNIQUE (channel_id, second_peer)
);

CREATE INDEX IF NOT EXISTS channel_policies_channel_id_idx ON channel_policies(channel_id);
CREATE INDEX IF NOT EXISTS channel_policies_block_height_idx ON channel_policies(block_height);
CREATE INDEX IF NOT EXISTS channel_policies_disabled_flags_idx ON channel_policies(disable_flags);

