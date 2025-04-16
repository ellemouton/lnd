DROP INDEX IF EXISTS node_addresses_node_id_idx;
DROP INDEX IF EXISTS node_feature_node_id_idx;
DROP INDEX IF EXISTS node_extra_types_node_id_idx;
DROP INDEX IF EXISTS nodes_unique;
DROP INDEX IF EXISTS nodes_v1_data_node_id_idx;
DROP INDEX IF EXISTS node_features_unique;
DROP INDEX IF EXISTS features_bit_unique;

DROP TABLE IF EXISTS features;
DROP TABLE IF EXISTS node_addresses;
DROP TABLE IF EXISTS node_features;
DROP TABLE IF EXISTS node_extra_types;
DROP TABLE IF EXISTS source_nodes;
DROP TABLE IF EXISTS nodes_v1_data;
DROP TABLE IF EXISTS nodes;