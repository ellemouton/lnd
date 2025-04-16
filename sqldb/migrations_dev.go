//go:build test_db_postgres || test_db_sqlite

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
}
