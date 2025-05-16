/* ─────────────────────────────────────────────
   node data tables
   ─────────────────────────────────────────────
*/

-- nodes stores all the nodes that we are aware of in the LN graph.
CREATE TABLE IF NOT EXISTS nodes (
    -- The db ID of the node. This will only be used DB level
    -- relations.
    id INTEGER PRIMARY KEY,

    -- The protocol version that this node was gossiped on.
    version SMALLINT NOT NULL,

    -- The public key (serialised compressed) of the node.
    pub_key BLOB NOT NULL,

    -- The alias of the node.
    alias VARCHAR,

    -- The unix timestamp of the last time the node was updated.
    -- This is nullable since it is only set if the node_announcement
    -- message has been received on the v1 protocol version.
    last_update BIGINT,

    -- The color of the node.
    color VARCHAR,

    -- The signature of the node announcement. If this is null, then
    -- the node announcement has not been received yet and this record
    -- is a shell node. This can be the case if we receive a channel
    -- announcement for a channel that is connected to a node that we
    -- have not yet received a node announcement for. If the version is
    -- 1, then this an ECDSA signature. If the version is 2, then this
    -- is a BIP340 signature.
    signature BLOB
);
CREATE INDEX IF NOT EXISTS nodes_version_idx ON nodes(version);

-- A node can only have one active node announcement per protocol.
CREATE UNIQUE INDEX IF NOT EXISTS nodes_unique ON nodes (
    pub_key, version
);

-- node_extra_types stores any extra TLV fields covered by a node announcement that
-- we do not have an explicit column for in the nodes table.
CREATE TABLE IF NOT EXISTS node_extra_types (
    -- The node id this TLV field belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The Type field.
    type BIGINT NOT NULL,

    -- The value field.
    value BLOB
);
CREATE INDEX IF NOT EXISTS node_extra_types_node_id_idx ON node_extra_types(node_id);
CREATE UNIQUE INDEX IF NOT EXISTS node_extra_types_unique ON node_extra_types (
    type, node_id
);

CREATE TABLE IF NOT EXISTS features (
    -- The DB ID of the feature.
    id INTEGER PRIMARY KEY,

    -- The feature bit value.
    bit INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS features_bit_unique ON features (bit);

-- node_features contains the feature bits of a node.
CREATE TABLE IF NOT EXISTS node_features (
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The id of the feature that this node has.
    feature_id BIGINT NOT NULL REFERENCES features(id)
);
CREATE INDEX IF NOT EXISTS node_feature_node_id_idx ON node_features(node_id);
CREATE UNIQUE INDEX IF NOT EXISTS node_features_unique ON node_features (
    node_id, feature_id
);

-- node_addresses contains the advertised addresses of nodes.
CREATE TABLE IF NOT EXISTS node_addresses (
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- An enum that represents the type of address. This will
    -- dictate how the address column should be parsed.
    type SMALLINT NOT NULL,

    position INTEGER NOT NULL, -- ordering within (node_id, type)

    -- The advertised address of the node.
    address TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS node_addresses_unique ON node_addresses (
    node_id, type, position
);
CREATE INDEX IF NOT EXISTS node_addresses_node_id_idx ON node_addresses(node_id);

CREATE TABLE IF NOT EXISTS source_nodes (
   node_id BIGINT PRIMARY KEY REFERENCES nodes (id) ON DELETE CASCADE
);

/* ─────────────────────────────────────────────
   channel data tables
   ─────────────────────────────────────────────
*/

-- channels stores all teh channels that we are aware of in the graph.
CREATE TABLE IF NOT EXISTS channels (
   -- The db ID of the channel.
   id INTEGER PRIMARY KEY,

   -- The protocol version that this node was gossiped on.
   version SMALLINT NOT NULL,

   -- The channel id (short channel id) of the channel.
   scid BLOB NOT NULL,

   node_id_1 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
   node_id_2 BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

   -- The outpoint of the funding transaction.
   outpoint TEXT NOT NULL,

   capacity BIGINT NOT NULL,

   bitcoin_key_1 BLOB,
   bitcoin_key_2 BLOB,
   node_1_signature BLOB,
   node_2_signature BLOB,
   bitcoin_1_signature BLOB,
   bitcoin_2_signature BLOB
);
CREATE INDEX IF NOT EXISTS channels_node_id_1_idx ON channels(node_id_1);
CREATE INDEX IF NOT EXISTS channels_node_id_2_idx ON channels(node_id_2);
CREATE UNIQUE INDEX IF NOT EXISTS channels_unique ON channels(version, scid DESC);
CREATE INDEX IF NOT EXISTS channels_version_outpoint_idx ON channels(version, outpoint);

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

/* ─────────────────────────────────────────────
   channel policy data tables
   ─────────────────────────────────────────────
*/

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

/* ─────────────────────────────────────────────
   Other graph related tables
   ─────────────────────────────────────────────
*/

CREATE TABLE IF NOT EXISTS zombie_channels (
    -- The channel id (short channel id) of the channel.
    scid BIGINT NOT NULL,

    -- The protocol version that this node was gossiped on.
    version SMALLINT NOT NULL,

    node_key_1 BYTEA,
    node_key_2 BYTEA
);

CREATE UNIQUE INDEX IF NOT EXISTS zombie_channels_channel_id_version_idx
    ON zombie_channels(scid, version);

CREATE TABLE IF NOT EXISTS prune_log (
    -- The block height that the prune was performed at.
    block_height BIGINT PRIMARY KEY,

    -- The block hash that the prune was performed at.
    block_hash BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS closed_scids (
    -- The short channel id of the channel.
    scid BLOB PRIMARY KEY
);