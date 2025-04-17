//go:build test_db_postgres || test_db_sqlite || test_native_sql

package sqldb

var migrationAdditions = []MigrationConfig{
	{
		Name:          "000007_nodes",
		Version:       8,
		SchemaVersion: 7,
	},
	{
		Name:          "000009_channels",
		Version:       9,
		SchemaVersion: 8,
	},
	{
		Name:          "000010_channel_policies",
		Version:       10,
		SchemaVersion: 9,
	},
	{
		Name:          "kv_graph_migration",
		Version:       11,
		SchemaVersion: 9,
		// A migration function may be attached to this
		// migration to migrate KV graph to the native SQL
		// schema. This is optional and can be disabled by the
		// user if necessary.
	},
}
