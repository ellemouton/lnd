package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	defaultMaxLineLen       = 80
	defaultTabWidthInSpaces = 8
)

// TestGetLLLIssuesForFile tests the line-too-long linter.
//
//nolint:lll
func TestGetLLLIssuesForFile(t *testing.T) {
	// Test data
	testCases := []struct {
		name          string
		content       string
		expectedIssue string
	}{
		{
			name: "Single long line",
			content: `
				fmt.Println("This is a very long line that exceeds the maximum length and should be flagged by the linter.")`,
			expectedIssue: "the line is 140 characters long, which " +
				"exceeds the maximum of 80 characters.",
		},
		{
			name: "Short lines",
			content: `
				fmt.Println("Short line")`,
		},
		{
			name: "Directive ignored",
			content: ` 
				//go:generate something	very very very very very very very very very long and complex here wowowow`,
		},
		{
			name:    "Long single line import",
			content: `import "github.com/lightningnetwork/lnd/lnrpc/walletrpc/more/more/more/more/more/more/ok/that/is/enough"`,
		},
		{
			// NOTE: the test case here is indented by 1 tab so that
			// we can work around `gosimports` from removing the
			// "unused" imports.
			name: "Multi-line import",
			content: `
			import (
				"os"
				"fmt"

				"github.com/lightningnetwork/lnd/lnrpc/walletrpc/more/ok/that/is/enough"
			)`,
		},
		{
			name: "Long single line log",
			content: `
		log.Infof("This is a very long log line but since it is a log line, it should be skipped by the linter.")
		rpcLog.Info("Another long log line with a slightly different name and should still be skipped")`,
		},
		{
			name: "Long single line log followed by a non-log line",
			content: `
				log.Infof("This is a very long log line but since it is a log line, it should be skipped by the linter.")
				fmt.Println("This is a very long line that exceeds the maximum length and should be flagged by the linter.")`,
			expectedIssue: "the line is 124 characters long, which exceeds the maximum of 80 characters.",
		},
		{
			name: "Multi-line log",
			content: `
				log.Infof("This is a very long log line but 
						since it is a log line, it 
						should be skipped by the linter.")`,
		},
		{
			name: "Multi-line log followed by a non-log line",
			content: `
				log.Infof("This is a very long log line but 
						since it is a log line, it 
						should be skipped by the linter.")
				fmt.Println("This is a very long line that 
						exceeds the maximum length and 
						should be flagged by the linter.")`,
			expectedIssue: "the line is 82 characters long, which " +
				"exceeds the maximum of 80 characters.",
		},
	}

	tabSpaces := strings.Repeat(" ", defaultTabWidthInSpaces)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Write content to a temporary file.
			tmpFile := t.TempDir() + "/test.go"
			err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
			require.NoError(t, err)

			// Run the linter on the file.
			issues, err := getLLLIssuesForFile(
				tmpFile, defaultMaxLineLen, tabSpaces,
			)
			require.NoError(t, err)

			if tc.expectedIssue == "" {
				require.Len(t, issues, 0)

				return
			}

			// All our tests currently return a maximum of one
			// issue.
			require.Len(t, issues, 1)

			// Assert the issues match expectations
			require.Equal(t, tc.expectedIssue, issues[0].Text)
		})
	}
}
