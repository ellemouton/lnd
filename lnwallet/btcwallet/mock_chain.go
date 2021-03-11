package btcwallet

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

type mockChainClient struct {
	blocks         map[chainhash.Hash]*wire.MsgBlock
	chainCallCount int

	sync.RWMutex
}

func newMockChain() *mockChainClient {
	return &mockChainClient{
		blocks: make(map[chainhash.Hash]*wire.MsgBlock),
	}
}

var _ chain.Interface = (*mockChainClient)(nil)

func (m *mockChainClient) resetChainCallCount() {
	m.RLock()
	defer m.RUnlock()

	m.chainCallCount = 0
}

func (m *mockChainClient) Start() error {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil
}

func (m *mockChainClient) Stop() {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
}

func (m *mockChainClient) WaitForShutdown() {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
}

func (m *mockChainClient) GetBestBlock() (*chainhash.Hash, int32, error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil, 0, nil
}

func (m *mockChainClient) addBlock(block *wire.MsgBlock, nonce uint32) {
	m.Lock()
	block.Header.Nonce = nonce
	hash := block.Header.BlockHash()
	m.blocks[hash] = block
	m.Unlock()
}
func (m *mockChainClient) GetBlock(blockHash *chainhash.Hash) (*wire.MsgBlock, error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++

	block, ok := m.blocks[*blockHash]
	if !ok {
		return nil, fmt.Errorf("block not found")
	}

	return block, nil
}

func (m *mockChainClient) GetBlockHash(int64) (*chainhash.Hash, error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil, nil
}

func (m *mockChainClient) GetBlockHeader(*chainhash.Hash) (*wire.BlockHeader,
	error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil, nil
}

func (m *mockChainClient) IsCurrent() bool {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return false
}

func (m *mockChainClient) FilterBlocks(*chain.FilterBlocksRequest) (
	*chain.FilterBlocksResponse, error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil, nil
}

func (m *mockChainClient) BlockStamp() (*waddrmgr.BlockStamp, error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil, nil
}

func (m *mockChainClient) SendRawTransaction(*wire.MsgTx, bool) (
	*chainhash.Hash, error) {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil, nil
}

func (m *mockChainClient) Rescan(*chainhash.Hash, []btcutil.Address,
	map[wire.OutPoint]btcutil.Address) error {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil
}

func (m *mockChainClient) NotifyReceived([]btcutil.Address) error {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil
}

func (m *mockChainClient) NotifyBlocks() error {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil
}

func (m *mockChainClient) Notifications() <-chan interface{} {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return nil
}

func (m *mockChainClient) BackEnd() string {
	m.RLock()
	defer m.RUnlock()
	m.chainCallCount++
	return "mock"
}
