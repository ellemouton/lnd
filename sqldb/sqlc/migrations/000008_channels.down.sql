-- Drop indexes first (safely, if they exist)
DROP INDEX IF EXISTS channel_extra_types_unique;
DROP INDEX IF EXISTS channel_extra_types_channel_id_idx;

DROP INDEX IF EXISTS channel_features_unique;
DROP INDEX IF EXISTS channel_feature_node_id_idx;

DROP INDEX IF EXISTS v1_channel_proofs_channel_id_idx;

DROP INDEX IF EXISTS channels_v1_data_node_id_idx;

DROP INDEX IF EXISTS channels_version_channel_id_idx;
DROP INDEX IF EXISTS channels_node_id_1_idx;
DROP INDEX IF EXISTS channels_node_id_2_idx;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS channel_extra_types;
DROP TABLE IF EXISTS channel_features;
DROP TABLE IF EXISTS v1_channel_proofs;
DROP TABLE IF EXISTS channels_v1_data;
DROP TABLE IF EXISTS channels;