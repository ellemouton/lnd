package channeldb

import (
	"net"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	addr1 = &net.TCPAddr{IP: (net.IP)([]byte{0x1}), Port: 1}
	addr2 = &net.TCPAddr{IP: (net.IP)([]byte{0x2}), Port: 2}
	addr3 = &net.TCPAddr{IP: (net.IP)([]byte{0x3}), Port: 3}
)

// TestMultiAddrSource tests that the multiAddrSource correctly merges and
// deduplicates the results of a set of AddrSource implementations.
func TestMultiAddrSource(t *testing.T) {
	t.Parallel()

	var pk1 = newTestPubKey(t)

	t.Run("both sources have results", func(t *testing.T) {
		t.Parallel()

		var (
			src1 = newMockAddrSource(t)
			src2 = newMockAddrSource(t)
		)
		t.Cleanup(func() {
			src1.AssertExpectations(t)
			src2.AssertExpectations(t)
		})

		// Let source 1 know of 2 addresses (addr 1 and 2) for node 1.
		src1.On("AddrsForNode", pk1).Return(
			[]net.Addr{addr1, addr2}, nil,
		).Once()

		// Let source 2 know of 2 addresses (addr 2 and 3) for node 1.
		src2.On("AddrsForNode", pk1).Return(
			[]net.Addr{addr2, addr3}, nil,
		).Once()

		// Create a multi-addr source that consists of both source 1
		// and 2.
		multiSrc := NewMultiAddrSource(src1, src2)

		// Query it for the addresses known for node 1. The results
		// should contain addr 1, 2 and 3.
		addrs, err := multiSrc.AddrsForNode(pk1)
		require.NoError(t, err)
		require.ElementsMatch(t, addrs, []net.Addr{addr1, addr2, addr3})
	})

	t.Run("only once source has results", func(t *testing.T) {
		t.Parallel()

		var (
			src1 = newMockAddrSource(t)
			src2 = newMockAddrSource(t)
		)
		t.Cleanup(func() {
			src1.AssertExpectations(t)
			src2.AssertExpectations(t)
		})

		// Let source 1 know of address 1 for node 1.
		src1.On("AddrsForNode", pk1).Return(
			[]net.Addr{addr1}, nil,
		).Once()
		src2.On("AddrsForNode", pk1).Return(nil, nil).Once()

		// Create a multi-addr source that consists of both source 1
		// and 2.
		multiSrc := NewMultiAddrSource(src1, src2)

		// Query it for the addresses known for node 1. The results
		// should contain addr 1.
		addrs, err := multiSrc.AddrsForNode(pk1)
		require.NoError(t, err)
		require.ElementsMatch(t, addrs, []net.Addr{addr1})
	})

}

type mockAddrSource struct {
	t *testing.T
	mock.Mock
}

var _ AddrSource = (*mockAddrSource)(nil)

func newMockAddrSource(t *testing.T) *mockAddrSource {
	return &mockAddrSource{t: t}
}

func (m *mockAddrSource) AddrsForNode(pub *btcec.PublicKey) ([]net.Addr,
	error) {

	args := m.Called(pub)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	addrs, ok := args.Get(0).([]net.Addr)
	require.True(m.t, ok)

	return addrs, args.Error(1)
}

func newTestPubKey(t *testing.T) *btcec.PublicKey {
	priv, err := btcec.NewPrivateKey()
	require.NoError(t, err)

	return priv.PubKey()
}
