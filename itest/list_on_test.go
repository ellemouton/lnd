//go:build integration

package itest

import (
	"fmt"

	"github.com/lightningnetwork/lnd/lntest"
)

var allTestCases = []*lntest.TestCase{
	{
		Name:     "update channel policy",
		TestFunc: testUpdateChannelPolicy,
	},
}

// appendPrefixed is used to add a prefix to each test name in the subtests
// before appending them to the main test cases.
func appendPrefixed(prefix string, testCases,
	subtestCases []*lntest.TestCase) []*lntest.TestCase {

	for _, tc := range subtestCases {
		name := fmt.Sprintf("%s-%s", prefix, tc.Name)
		testCases = append(testCases, &lntest.TestCase{
			Name:     name,
			TestFunc: tc.TestFunc,
		})
	}

	return testCases
}

func init() {
	// Register subtests.
	allTestCases = appendPrefixed(
		"multihop", allTestCases, multiHopForceCloseTestCases,
	)
	allTestCases = appendPrefixed(
		"watchtower", allTestCases, watchtowerTestCases,
	)
	allTestCases = appendPrefixed(
		"psbt", allTestCases, psbtFundingTestCases,
	)
	allTestCases = appendPrefixed(
		"remote signer", allTestCases, remoteSignerTestCases,
	)
	allTestCases = appendPrefixed(
		"channel backup", allTestCases, channelRestoreTestCases,
	)
	allTestCases = appendPrefixed(
		"utxo selection", allTestCases, fundUtxoSelectionTestCases,
	)
	allTestCases = appendPrefixed(
		"zero conf", allTestCases, zeroConfPolicyTestCases,
	)
	allTestCases = appendPrefixed(
		"channel fee policy", allTestCases, channelFeePolicyTestCases,
	)
	allTestCases = appendPrefixed(
		"wallet import account", allTestCases,
		walletImportAccountTestCases,
	)
	allTestCases = appendPrefixed(
		"funding", allTestCases, basicFundingTestCases,
	)
	allTestCases = appendPrefixed(
		"send to route", allTestCases, sendToRouteTestCases,
	)
	allTestCases = appendPrefixed(
		"channel force close", allTestCases, channelForceCloseTestCases,
	)

}
