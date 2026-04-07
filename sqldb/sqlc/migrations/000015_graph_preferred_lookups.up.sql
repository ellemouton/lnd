-- Preferred-node mapping: one row per unique pub_key pointing at the "best"
-- node row across gossip versions.  Priority: v2 announced > v1 announced >
-- v2 shell > v1 shell.
CREATE TABLE IF NOT EXISTS graph_preferred_nodes (
    pub_key  BLOB PRIMARY KEY,
    node_id  BIGINT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE
);

-- Preferred-channel mapping: one row per unique SCID pointing at the "best"
-- channel row across gossip versions.  Priority: v2 with policies >
-- v1 with policies > v2 bare > v1 bare.
CREATE TABLE IF NOT EXISTS graph_preferred_channels (
    scid       BLOB PRIMARY KEY,
    channel_id BIGINT NOT NULL REFERENCES graph_channels(id) ON DELETE CASCADE
);

-- Populate the preferred nodes table from existing data.
INSERT INTO graph_preferred_nodes (pub_key, node_id)
SELECT sub.pub_key, sub.node_id
FROM (
    SELECT
        n.pub_key,
        n.id AS node_id,
        ROW_NUMBER() OVER (
            PARTITION BY n.pub_key
            ORDER BY
                (COALESCE(length(n.signature), 0) > 0) DESC,
                n.version DESC
        ) AS rn
    FROM graph_nodes n
) sub
WHERE sub.rn = 1;

-- Populate the preferred channels table from existing data.
INSERT INTO graph_preferred_channels (scid, channel_id)
SELECT sub.scid, sub.channel_id
FROM (
    SELECT
        c.scid,
        c.id AS channel_id,
        ROW_NUMBER() OVER (
            PARTITION BY c.scid
            ORDER BY
                EXISTS (
                    SELECT 1 FROM graph_channel_policies p
                    WHERE p.channel_id = c.id
                      AND p.version = c.version
                ) DESC,
                c.version DESC
        ) AS rn
    FROM graph_channels c
) sub
WHERE sub.rn = 1;
