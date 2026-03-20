package lnwire

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNodeAliasValidation tests that the NewNodeAlias method will only accept
// valid node announcements.
func TestNodeAliasValidation(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		alias string
		valid bool
	}{
		// UTF-8 alias with valid length.
		{
			alias: "meruem",
			valid: true,
		},

		// UTF-8 alias with invalid length.
		{
			alias: "p3kysxqr23swl33m6h5grmzddgw5nsgkky3g52zc6frpwz",
			valid: false,
		},

		// String with non UTF-8 characters.
		{
			alias: "\xE0\x80\x80",
			valid: false,
		},
	}
	for i, testCase := range testCases {
		_, err := NewNodeAlias(testCase.alias)
		switch {
		case err != nil && testCase.valid:
			t.Fatalf("#%v: alias should have been invalid", i)

		case err == nil && !testCase.valid:
			t.Fatalf("#%v: invalid alias was missed", i)
		}
	}
}

// TestNodeAnnouncement1UpdateTimestamp tests HasZeroUpdateTime and
// UpdateTimestamp on NodeAnnouncement1.
func TestNodeAnnouncement1UpdateTimestamp(t *testing.T) {
	t.Parallel()

	zero := &NodeAnnouncement1{Timestamp: 0}
	require.True(t, zero.HasZeroUpdateTime())
	require.Equal(t, UnixTimestamp(0), zero.UpdateTimestamp())

	nonZero := &NodeAnnouncement1{Timestamp: 12345}
	require.False(t, nonZero.HasZeroUpdateTime())
	require.Equal(t, UnixTimestamp(12345), nonZero.UpdateTimestamp())
}
