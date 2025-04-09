package htlcswitch

import (
	"context"
	"testing"

	"github.com/lightningnetwork/lnd/graph/db/models"
	"github.com/lightningnetwork/lnd/lnwire"
	"github.com/stretchr/testify/require"
)

func TestHeldHtlcSetEmpty(t *testing.T) {
	set := newHeldHtlcSet()

	// Test operations on an empty set.
	require.False(t, set.exists(models.CircuitKey{}))

	_, err := set.pop(models.CircuitKey{})
	require.Error(t, err)

	set.popAll(context.Background(),
		func(_ context.Context, _ InterceptedForward) {
			require.Fail(t, "unexpected fwd")
		},
	)
}

func TestHeldHtlcSet(t *testing.T) {
	set := newHeldHtlcSet()

	key := models.CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(1),
		HtlcID: 2,
	}

	// Test pushing a nil forward.
	require.Error(t, set.push(key, nil))

	// Test pushing a forward.
	fwd := &interceptedForward{
		htlc: &lnwire.UpdateAddHTLC{},
	}
	require.NoError(t, set.push(key, fwd))

	// Re-pushing should fail.
	require.Error(t, set.push(key, fwd))

	// Test popping the fwd.
	poppedFwd, err := set.pop(key)
	require.NoError(t, err)
	require.Equal(t, fwd, poppedFwd)

	_, err = set.pop(key)
	require.Error(t, err)

	// Pushing the forward again.
	require.NoError(t, set.push(key, fwd))

	// Test for each.
	var cbCalled bool
	set.forEach(func(_ InterceptedForward) {
		cbCalled = true

		require.Equal(t, fwd, poppedFwd)
	})
	require.True(t, cbCalled)

	// Test popping all forwards.
	cbCalled = false
	set.popAll(context.Background(), func(_ context.Context,
		_ InterceptedForward) {

		cbCalled = true
		require.Equal(t, fwd, poppedFwd)
	})
	require.True(t, cbCalled)

	_, err = set.pop(key)
	require.Error(t, err)
}

func TestHeldHtlcSetAutoFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	set := newHeldHtlcSet()

	key := models.CircuitKey{
		ChanID: lnwire.NewShortChanIDFromInt(1),
		HtlcID: 2,
	}

	const autoFailHeight = 100
	fwd := &interceptedForward{
		packet:         &htlcPacket{},
		htlc:           &lnwire.UpdateAddHTLC{},
		autoFailHeight: autoFailHeight,
	}
	require.NoError(t, set.push(key, fwd))

	// Test popping auto fails up to one block before the auto-fail height
	// of our forward.
	set.popAutoFails(
		ctx, autoFailHeight-1,
		func(_ context.Context, _ InterceptedForward) {
			require.Fail(t, "unexpected fwd")
		},
	)

	// Popping succeeds at the auto-fail height.
	cbCalled := false
	set.popAutoFails(
		ctx, autoFailHeight,
		func(_ context.Context, poppedFwd InterceptedForward) {
			cbCalled = true

			require.Equal(t, fwd, poppedFwd)
		},
	)
	require.True(t, cbCalled)

	// After this, there should be nothing more to pop.
	set.popAutoFails(
		ctx, autoFailHeight,
		func(_ context.Context, _ InterceptedForward) {
			require.Fail(t, "unexpected fwd")
		},
	)
}
