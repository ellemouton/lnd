package chainfee

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockChainBackend struct {
	minRelayFee SatPerKWeight
	callCount   int
}

func (m *mockChainBackend) fetchFee() (SatPerKWeight, error) {
	m.callCount++
	return m.minRelayFee, nil
}

// TestMinRelayFeeManager tests that the minRelayFeeManager returns an up to
// date min relay fee by querying the chain backend and that it returns a cached
// fee if the chain backend was recently queried.
func TestMinRelayFeeManager(t *testing.T) {
	t.Parallel()

	chainBackend := &mockChainBackend{
		minRelayFee: SatPerKWeight(1000),
	}

	// Initialise the min relay fee manager. This should call the chain
	// backend once.
	feeManager, err := newMinRelayFeeManager(
		100*time.Millisecond,
		chainBackend.fetchFee,
	)
	require.NoError(t, err)
	require.Equal(t, 1, chainBackend.callCount)

	// If the fee is requested again, the stored fee should be returned
	// and the chain backend should not be queried.
	chainBackend.minRelayFee = SatPerKWeight(2000)
	minRelayFee := feeManager.fetchMinFee()
	require.Equal(t, minRelayFee, SatPerKWeight(1000))
	require.Equal(t, 1, chainBackend.callCount)

	// Fake the passing of time.
	feeManager.lastUpdatedTime = time.Now().Add(-200 * time.Millisecond)

	// If the fee is queried again after the backoff period has passed
	// then the chain backend should be queried again for the min relay
	// fee.
	minRelayFee = feeManager.fetchMinFee()
	require.Equal(t, SatPerKWeight(2000), minRelayFee)
	require.Equal(t, 2, chainBackend.callCount)
}
