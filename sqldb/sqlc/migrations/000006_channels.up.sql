-- channels stores all teh channels that we are aware of in the graph.
CREATE TABLE IF NOT EXISTS channels (
    -- The db ID of the channel.
    id BIGINT PRIMARY KEY,

    channel_id BIGINT NOT NULL UNIQUE,

    outpoint TEXT NOT NULL UNIQUE,

    node_id_1 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    node_id_2 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    capacity BIGINT,

    serialised_announcement BYTEA
);

CREATE INDEX IF NOT EXISTS channels_node_id_1_idx ON channels(node_id_1);
CREATE INDEX IF NOT EXISTS channels_node_id_2_idx ON channels(node_id_2);

-- channel_features contains the feature bits of a channel.
CREATE TABLE IF NOT EXISTS channel_features (
    -- The channel id (DB ID) this feature belongs to.
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    -- The feature bit.
    feature INTEGER NOT NULL,

    -- The feature bit is unique per channe.
    UNIQUE (feature, channel_id)
);

CREATE INDEX IF NOT EXISTS channel_feature_channel_id_idx ON channel_features(channel_id);

CREATE TABLE IF NOT EXISTS source_channels (
    channel_id BIGINT PRIMARY KEY REFERENCES channels(id) ON DELETE CASCADE,
    announced BOOLEAN NOT NULL
);