//go:build test_db_postgres || test_db_sqlite

package sqldb

var migrationAdditions = []MigrationConfig{
	{
		Name:          "000007_nodes",
		Version:       8,
		SchemaVersion: 7,
	},
}
