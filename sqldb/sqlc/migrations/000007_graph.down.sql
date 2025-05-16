DROP INDEX IF EXISTS node_addresses_node_id_idx;
DROP INDEX IF EXISTS node_feature_node_id_idx;
DROP INDEX IF EXISTS node_extra_types_node_id_idx;
DROP INDEX IF EXISTS nodes_unique;
DROP INDEX IF EXISTS node_features_unique;
DROP INDEX IF EXISTS features_bit_unique;

DROP TABLE IF EXISTS features;
DROP TABLE IF EXISTS node_addresses;
DROP TABLE IF EXISTS node_features;
DROP TABLE IF EXISTS node_extra_types;
DROP TABLE IF EXISTS source_nodes;
DROP TABLE IF EXISTS nodes;

-- Drop indexes first (safely, if they exist)
DROP INDEX IF EXISTS channel_extra_types_unique;
DROP INDEX IF EXISTS channel_extra_types_channel_id_idx;

DROP INDEX IF EXISTS channel_features_unique;
DROP INDEX IF EXISTS channel_feature_node_id_idx;

DROP INDEX IF EXISTS channels_v1_data_node_id_idx;

DROP INDEX IF EXISTS channels_version_channel_id_idx;
DROP INDEX IF EXISTS channels_node_id_1_idx;
DROP INDEX IF EXISTS channels_node_id_2_idx;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS channel_extra_types;
DROP TABLE IF EXISTS channel_features;
DROP TABLE IF EXISTS channels_v1_data;
DROP TABLE IF EXISTS channels;
