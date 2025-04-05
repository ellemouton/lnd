-- channels stores all teh channels that we are aware of in the graph.
CREATE TABLE IF NOT EXISTS channels (
    -- The db ID of the channel.
    id INTEGER PRIMARY KEY NOT NULL,

    -- The protocol version that this node was gossiped on.
    version SMALLINT NOT NULL,

    -- The channel id (short channel id) of the channel.
    channel_id BLOB NOT NULL,

    node_id_1 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    node_id_2 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The outpoint of the funding transaction.
    outpoint TEXT NOT NULL,

    capacity BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS channels_node_id_1_idx ON channels(node_id_1);
CREATE INDEX IF NOT EXISTS channels_node_id_2_idx ON channels(node_id_2);
CREATE INDEX IF NOT EXISTS channels_version_channel_id_idx ON channels(version, channel_id DESC);
CREATE INDEX IF NOT EXISTS channels_outpoint_idx ON channels(outpoint);

-- Any info about the channel that is specific to the V1 gossip protocol.
CREATE TABLE IF NOT EXISTS channels_v1_data (
    -- The channel id this V1 data belongs to.
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    bitcoin_key_1 BLOB NOT NULL,

    bitcoin_key_2 BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS channels_v1_data_node_id_idx ON channels_v1_data(channel_id);

CREATE TABLE IF NOT EXISTS v1_channel_proofs (
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    node_1_signature BLOB NOT NULL,
    node_2_signature BLOB NOT NULL,
    bitcoin_1_signature BLOB NOT NULL,
    bitcoin_2_signature BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS v1_channel_proofs_channel_id_idx ON v1_channel_proofs(channel_id);

-- channel_features contains the feature bits of a channel.
CREATE TABLE IF NOT EXISTS channel_features (
    -- The channel id this feature belongs to.
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    -- The id of the feature that this node has.
    feature_id BIGINT NOT NULL REFERENCES features(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS channel_feature_node_id_idx ON channel_features(channel_id);
CREATE UNIQUE INDEX IF NOT EXISTS channel_features_unique ON channel_features (
    channel_id, feature_id
);

-- channel_extra_types stores any extra TLV fields covered by a channels
-- announcement that we do not have an explicit column for in the channels
-- table.
CREATE TABLE IF NOT EXISTS channel_extra_types (
    -- The channel id this TLV field belongs to.
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    -- The Type field.
    type BIGINT NOT NULL,

    -- The value field.
    value BLOB
);
CREATE INDEX IF NOT EXISTS channel_extra_types_channel_id_idx ON channel_extra_types(channel_id);
CREATE UNIQUE INDEX IF NOT EXISTS channel_extra_types_unique ON channel_extra_types (
    type, channel_id
);

CREATE TABLE IF NOT EXISTS zombie_channels (
    -- The channel id (short channel id) of the channel.
    channel_id BIGINT NOT NULL,

    -- The protocol version that this node was gossiped on.
    version SMALLINT NOT NULL,

    node_key_1 BYTEA,
    node_key_2 BYTEA
);
CREATE UNIQUE INDEX IF NOT EXISTS zombie_channels_channel_id_version_idx
    ON zombie_channels(channel_id, version);


CREATE TABLE IF NOT EXISTS closed_scids (
    -- The short channel id of the channel.
    channel_id BLOB NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS prune_log (
    -- The block height that the prune was performed at.
    block_height BIGINT PRIMARY KEY NOT NULL,

    -- The block hash that the prune was performed at.
    block_hash BYTEA NOT NULL
);
