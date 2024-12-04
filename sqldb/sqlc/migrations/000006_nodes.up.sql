-- nodes stores all the nodes that we are aware of in the graph.
CREATE TABLE IF NOT EXISTS nodes (
    -- The db ID of the node.
    id BIGINT PRIMARY KEY,

    -- The public key (serialised compressed) of the node.
    pub_key BYTEA NOT NULL UNIQUE,

    -- The block height timestamp of this node's latest received node announcement.
    -- It will only be null if an announcement for this node has not yet been
    -- received which will be the case if we receive a channel announcement for this
    -- node before we receive a node announcement.
    block_height INTEGER,

    -- The alias of the node.
    alias VARCHAR,

    serialised_announcement BYTEA
);

-- node_features contains the feature bits of a node.
CREATE TABLE IF NOT EXISTS node_features (
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    -- The feature bit.
    feature INTEGER NOT NULL,

    -- The feature bit is unique per node.
    UNIQUE (feature, node_id)
);

CREATE TABLE IF NOT EXISTS node_address_types(
    id INTEGER PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO node_address_types (id, description)
VALUES
    (0, 'ipv_4'),
    (1, 'ipv_6'),
    (2, 'tor_v3');


CREATE TABLE IF NOT EXISTS node_addresses(
    -- The node id this feature belongs to.
    node_id BIGINT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,

    address_type INTEGER NOT NULL REFERENCES node_address_types(id) ON DELETE CASCADE,

    address TEXT NOT NULL
);