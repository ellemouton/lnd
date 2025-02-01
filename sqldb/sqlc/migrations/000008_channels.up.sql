-- channels stores all teh channels that we are aware of in the graph.
CREATE TABLE IF NOT EXISTS channels (
    -- The db ID of the channel.
    id INTEGER PRIMARY KEY NOT NULL,

    -- The channel id (short channel id) of the channel.
    channel_id BIGINT NOT NULL UNIQUE,

    -- The outpoint of the funding transaction.
    outpoint TEXT NOT NULL UNIQUE,

    node_id_1 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    node_id_2 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    capacity BIGINT NOT NULL,

    -- The signature of the channel announcement. If this is null, then the
    -- channel belongs to the source node and the channel has not been
    -- announced yet.
    signature BLOB,

    -- The timestamp that this channel record was created. This is metadata
    -- about the record and is not present in the protocol message.
    created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS channels_node_id_1_idx ON channels(node_id_1);
CREATE INDEX IF NOT EXISTS channels_node_id_2_idx ON channels(node_id_2);

-- channel_features contains the feature bits of a channel.
CREATE TABLE IF NOT EXISTS channel_features (
    -- The channel id (DB ID) this feature belongs to.
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    -- The feature bit.
    feature INTEGER NOT NULL,

    -- The feature bit is unique per channel.
    UNIQUE (feature, channel_id)
);
CREATE INDEX IF NOT EXISTS channel_feature_channel_id_idx ON channel_features(channel_id);

-- channel_extra_types stores any extra TLV fields covered by a channels
-- announcement that we do not have an explicit column for in the channels
-- table.
CREATE TABLE IF NOT EXISTS channel_extra_types (
    -- The channel id this TLV field belongs to.
    channel_id BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,

    -- The Type field.
    type BIGINT NOT NULL,

    -- The value field.
    value BYTEA NOT NULL,

    -- Each channel announcement can only have one entry per type.
    UNIQUE (type, channel_id)
);
CREATE INDEX IF NOT EXISTS channel_extra_types_channel_id_idx ON channel_extra_types(channel_id);