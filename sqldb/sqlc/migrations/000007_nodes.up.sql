-- nodes stores all the nodes that we are aware of in the graph
-- that were gossiped via the V2 gossip protocol.
CREATE TABLE IF NOT EXISTS nodes (
    -- The db ID of the node. This will only be used DB level
    -- relations.
    id INTEGER PRIMARY KEY NOT NULL,

    -- The protocol version that this node was gossiped on.
    version SMALLINT NOT NULL,

    -- The public key (serialised compressed) of the node.
    pub_key BLOB NOT NULL UNIQUE,

    -- The alias of the node.
    alias VARCHAR,

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

-- Any info about the node that is specific to the V1 gossip protocol.
CREATE TABLE IF NOT EXISTS nodes_v1_data (
    -- The node id this V1 data belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE UNIQUE,

    -- The unix timestamp of the last time the node was updated.
    last_update BIGINT NOT NULL,

    -- The color of the node.
    color VARCHAR NOT NULL
);
CREATE INDEX IF NOT EXISTS nodes_v1_data_node_id_last_update_idx
    ON nodes_v1_data(node_id, last_update);

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
    id INTEGER PRIMARY KEY NOT NULL,

    -- The feature bit value.
    bit INTEGER NOT NULL UNIQUE
);

-- node_features contains the feature bits of a node.
CREATE TABLE IF NOT EXISTS node_features (
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The id of the feature that this node has.
    feature_id BIGINT NOT NULL REFERENCES features(id) ON DELETE CASCADE
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

CREATE TABLE IF NOT EXISTS  source_nodes(
    node_id BIGINT NOT NULL UNIQUE REFERENCES nodes (id) ON DELETE CASCADE
);