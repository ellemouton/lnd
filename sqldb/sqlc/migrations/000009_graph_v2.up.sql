-- The block height timestamp of this node's latest received node announcement.
-- It may be zero if we have not received a node announcement yet.
ALTER TABLE graph_nodes ADD COLUMN block_height BIGINT;

-- Support v2 node horizon queries by indexing the versioned block-height and
-- pubkey cursor fields together.
CREATE INDEX IF NOT EXISTS graph_nodes_version_block_height_pub_key_idx
    ON graph_nodes(version, block_height, pub_key);

-- The signature of the channel announcement. If this is null, then the channel
-- belongs to the source node and the channel has not been announced yet.
ALTER TABLE graph_channels ADD COLUMN signature BLOB;

-- For v2 channels onwards, we cant necessarily derive the funding pk script
-- from the other fields in the announcement, so we store it here so that
-- we have easy access to it when we want to subscribe to channel closures.
ALTER TABLE graph_channels ADD COLUMN funding_pk_script BLOB;

-- The optional merkel root hash advertised in the V2 channel announcement.
ALTER TABLE graph_channels ADD COLUMN merkle_root_hash BLOB;

-- The block height timestamp of this channel's latest received channel-update
-- message (for v2 channel update messages).
ALTER TABLE graph_channel_policies ADD COLUMN block_height BIGINT;

-- A bitfield describing the disabled flags for a v2 channel update.
ALTER TABLE graph_channel_policies ADD COLUMN disable_flags SMALLINT
    CHECK (disable_flags >= 0 AND disable_flags <= 255);

-- Support v2 channel horizon queries by indexing the versioned block-height
-- cursor fields on channel policies.
CREATE INDEX IF NOT EXISTS graph_channel_policy_version_block_height_channel_id_idx
    ON graph_channel_policies(version, block_height, channel_id);

-- Support version-aware channel horizon queries by indexing the v1 timestamp
-- cursor fields on channel policies.
CREATE INDEX IF NOT EXISTS graph_channel_policy_version_last_update_channel_id_idx
    ON graph_channel_policies(version, last_update, channel_id);
