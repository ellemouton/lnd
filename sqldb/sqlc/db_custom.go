package sqlc

import "fmt"

// makeQueryParams generates a string of query parameters for a SQL query. It is
// meant to replace the `?` placeholders in a SQL query with numbered parameters
// like `$1`, `$2`, etc. This is required for the sqlc /*SLICE:<field_name>*/
// workaround. See scripts/gen_sqlc_docker.sh for more details.
func makeQueryParams(numTotalArgs, numListArgs int) string {
	diff := numTotalArgs - numListArgs
	result := ""
	for i := diff + 1; i <= numTotalArgs; i++ {
		if i == numTotalArgs {
			result += fmt.Sprintf("$%d", i)

			continue
		}
		result += fmt.Sprintf("$%d,", i)
	}
	return result
}
