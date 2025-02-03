-- nodes stores all the nodes that we are aware of in the graph
-- that were gossiped via the V2 gossip protocol.
CREATE TABLE IF NOT EXISTS nodes (
    -- The db ID of the node. This will only be used DB level
    -- relations.
    id INTEGER PRIMARY KEY NOT NULL,

    -- The public key (serialised compressed) of the node.
    pub_key BYTEA NOT NULL UNIQUE,

    -- The alias of the node.
    alias VARCHAR,

    -- The block height timestamp of this node's latest received node
    -- announcement. It may be zero if we have not received a node
    -- announcement yet.
    block_height BIGINT NOT NULL,

    -- The signature of the node announcement. If this is null, then
    -- the node announcement has not been received yet and this record
    -- is a shell node. This can be the case if we receive a channel
    -- announcement for a channel that is connected to a node that we
    -- have not yet received a node announcement for.
    signature BLOB,

    -- The timestamp that this node record was created. This is metadata
    -- about the record and is not present in the protocol message.
    created_at TIMESTAMP NOT NULL,

    -- The timestamp that this node record was last updated. This is
    -- metadata about the record and is not present in the protocol message.
    updated_at TIMESTAMP NOT NULL
);

-- node_extra_types stores any extra TLV fields covered by a node announcement that
-- we do not have an explicit column for in the nodes table.
CREATE TABLE IF NOT EXISTS node_extra_types (
    -- The node id this TLV field belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The Type field.
    type BIGINT NOT NULL,

    -- The value field.
    value BLOB,

    -- Each node can only have one entry per type.
    UNIQUE (type, node_id)
);
CREATE INDEX IF NOT EXISTS node_extra_types_node_id_idx ON node_extra_types(node_id);

-- node_features contains the feature bits of a node.
CREATE TABLE IF NOT EXISTS node_features (
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The feature bit.
    feature INTEGER NOT NULL,

    -- The feature bit is unique per node.
    UNIQUE (feature, node_id)
);
CREATE INDEX IF NOT EXISTS node_feature_node_id_idx ON node_features(node_id);

-- node_addresses contains the advertised addresses of nodes.
CREATE TABLE IF NOT EXISTS node_addresses(
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- An enum that represents the type of address. This will
    -- dictate how the address column should be parsed.
    type SMALLINT NOT NULL,

    -- The advertised address of the node.
    address TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS node_addresses_node_id_idx ON node_addresses(node_id);

CREATE TABLE source_node(
    node_id BIGINT PRIMARY KEY REFERENCES nodes (id)
);

